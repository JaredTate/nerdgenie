package loop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	workingcontext "github.com/JaredTate/nerdgenie/internal/context"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/repair"
)

// The bounds one task runs inside, beyond the budget itself.
const (
	// extraRounds is how many turns of the loop are left past the budget: the
	// one in which the loop notices the budget is spent and asks for the final
	// report with the tools off, and one of slack behind it. The report is one
	// model call, and it is the only one that ending makes.
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
	theLoop          *Loop
	task             Task
	channel          contract.Channel
	keeper           *record.Keeper
	jobSummary       string
	messages         []contract.Message
	roundsAllowed    int
	timeAllowed      time.Duration
	startedAt        time.Time
	roundsUsed       int
	failedParses     int
	doneNudges       int
	recentCalls      []pastCall
	lastOrient       string
	browserFact      string
	commandFact      string
	continuedFact    string
	filesChanged     []string
	hadCorrection    bool
	hadFailure       bool
	hadStop          bool
	lessonUnoffered  bool
	stopLine         string
	stopNow          string
	pinned           []workingcontext.Pin
	provedByTheReply []string
	number           string
	perTask          contract.ToolRegistry
}

// tools is the registry this task's calls go to: the one built for this task
// when the caller builds one, and the one every task shares otherwise.
func (running *run) tools() contract.ToolRegistry {
	if running.perTask != nil {
		return running.perTask
	}
	return running.theLoop.options.Tools
}

// useTheTaskRegistry asks the caller for the registry of this task. It is built
// before the first call, because the model has to be told about the tools on
// the very first one, and the record it is given finds the keeper when there is
// one, because a record is only made on the first tool call.
func (running *run) useTheTaskRegistry() error {
	if running.theLoop.options.ToolsForTask == nil {
		return nil
	}
	made, err := running.theLoop.options.ToolsForTask(running.number, theRecordOfTheTask{running: running})
	if err != nil {
		return fmt.Errorf("cannot build the tools for task %s: %w", running.number, err)
	}
	running.perTask = made
	return nil
}

// TaskRecord is the record of the task running now, as the tools that read and
// write it see it. A *record.Keeper is one, and so is what the loop hands the
// tools before the first tool call has made a record.
type TaskRecord interface {
	// Record returns the record as it stands.
	Record() contract.Record
	// Apply writes the model's half of it, all or nothing.
	Apply(ctx context.Context, update record.Update) error
	// Read brings back the whole text of one result by its label.
	Read(ctx context.Context, id string) (string, error)
}

// theRecordOfTheTask is the record of the task running now, found when it is
// asked for rather than when the tools were built.
type theRecordOfTheTask struct {
	running *run
}

// Record returns the record as it stands, which is empty until the first tool
// call has made one.
func (held theRecordOfTheTask) Record() contract.Record {
	if held.running.keeper == nil {
		return contract.Record{}
	}
	return held.running.keeper.Record()
}

// Apply writes the model's half of the record.
func (held theRecordOfTheTask) Apply(ctx context.Context, update record.Update) error {
	if held.running.keeper == nil {
		return errors.New("this task has no record yet, so ask for a tool before writing the record")
	}
	return held.running.keeper.Apply(ctx, update)
}

// Read brings back the whole text of one result by its label.
func (held theRecordOfTheTask) Read(ctx context.Context, id string) (string, error) {
	if held.running.keeper == nil {
		return "", fmt.Errorf("this task has no result %s yet, because nothing has been run", id)
	}
	return held.running.keeper.Read(ctx, id)
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
	if err := running.takeANumber(ctx); err != nil {
		return nil, err
	}
	if err := running.resume(ctx); err != nil {
		return nil, err
	}
	if err := running.useTheTaskRegistry(); err != nil {
		return nil, err
	}
	if err := running.readJobSummary(ctx); err != nil {
		return nil, err
	}
	running.remember(contract.Message{Role: contract.RoleUser, Text: task.Message.Text})
	// The ask is written under the number the task has just taken rather than
	// under the record's, which does not exist until the first tool call, so
	// that the one event saying which screen the task came from is the task's
	// own. A restart reads it back to know whose task is waiting.
	if err := theLoop.logEvent(ctx, running.number, contract.EventMessage, task.Message); err != nil {
		return nil, err
	}
	return running, nil
}

// resume picks a waiting or stopped task up again from its last checkpoint,
// which is what the user's next message does even days later.
//
// The budget it picks up on is the record's own, so that a task cannot buy
// itself more rounds by waiting for an answer it asked for. A task that stopped
// because its budget ran out is the one exception, and it is the whole of
// carryOnFromAStop below.
func (running *run) resume(ctx context.Context) error {
	if running.task.ResumeID == "" {
		return nil
	}
	keeper, err := record.Load(ctx, running.theLoop.options.Store, contract.RecordTask, running.task.ResumeID)
	if err != nil {
		return fmt.Errorf("cannot pick task %s up again: %w", running.task.ResumeID, err)
	}
	keeper.SaveOncePerRound()
	running.keeper = keeper
	running.takeThePinsBackFromTheRecord(ctx)
	header := keeper.Record().Header
	running.roundsAllowed, running.timeAllowed = 0, 0
	if !header.NoRoundBudget {
		running.roundsAllowed = header.RoundsLeft
	}
	if !header.NoTimeBudget {
		running.timeAllowed = time.Duration(header.MinutesLeft) * time.Minute
	}
	if err := running.carryOnFromAStop(ctx, header.Status); err != nil {
		return err
	}
	return keeper.SetStatus(ctx, contract.StatusRunning)
}

// carryOnFromAStop gives a stopped task a fresh budget, because a person who
// says to carry on is asking for more. A task whose budget ran out ends stopped
// with nothing left and a report saying to tell it how to carry on; picked up on
// the nothing it stopped with, it spent its two spare rounds writing that same
// ending again and told the person the budget was used up, which is the answer
// to a question they had already answered.
//
// A waiting task is left alone. There the model stopped the work to ask
// something, the budget was never what ran out, and a wait that bought rounds
// would be a way round the budget rather than an answer to the person.
func (running *run) carryOnFromAStop(ctx context.Context, standing contract.RecordStatus) error {
	if standing != contract.StatusStopped {
		return nil
	}
	running.roundsAllowed = budgetRounds(running.task, running.theLoop.options.Caps)
	running.timeAllowed = budgetTime(running.task, running.theLoop.options.Caps)
	given := describeBudget(running.roundsAllowed, running.timeAllowed)
	running.continuedFact = "the person asked this task to carry on, so it was given a fresh budget of " + given
	if given == "no budget" {
		running.continuedFact = "the person asked this task to carry on, and it runs with no budget"
	}
	if err := running.keeper.SetBudget(ctx, running.budgetLeft()); err != nil {
		return fmt.Errorf("cannot give task %s the fresh budget it was asked to carry on with: %w",
			running.task.ResumeID, err)
	}
	return nil
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

// takeANumber gives the task the number its record will carry. It is settled
// before the first call, because the tools that write the record are built with
// it, and a task that never needs a record simply never uses it.
func (running *run) takeANumber(ctx context.Context) error {
	if running.task.ResumeID != "" {
		running.number = running.task.ResumeID
		return nil
	}
	number, err := running.theLoop.nextTaskNumber(ctx)
	if err != nil {
		return err
	}
	running.number = number
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

// play runs the rounds, and ends the task as failed when a round could not be
// finished and the record still stands at running: a write that failed because
// the turn was cut off under it used to come straight back out of the loop,
// leaving the record at running with nobody working on it, which is a task the
// program has lost. The ending is written under a context of its own, so it
// lands even when the turn's is cancelled.
func (running *run) play(ctx context.Context) (Outcome, error) {
	outcome, err := running.playTheRounds(ctx)
	if err != nil && running.standsAtRunning() {
		return running.failHere(ctx, err)
	}
	return outcome, err
}

// standsAtRunning says whether the task has a record and that record still
// says it is running, which is the state nothing may leave a task in.
func (running *run) standsAtRunning() bool {
	return running.keeper != nil && running.keeper.Record().Header.Status == contract.StatusRunning
}

// playTheRounds runs round after round until the task reaches an end state. A
// task with a round budget is also held to that many rounds and the two spare
// ones here, as a second wall behind the check at the top of every round. A
// task with no round budget, which is the default, has no wall: what ends it is
// its own answer, a stop line, the person's stop, the detector, a failure, or
// the context it runs under being cancelled, and nothing else.
func (running *run) playTheRounds(ctx context.Context) (Outcome, error) {
	for round := 0; running.mayPlayRound(round); round++ {
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
		outcome, err := running.stopForThePerson(ctx, "the user asked the task to stop")
		return outcome, false, err
	}
	reply, err := running.callTheModel(ctx)
	if err != nil {
		// A call that was cancelled because somebody asked the task to stop is
		// a stop, not a failure: the person pressed Escape and is owed the
		// stopped report rather than an error about a cancelled context.
		if running.theLoop.stopAsked() {
			outcome, stopped := running.stopForThePerson(ctx, "the user asked the task to stop")
			return outcome, false, stopped
		}
		outcome, failed := running.failHere(ctx, err)
		return outcome, false, failed
	}
	running.roundsUsed++
	found := repair.Find(reply, running.specs(), running.failedParses)
	running.orient(found.Text, reply.Text)
	if err := running.writeCostAndBudget(ctx, reply.Usage); err != nil {
		return Outcome{}, false, err
	}
	// One model call is one checkpoint, taken here: after the call and before
	// anything this round does, so that it carries this round's budget and the
	// whole of the round before it, which is where a replay reads the boundary.
	if err := running.saveTheRound(ctx); err != nil {
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

	// The call gets a context of its own so that a stop can cancel it without
	// cancelling the task, which still has a stopped report to write and send.
	calling, stopTheCall := context.WithCancel(ctx)
	defer stopTheCall()
	running.theLoop.holdTheCall(stopTheCall)
	defer running.theLoop.releaseTheCall()
	return running.theLoop.options.Model.Send(calling, request, running.theLoop.options.Deltas)
}

// buildRequest asks the working-context builder for one call's prompt.
func (running *run) buildRequest(ctx context.Context, toolsOff bool) (contract.Request, error) {
	request, err := running.theLoop.options.Context.Build(ctx, BuildInput{
		Record:        running.recordOrNothing(),
		ContextLength: running.theLoop.options.Model.ContextLength(),
		JobSummary:    running.jobSummary,
		Messages:      running.messages,
		Tools:         running.specs(),
		Pinned:        running.pinned,
		ToolsOff:      toolsOff,
		MemoryHint:    running.memoryHint(ctx),
	})
	if err != nil {
		return contract.Request{}, fmt.Errorf("cannot build the working context for this call: %w", err)
	}
	// How hard to think is not part of the working context, so it goes on here,
	// beside the model it was chosen for: the provider reads it off the request
	// and falls back to the level in config.toml when the request names none.
	request.Think = running.theLoop.thinkFor(running.theLoop.options.Model.Name())
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

// specs is what the model is told about the tools. When the registry holds no
// task tool, which is what a registry built without the record of the task
// running now looks like, the loop adds the one it applies itself, so that the
// model is always told how to write its half of the record.
func (running *run) specs() []contract.ToolSpec {
	specs := running.tools().Specs()
	if _, found := running.tools().Lookup(contract.ToolTask); !found {
		specs = append(specs, TheTaskToolSpec)
	}
	return specs
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
