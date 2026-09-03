package loop

import (
	"encoding/json"
	"strings"

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
)

// aTaskCall is what the loop reads out of a task call before anything else
// does: which operation it is and the one piece of text it carries. Everything
// else in the call belongs to the task tool.
type aTaskCall struct {
	// Operation says which operation this call is.
	Operation string `json:"operation"`
	// Text is the one line the operation carries.
	Text string `json:"text"`
}

// theLoopsOwnOperation answers a task call that belongs to the loop rather than
// to the record. It returns the result the model gets back, whether that result
// is a refusal, and whether this was one of the loop's own operations at all.
func (running *run) theLoopsOwnOperation(call contract.ToolCall) (string, bool, bool) {
	if call.Name != contract.ToolTask {
		return "", false, false
	}
	written := aTaskCall{}
	if err := json.Unmarshal(call.Input, &written); err != nil {
		return "", false, false
	}
	if written.Operation != OperationStopNow {
		return "", false, false
	}
	line := strings.TrimSpace(written.Text)
	if line == "" {
		return "This call says to stop and does not say which line of the stop list came true, " +
			"so write that line in the text field.", true, true
	}
	running.stopNow = line
	return "The task is stopping, because " + line + ".", false, true
}

// theStopTheModelAskedFor is the line the model named as having come true, taken
// once so that it stops the task once.
func (running *run) theStopTheModelAskedFor() string {
	line := running.stopNow
	running.stopNow = ""
	return line
}
