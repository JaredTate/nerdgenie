package loop

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The tests run themselves after a change. On the fifth game build the model
// never put two calls in one reply, so every test run was a round of its own,
// eighteen percent of the run, sixteen seconds each. Once the harness has seen
// the model run the tests, it runs the same command after every write or edit
// and puts the tests line on the change's own result, so the next round can
// act on it. The run is the model's command word for word, through the same
// shell tool, under the tool's own ten-second yield: a suite that takes longer
// is left to the model. A failure line is never written for a run the model
// did not ask for; only the situation's tests line and the meter move.

// TheTestsAfterAChange opens the line the tests' state rides on.
const TheTestsAfterAChange = "tests after this change: "

// rememberTheTestCommand keeps the command of a shell call whose output read
// as a test run, so that it can be run again after a change.
func (running *run) rememberTheTestCommand(call contract.ToolCall, text string) {
	if call.Name != contract.ToolShell {
		return
	}
	if _, found := testStateIn(text); !found {
		return
	}
	if command := fieldOfCall(call, "command"); command != "" {
		running.testCommand = command
	}
}

// runTheTestsAfter runs the remembered test command after a write or an edit
// that worked, and hands back the line to put on the change's result, or
// nothing when there is no command yet, no shell tool, or the run did not read
// as a test run within the yield.
func (running *run) runTheTestsAfter(ctx context.Context, call contract.ToolCall, failed bool) string {
	if failed || running.testCommand == "" || (call.Name != contract.ToolWrite && call.Name != contract.ToolEdit) {
		return ""
	}
	shell, found := running.tools().Lookup(contract.ToolShell)
	if !found {
		return ""
	}
	arguments, err := json.Marshal(map[string]string{"command": running.testCommand})
	if err != nil {
		return ""
	}
	output, err := running.underTheTimeLimit(ctx, shell, contract.ToolCall{ID: call.ID + "-tests", Name: contract.ToolShell, Input: arguments})
	if err != nil {
		return ""
	}
	state, found := testStateIn(output.Text)
	if !found {
		return ""
	}
	running.testsFact = state.line()
	running.noteTheTestsImprovedOrNot(state)
	return TheTestsAfterAChange + strings.TrimPrefix(state.line(), "tests: ")
}
