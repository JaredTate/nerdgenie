package loop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
	"github.com/JaredTate/coeus/internal/repair"
)

// The bounds one task runs inside, beyond the budget itself.
const (
	// extraRounds is how many calls past the budget the loop may make: the one
	// with the tools off that asks for the final report, and the review.
	extraRounds = 2
	// MaxMessagesKept is how many recent messages the loop carries from one
	// call to the next. The record is what a task remembers; this is only the
	// tail of the conversation around it.
	MaxMessagesKept = 200
	// MaxDoneCheckNudges is how many times the model is sent back to work for a
	// done list with nothing behind it before the task is given up on.
	MaxDoneCheckNudges = 3
)

// run is one task in flight, with everything that is true only while it runs.
type run struct {
	theLoop        *Loop
	task           Task
	channel        contract.Channel
	keeper         *record.Keeper
	jobSummary     string
	messages       []contract.Message
	roundsAllowed  int
	timeAllowed    time.Duration
	startedAt      time.Time
	roundsUsed     int
	failedParses   int
	doneNudges     int
	recentCalls    []string
	repeatsRefused int
	lastOrient     string
	browserFact    string
	commandFact    string
	filesChanged   []string
	hadCorrection  bool
	hadFailure     bool
	hadStop        bool
	stopLine       string
}

// newRun sets one task up: its budget, its record if it is being resumed, the
// summary of its job if it belongs to one, and the user's message.
func (theLoop *Loop) newRun(ctx context.Context, task Task) (*run, error) {
	if task.Channel == nil {
		return nil, errors.New("a task needs a channel to answer on, so pass the one the message came in through")
	}
	running := &run{
		theLoop:       theLoop,
		task:          task,
		channel:       task.Channel,
		startedAt:     theLoop.options.Clock.Now(),
		roundsAllowed: budgetRounds(task, theLoop.options.Caps),
		timeAllowed:   budgetTime(task, theLoop.options.Caps),
	}
	if err := running.resume(ctx); err != nil {
		return nil, err
	}
	if err := running.readJobSummary(ctx); err != nil {
		return nil, err
	}
	running.remember(contract.Message{Role: contract.RoleUser, Text: task.Message.Text})
	if err := theLoop.logEvent(ctx, running.taskID(), contract.EventMessage, task.Message); err != nil {
		return nil, err
	}
	return running, nil
}

// budgetRounds is how many rounds this task may take: its own budget when a
// skill set one, and the cap otherwise.
func budgetRounds(task Task, caps contract.Caps) int {
	if task.Budget.Rounds > 0 {
		return task.Budget.Rounds
	}
	return caps.RoundsPerTask
}

// budgetTime is how long this task may take, the same way.
func budgetTime(task Task, caps contract.Caps) time.Duration {
	if task.Budget.Time > 0 {
		return task.Budget.Time
	}
	return caps.TimePerTask
}

// resume picks a waiting or stopped task up again from its last checkpoint,
// which is what the user's next message does even days later.
func (running *run) resume(ctx context.Context) error {
	if running.task.ResumeID == "" {
		return nil
	}
	keeper, err := record.Load(ctx, running.theLoop.options.Store, contract.RecordTask, running.task.ResumeID)
	if err != nil {
		return fmt.Errorf("cannot pick task %s up again: %w", running.task.ResumeID, err)
	}
	running.keeper = keeper
	header := keeper.Record().Header
	running.roundsAllowed = header.RoundsLeft
	running.timeAllowed = time.Duration(header.MinutesLeft) * time.Minute
	return keeper.SetStatus(ctx, contract.StatusRunning)
}

// readJobSummary prints the job this task belongs to, which rides above the
// task record so that a later task can lean on the reports of the earlier ones.
func (running *run) readJobSummary(ctx context.Context) error {
	if running.task.FromJob == nil || running.theLoop.options.Jobs == nil {
		return nil
	}
	held, err := running.theLoop.options.Jobs.Load(ctx, running.task.FromJob.JobID)
	if err != nil {
		return fmt.Errorf("cannot read job %s to put its summary above the task: %w", running.task.FromJob.JobID, err)
	}
	running.jobSummary = string(record.Print(held))
	return nil
}

// taskID is the record's number, and is empty until the first tool call makes
// one.
func (running *run) taskID() string {
	if running.keeper == nil {
		return ""
	}
	return running.keeper.ID()
}

// play runs round after round until the task reaches an end state.
func (running *run) play(ctx context.Context) (Outcome, error) {
	for range running.roundsAllowed + extraRounds {
		outcome, more, err := running.oneRound(ctx)
		if err != nil {
			return outcome, err
		}
		if !more {
			return outcome, nil
		}
	}
	return running.finalReport(ctx, "the task used every round it was allowed")
}

// oneRound is one turn of the loop: orient, call, guard, permit, run, update.
// It returns false when the task has reached an end state.
func (running *run) oneRound(ctx context.Context) (Outcome, bool, error) {
	if spent := running.budgetIsSpent(); spent != "" {
		outcome, err := running.finalReport(ctx, spent)
		return outcome, false, err
	}
	if running.theLoop.stopAsked() {
		outcome, err := running.stopHere(ctx, "the user asked the task to stop")
		return outcome, false, err
	}
	reply, err := running.callTheModel(ctx)
	if err != nil {
		outcome, failed := running.failHere(ctx, err)
		return outcome, false, failed
	}
	running.roundsUsed++
	found := repair.Find(reply, running.specs(), running.failedParses)
	running.orient(found.Text, reply.Text)
	if err := running.writeCostAndBudget(ctx, reply.Usage); err != nil {
		return Outcome{}, false, err
	}
	if found.Problem != "" {
		running.failedParses++
		running.remember(contract.Message{Role: contract.RoleUser, Text: found.Problem + "\n" + ThreeOptions})
		return Outcome{}, true, nil
	}
	running.failedParses = 0
	if len(found.Calls) == 0 {
		return running.endOfTurn(ctx, found.Text, reply.Finish)
	}
	return running.runTheCalls(ctx, found)
}

// callTheModel builds the working context and makes one call, streaming the
// reply as it arrives. Retries and the fallback chain live in the provider, so
// one call here is one call.
func (running *run) callTheModel(ctx context.Context) (contract.Reply, error) {
	request, err := running.buildRequest(ctx, false)
	if err != nil {
		return contract.Reply{}, err
	}
	return running.theLoop.options.Model.Send(ctx, request, running.theLoop.options.Deltas)
}

// buildRequest asks the working-context builder for one call's prompt.
func (running *run) buildRequest(ctx context.Context, toolsOff bool) (contract.Request, error) {
	request, err := running.theLoop.options.Context.Build(ctx, BuildInput{
		Record:          running.recordOrNothing(),
		JobSummary:      running.jobSummary,
		Messages:        running.messages,
		Tools:           running.specs(),
		ToolsOff:        toolsOff,
		MemoryHint:      running.memoryHint(ctx),
		MaxOutputTokens: running.theLoop.options.Caps.OutputTokensPerCall,
	})
	if err != nil {
		return contract.Request{}, fmt.Errorf("cannot build the working context for this call: %w", err)
	}
	return request, nil
}

// recordOrNothing is the record to put in front of the model, or nil when the
// task has not made one yet.
func (running *run) recordOrNothing() *contract.Record {
	if running.keeper == nil {
		return nil
	}
	held := running.keeper.Record()
	return &held
}

// specs is what the model is told about the tools.
func (running *run) specs() []contract.ToolSpec {
	return running.theLoop.options.Tools.Specs()
}

// memoryHint is the few lines from memory that ride at the end of the prompt. A
// memory that cannot be searched costs the turn a hint and nothing more, so the
// error is not carried up.
func (running *run) memoryHint(ctx context.Context) []string {
	if running.theLoop.options.Memory == nil {
		return nil
	}
	asked := strings.TrimSpace(running.task.Message.Text + " " + running.lastOrient)
	hint, err := running.theLoop.options.Memory.Hint(ctx, asked)
	if err != nil {
		return nil
	}
	return hint
}

// remember appends one message to the conversation the model sees, keeping only
// the most recent ones, because every buffer here has a cap.
func (running *run) remember(message contract.Message) {
	if message.Text == "" && len(message.ToolCalls) == 0 && len(message.ToolResults) == 0 {
		return
	}
	running.messages = append(running.messages, message)
	if len(running.messages) > MaxMessagesKept {
		running.messages = running.messages[len(running.messages)-MaxMessagesKept:]
	}
}

// orient takes the model's first line, which says where the work stands, and
// keeps it for the record's situation.
func (running *run) orient(text string, whole string) {
	said := firstLine(text)
	if said == "" {
		said = firstLine(whole)
	}
	if said != "" {
		running.lastOrient = said
	}
}

// firstLine is the first line of a piece of text with nothing else on it.
func firstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
