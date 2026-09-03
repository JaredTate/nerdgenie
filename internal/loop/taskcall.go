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
	switch written.Operation {
	case OperationStopNow:
		answer, refused := running.stopNowAsked(written)
		return answer, refused, true
	case OperationPinEvidence:
		answer, refused := running.pinEvidence(ctx, written)
		return answer, refused, true
	case OperationUnpinEvidence:
		answer, refused := running.unpinEvidence(written)
		return answer, refused, true
	default:
		return "", false, false
	}
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
// the window would otherwise drop is still there twenty rounds later.
func (running *run) pinEvidence(ctx context.Context, written aTaskCall) (string, bool) {
	label := strings.TrimSpace(written.Result)
	if label == "" {
		return "This call pins nothing, so name the result to pin, such as r7.", true
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
	running.pinned = append(running.pinned, workingcontext.Pin{ID: label, Text: text})
	return "The result " + label + " is pinned and stays in front of you until you unpin it.", false
}

// unpinEvidence lets one pinned result go.
func (running *run) unpinEvidence(written aTaskCall) (string, bool) {
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
	running.pinned = kept
	return "The result " + label + " is unpinned.", false
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
