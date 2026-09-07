package loop

import (
	"fmt"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The calls a rethink closes. The model's first line said "time to stop
// circling" for fifty rounds while it made the same call on every one of
// them, so after a rethink the call it was repeating is refused, before the
// detector sees it, for ClosedForRounds rounds, with a line saying the answer
// is on the record already and which line of the rethink to do instead.

// closedCall is one call a rethink closed: the round it may be made again
// from, and the label of the result on the record that already answers it.
type closedCall struct {
	reopensAt int
	label     string
}

// closeTheStalledCall closes the call the stall was about, whose mark the
// guard or the meter set beside the stall's text, for ClosedForRounds rounds
// from now. Any closure that has already run out is forgotten at the same
// time, so the map never holds more than the rethinks a task may make.
func (running *run) closeTheStalledCall(label string) {
	if running.stallMark == "" {
		return
	}
	if running.closedCalls == nil {
		running.closedCalls = map[string]closedCall{}
	}
	for mark, closed := range running.closedCalls {
		if running.roundsUsed >= closed.reopensAt {
			delete(running.closedCalls, mark)
		}
	}
	running.closedCalls[running.stallMark] = closedCall{reopensAt: running.roundsUsed + ClosedForRounds + 1, label: label}
	running.stallMark = ""
}

// theMarkToCloseAfterTheMeter is the mark the meter's rethink closes: the
// newest call of the round that is not the task's own test run, because the
// tests are the harness's own measure of progress and it reruns them after
// every change itself, so closing them would only hide the measure. A round
// of nothing but test runs closes nothing.
func (running *run) theMarkToCloseAfterTheMeter(calls []contract.ToolCall) string {
	for at := len(calls) - 1; at >= 0; at-- {
		call := calls[at]
		if call.Name == contract.ToolShell && running.testCommand != "" && fieldOfCall(call, "command") == running.testCommand {
			continue
		}
		return fingerprintOf(call)
	}
	return ""
}

// theClosedLine is what the model is told when it asks again for a call a
// rethink closed, and is empty when the call is open. It is read before the
// detector, so that a closed call asked for ten times is not a run of the
// same call.
func (running *run) theClosedLine(call contract.ToolCall) string {
	closed, held := running.closedCalls[fingerprintOf(call)]
	if !held || running.roundsUsed >= closed.reopensAt {
		return ""
	}
	if closed.label == "" {
		return "You already have that. Do the Next line of your rethink instead."
	}
	return fmt.Sprintf("You already have that; it is on the record as %s. Do the Next line of your rethink instead.", closed.label)
}
