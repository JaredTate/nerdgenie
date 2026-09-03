package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	workingcontext "github.com/JaredTate/coeus/internal/context"
	"github.com/JaredTate/coeus/internal/contract"
)

// The operations of the task tool that the loop keeps for itself. They are
// written in the same reply as the model's other calls, so saying one of them
// costs no extra model call, and the loop answers them before the registry sees
// them, because they are about the turn and not about the record.
const (
	// OperationStopNow says that one line of the stop list has come true. The
	// harness cannot read a line about the world, so the model says when its
	// own line has come true and names it.
	OperationStopNow = "stop_now"
	// OperationPinEvidence keeps one result in front of the model word for
	// word, however small the window is, until it is unpinned.
	OperationPinEvidence = "pin_evidence"
	// OperationUnpinEvidence lets a pinned result go.
	OperationUnpinEvidence = "unpin_evidence"
)

// MaxResultsPinned is how many results may be pinned at once. Pinned evidence
// never leaves the window, so a task that pinned everything would leave no room
// for the work it is doing.
const MaxResultsPinned = 4

// TheReplyLabel is what a done line points at when the answer to the user is
// its own proof, such as "reply to the user with exactly three words". It reads
// like a result label and is not one: the answer has not been given while the
// line is being written, so the harness holds the line back and writes the
// answer into the record as the result that proves it once it has been given.
//
// This belongs beside the result labels in internal/contract; it lives here
// until that package is opened.
const TheReplyLabel = "reply"

// MaxLinesProvedByTheReply is how many done lines may wait on the answer at
// once, because every list the harness keeps for a task has a cap.
const MaxLinesProvedByTheReply = 8

// aTaskCall is what the loop reads out of a task call before anything else
// does: which operation it is and the one piece of text it carries. Everything
// else in the call belongs to the task tool.
type aTaskCall struct {
	// Operation says which operation this call is.
	Operation string `json:"operation"`
	// Text is the one line the operation carries.
	Text string `json:"text"`
	// Result is the label of the result the operation is about, such as "r7".
	Result string `json:"result"`
}

// theLoopsOwnOperation answers a task call that belongs to the loop rather than
// to the record. It returns the result the model gets back, whether that result
// is a refusal, and whether this was one of the loop's own operations at all.
func (running *run) theLoopsOwnOperation(ctx context.Context, call contract.ToolCall) (string, bool, bool) {
	if call.Name != contract.ToolTask {
		return "", false, false
	}
	written := aTaskCall{}
	if err := json.Unmarshal(call.Input, &written); err != nil {
		return "", false, false
	}
	if answer, refused, mine := running.takeTheLinesTheReplyProves(ctx, call); mine {
		return answer, refused, true
	}
	switch written.Operation {
	case OperationStopNow:
		answer, refused := running.stopNowAsked(written)
		return answer, refused, true
	case OperationPinEvidence:
		answer, refused := running.pinEvidence(ctx, written)
		return answer, refused, true
	case OperationUnpinEvidence:
		answer, refused := running.unpinEvidence(ctx, written)
		return answer, refused, true
	default:
		return "", false, false
	}
}

// takeTheLinesTheReplyProves answers a record write that names the reply as the
// proof of a done line. The record refuses a line pointing at a result it never
// wrote, and the answer is not a result until it has been given, so the harness
// writes the rest of the change now, holds those lines back, and points them at
// the answer the moment the model gives it. A write that names the reply nowhere
// is none of the loop's business and goes to the tool as it always did.
func (running *run) takeTheLinesTheReplyProves(ctx context.Context, call contract.ToolCall) (string, bool, bool) {
	if running.keeper == nil {
		return "", false, false
	}
	update, err := readRecordUpdate(call.Input, running.keeper.Record())
	if err != nil {
		return "", false, false
	}
	waiting := []string{}
	for at := range update.DoneWhen {
		if !strings.EqualFold(update.DoneWhen[at].ResultID, TheReplyLabel) {
			continue
		}
		waiting = append(waiting, update.DoneWhen[at].Text)
		update.DoneWhen[at].Done, update.DoneWhen[at].ResultID = false, ""
	}
	if len(waiting) == 0 {
		return "", false, false
	}
	if len(running.provedByTheReply)+len(waiting) > MaxLinesProvedByTheReply {
		return fmt.Sprintf("%d done lines are already waiting on your answer, which is as many as one task may have, "+
			"so prove the rest with results.", len(running.provedByTheReply)), true, true
	}
	if err := running.keeper.Apply(ctx, update); err != nil {
		return "the record refused that change: " + err.Error(), true, true
	}
	running.provedByTheReply = append(running.provedByTheReply, waiting...)
	return fmt.Sprintf("The record is written, and %d of its done lines are proved by your answer, "+
		"so I will write your answer into the record as the result that proves them when you give it.", len(waiting)), false, true
}

// stopNowAsked takes the line the model says has come true, and refuses a stop
// that names none.
func (running *run) stopNowAsked(written aTaskCall) (string, bool) {
	line := strings.TrimSpace(written.Text)
	if line == "" {
		return "This call says to stop and does not say which line of the stop list came true, " +
			"so write that line in the text field.", true
	}
	running.stopNow = line
	return "The task is stopping, because " + line + ".", false
}

// pinEvidence keeps one result in front of the model word for word. The text is
// read out of the record now and held for the rest of the task, so that a result
// the window would otherwise drop is still there twenty rounds later, and the
// mark is written into the record itself, so that it is still there after the
// task has been put down and picked up again.
func (running *run) pinEvidence(ctx context.Context, written aTaskCall) (string, bool) {
	label := strings.TrimSpace(written.Result)
	if label == "" {
		return "This call pins nothing, so name the result to pin, such as r7.", true
	}
	if running.keeper == nil {
		return "This task has written no results yet, because nothing has been run, so run something before pinning it.", true
	}
	if running.alreadyPinned(label) {
		return "The result " + label + " is already pinned.", false
	}
	if len(running.pinned) >= MaxResultsPinned {
		return fmt.Sprintf("%d results are pinned already, which is as many as one task may pin, "+
			"so unpin one before pinning another.", MaxResultsPinned), true
	}
	text, err := running.keeper.Read(ctx, label)
	if err != nil {
		return fmt.Sprintf("there is no result %s in this task's record, so name one the record holds: %s", label, err), true
	}
	if err := running.keeper.Pin(ctx, label, true); err != nil {
		return fmt.Sprintf("the record would not mark %s pinned: %s", label, err), true
	}
	running.pinned = append(running.pinned, workingcontext.Pin{ID: label, Text: text})
	return "The result " + label + " is pinned and stays in front of you until you unpin it.", false
}

// unpinEvidence lets one pinned result go, in the record as well as in the
// window, so that picking the task up again does not bring it back.
func (running *run) unpinEvidence(ctx context.Context, written aTaskCall) (string, bool) {
	label := strings.TrimSpace(written.Result)
	kept := []workingcontext.Pin{}
	for _, pin := range running.pinned {
		if pin.ID != label {
			kept = append(kept, pin)
		}
	}
	if len(kept) == len(running.pinned) {
		return "The result " + label + " was not pinned, so there was nothing to unpin.", true
	}
	if err := running.keeper.Pin(ctx, label, false); err != nil {
		return fmt.Sprintf("the record would not take the pin off %s: %s", label, err), true
	}
	running.pinned = kept
	return "The result " + label + " is unpinned.", false
}

// takeThePinsBackFromTheRecord puts the evidence a resumed task had pinned back
// in front of the model. The marks are in the record, which is the only thing
// that survives the wait; the texts are in the log under their own labels, and
// one that cannot be read back is left out rather than taking the resume down
// with it.
func (running *run) takeThePinsBackFromTheRecord(ctx context.Context) {
	for _, result := range running.keeper.Record().Work.Results {
		if !result.Pinned || len(running.pinned) >= MaxResultsPinned {
			continue
		}
		text, err := running.keeper.Read(ctx, result.ID)
		if err != nil {
			continue
		}
		running.pinned = append(running.pinned, workingcontext.Pin{ID: result.ID, Text: text})
	}
}

// alreadyPinned says whether one result is pinned already.
func (running *run) alreadyPinned(label string) bool {
	for _, pin := range running.pinned {
		if pin.ID == label {
			return true
		}
	}
	return false
}

// theStopTheModelAskedFor is the line the model named as having come true, taken
// once so that it stops the task once.
func (running *run) theStopTheModelAskedFor() string {
	line := running.stopNow
	running.stopNow = ""
	return line
}
