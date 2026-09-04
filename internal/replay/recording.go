package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// The bounds one recording is read inside. A task has no round budget unless
// the user sets one, and a task of a few hundred rounds is a long one, so
// anything past these is a log that has gone wrong rather than a task, and the
// reader says so instead of filling memory.
const (
	// MaxRoundsRead is how many model replies one recording may hold.
	MaxRoundsRead = 2000
	// MaxCallsRead is how many tool calls one recording may hold.
	MaxCallsRead = 20000
)

// theOrientPrefix is what the turn loop writes in front of the model's own
// orient line when it puts that line into the record's situation. Taking the
// prefix off is what gives the line back as the model wrote it.
const theOrientPrefix = "where the work stands: "

// Call is one tool call as the log remembers it, together with the whole text
// the tool gave back.
type Call struct {
	// Name is the tool the model asked for.
	Name string `json:"name"`
	// Input is the arguments the model wrote.
	Input json.RawMessage `json:"input"`
	// Result is everything the tool returned, which is the text the replay hands
	// back in its place.
	Result string `json:"result"`
	// Ran says a tool really saw the call. A call the guard or the permission
	// function refused never reached one, and so has no result.
	Ran bool `json:"ran"`
}

// Round is one model reply as the log remembers it: the line the model wrote to
// say where the work stood, and the tools it asked for.
type Round struct {
	// Orient is the first line of the reply, which the loop writes into the
	// record's situation.
	Orient string `json:"orient"`
	// Calls are the tools the reply asked for, in order.
	Calls []Call `json:"calls"`
	// Delivered are the messages the user sent while this round's tools ran.
	Delivered []contract.Inbound `json:"delivered,omitempty"`
}

// Recording is one task read back out of the event log: everything a replay
// needs to run it again without asking the model or the world anything.
type Recording struct {
	// TaskID is the task's number.
	TaskID string `json:"taskId"`
	// Ask is the user's message, word for word.
	Ask string `json:"ask"`
	// Origin is the channel the ask came in on.
	Origin string `json:"origin"`
	// Rounds are the model's replies that asked for tools, in order.
	Rounds []Round `json:"rounds"`
	// Answer is the last thing the task said to the user, which is what the
	// replayed model says once the recorded rounds have run out.
	Answer string `json:"answer"`
	// Final is the record as the recording left it.
	Final contract.Record `json:"-"`
	// FinalText is that same record, printed, which is how it travels in a
	// fixture file.
	FinalText string `json:"finalText"`
}

// Read reads one task's events out of the log and turns them back into the run
// that wrote them.
//
// The rounds are found from the checkpoints. The loop writes how much of the
// budget is left once per model call, before that call's tools run, so a
// checkpoint whose budget differs from the one before it is where a new round
// starts. The orient line is found the same way, out of the situation the loop
// writes once the round's tools have finished. A round the model's reply could
// not be read on is not recovered, because the log does not keep the text that
// could not be read.
func Read(ctx context.Context, store contract.Store, taskID string) (Recording, error) {
	if store == nil {
		return Recording{}, fmt.Errorf("cannot read the recording of task %q without an event log to read it from", taskID)
	}
	events, err := store.ByTask(ctx, taskID)
	// A read that came back cut short hands over the events it did read
	// together with the log's own advice on how to read the rest. That advice is
	// for a caller walking the log in pages, and a replay is not one: it needs
	// the whole run or none of it. So the real reason is what is said here.
	if err != nil && len(events) > 0 {
		return Recording{}, fmt.Errorf("task %q wrote more events than one read of the log returns, so it is too long to replay; replay a shorter task, or read it with the tasks command: %w",
			taskID, err)
	}
	if err != nil {
		return Recording{}, fmt.Errorf("cannot read the events of task %q: %w", taskID, err)
	}
	if len(events) == 0 {
		return Recording{}, fmt.Errorf("there is no task numbered %q in the event log, so check the number with the tasks command", taskID)
	}
	reading := &reader{recording: Recording{TaskID: taskID}}
	for _, event := range events {
		if err := reading.take(event); err != nil {
			return Recording{}, err
		}
	}
	return reading.finish()
}

// reader builds one recording out of the events of a task, in the order the log
// wrote them.
type reader struct {
	// recording is what has been read so far.
	recording Recording
	// round is the round being read now.
	round Round
	// started says a round is open, so that the first checkpoint does not close
	// an empty one.
	started bool
	// standing is where the record stood on the last checkpoint, which is what
	// tells one round from the next: the loop saves one checkpoint for every
	// model call with the task running, so a checkpoint at running that
	// follows another at running opens a new round. The budget line used to be
	// read for this, and a task with no budget, which is the default, has none
	// to read.
	standing contract.RecordStatus
	// orient is the last orient line seen, so that an unchanged situation does
	// not overwrite the round with a line from before it.
	orient string
	// checkpoints counts the checkpoints, because a task with none is not a task
	// this package can replay.
	checkpoints int
}

// take reads one event into the recording.
func (reading *reader) take(event contract.Event) error {
	switch event.Kind {
	case contract.EventCheckpoint:
		return reading.takeCheckpoint(event)
	case contract.EventToolCall:
		return reading.takeCall(event)
	case contract.EventToolResult:
		return reading.takeResult(event)
	case contract.EventMessage:
		return reading.takeMessage(event)
	case contract.EventReply:
		return reading.takeReply(event)
	default:
		return nil
	}
}

// takeCheckpoint reads one saved copy of the record, which is where the rounds
// and the orient lines come from.
//
// Only the first checkpoint of a record carries the user's ask; the ones after
// it name that one for it, so the ask picked up on the way past is what puts
// each of them back together.
func (reading *reader) takeCheckpoint(event contract.Event) error {
	saved := record.Checkpoint{}
	if err := json.Unmarshal(event.Body, &saved); err != nil {
		return nil
	}
	held, err := saved.Read(reading.recording.Ask)
	if err != nil {
		return nil
	}
	reading.checkpoints++
	reading.recording.Final = held
	if reading.recording.Ask == "" {
		reading.recording.Ask = held.Goal.Ask
		reading.recording.Origin = held.Header.Origin
	}
	// The orient line of a round is written into the situation once that round's
	// tools have finished, so it reaches the log in the checkpoint of the round
	// after it. It is taken before the round is closed, so that it lands on the
	// round that wrote it rather than the one about to start.
	reading.takeOrient(held)
	// The checkpoint an ending or a resume writes stands at some other status
	// on one side of it, and belongs to the round around it; the one a model
	// call writes stands at running after running, and opens the next.
	if reading.started && held.Header.Status == contract.StatusRunning && reading.standing == contract.StatusRunning {
		if err := reading.closeRound(); err != nil {
			return err
		}
	}
	reading.standing = held.Header.Status
	reading.started = true
	return nil
}

// takeOrient keeps the model's own line about where the work stands, which the
// loop wrote into the situation once this round's tools had run.
func (reading *reader) takeOrient(held contract.Record) {
	for _, fact := range held.Work.Situation {
		said, found := strings.CutPrefix(fact, theOrientPrefix)
		if found && said != reading.orient {
			reading.orient = said
			reading.round.Orient = said
		}
	}
}

// takeCall reads one tool call into the round being read.
func (reading *reader) takeCall(event contract.Event) error {
	call := contract.ToolCall{}
	if err := json.Unmarshal(event.Body, &call); err != nil || call.Name == "" {
		return nil
	}
	if len(reading.round.Calls) >= MaxCallsRead {
		return fmt.Errorf("task %q asked for more than %d tools in one round, which is a log that has gone wrong rather than a task",
			reading.recording.TaskID, MaxCallsRead)
	}
	reading.round.Calls = append(reading.round.Calls, Call{Name: call.Name, Input: call.Input})
	return nil
}

// takeResult puts the whole text a tool returned onto the call it answered,
// which is the call before it in the log.
func (reading *reader) takeResult(event contract.Event) error {
	stored := record.StoredResult{}
	if err := json.Unmarshal(event.Body, &stored); err != nil {
		return nil
	}
	if len(reading.round.Calls) == 0 {
		return nil
	}
	last := &reading.round.Calls[len(reading.round.Calls)-1]
	last.Result = stored.Text
	last.Ran = true
	return nil
}

// takeMessage keeps a message the user sent while the task was running. The
// ask itself is written under the task's number before the record exists, and
// so before any checkpoint; a message read before the first checkpoint is that
// ask, or a correction that arrived before any round had begun, and neither is
// something a replay delivers to a round, because the ask is the recording's
// own and there is no round before the first for a correction to land on.
func (reading *reader) takeMessage(event contract.Event) error {
	if reading.checkpoints == 0 {
		return nil
	}
	message := contract.Inbound{}
	if err := json.Unmarshal(event.Body, &message); err != nil || message.Text == "" {
		return nil
	}
	if message.Text == reading.recording.Ask {
		return nil
	}
	reading.round.Delivered = append(reading.round.Delivered, message)
	return nil
}

// takeReply keeps the last thing the task said to the user.
func (reading *reader) takeReply(event contract.Event) error {
	said := struct {
		Text string `json:"text"`
	}{}
	if err := json.Unmarshal(event.Body, &said); err != nil {
		return nil
	}
	if said.Text != "" {
		reading.recording.Answer = said.Text
	}
	return nil
}

// closeRound files the round being read and starts the next one. A round that
// asked for no tools and carried no message from the user is left out, because
// there is nothing in it for a replay to do.
func (reading *reader) closeRound() error {
	if len(reading.round.Calls) > 0 || len(reading.round.Delivered) > 0 {
		if len(reading.recording.Rounds) >= MaxRoundsRead {
			return fmt.Errorf("task %q ran more than %d rounds, which is a log that has gone wrong rather than a task",
				reading.recording.TaskID, MaxRoundsRead)
		}
		reading.recording.Rounds = append(reading.recording.Rounds, reading.round)
	}
	reading.round = Round{}
	return nil
}

// finish closes the last round and hands back the whole recording.
func (reading *reader) finish() (Recording, error) {
	if reading.checkpoints == 0 {
		return Recording{}, fmt.Errorf("task %q saved no checkpoint, so there is no record to replay it against",
			reading.recording.TaskID)
	}
	if err := reading.closeRound(); err != nil {
		return Recording{}, err
	}
	reading.recording.FinalText = string(record.Print(reading.recording.Final))
	return reading.recording, nil
}
