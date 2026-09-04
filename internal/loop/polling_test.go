package loop_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestPollingALongCommandIsNotTheSameCallOverAndOver is the gate review's fifth
// finding. The shell tool hands back a process id that can be polled, and a
// poll is the same call with the same arguments every time, so the identical-call
// detector refused the third poll and ended the task on the fourth before the
// build had finished. Rule 4 of design section 3 is about the same call being
// run twice, and a call whose result came back different is not the same call.
func TestPollingALongCommandIsNotTheSameCallOverAndOver(t *testing.T) {
	polling := &pollingTool{answers: []string{
		"process p1 started: make build",
		"p1 is still running, 12 seconds in",
		"p1 is still running, 24 seconds in",
		"p1 is still running, 36 seconds in",
		"p1 exited 0: the build finished",
	}}
	built := newHarness(t, []testkit.Step{
		callStep("Nothing is built yet. I will start the build.",
			taskCall("c1t", `{"why":"the user wants the build","doneWhen":["the build finished"]}`),
			callFor("c1", "shell", `{"action":"start","command":"make build"}`)),
		callStep("The build is running. I will wait for it.", callFor("c2", "shell", `{"action":"poll","id":"p1"}`)),
		callStep("Still running. I will wait for it.", callFor("c3", "shell", `{"action":"poll","id":"p1"}`)),
		callStep("Still running. I will wait for it.", callFor("c4", "shell", `{"action":"poll","id":"p1"}`)),
		callStep("Still running. I will wait for it.", callFor("c5", "shell", `{"action":"poll","id":"p1"}`)),
		callStep("The build is done, so I will write that down.",
			taskCall("c6t", `{"doneWhen":[{"text":"the build finished","done":true,"resultId":"r6"}]}`)),
		answerStep("The build finished and exited zero."),
	}, polling)

	outcome := built.ask(t, "build it and tell me when it is done")

	if outcome.Status == contract.StatusStopped {
		t.Errorf("the task ended stopped on %q, and waiting for a build is not asking for the same thing over and over",
			outcome.StopLine)
	}
	if polling.calls != 5 {
		t.Errorf("the shell tool ran %d times, want all five, because every poll came back saying something different",
			polling.calls)
	}
	if outcome.Status != contract.StatusDone {
		t.Errorf("the task ended %q, want done, because the build finished", outcome.Status)
	}
}

// TestTheSameCallWithTheSameResultIsStillRefused proves the other half: a call
// that comes back with the very same result is the same call, and the third one
// is still refused.
func TestTheSameCallWithTheSameResultIsStillRefused(t *testing.T) {
	polling := &pollingTool{answers: []string{
		"p1 is still running",
		"p1 is still running",
		"p1 is still running",
		"p1 is still running",
	}}
	same := func(id string) testkit.Step {
		return callStep("I will wait for it.", callFor(id, "shell", `{"action":"poll","id":"p1"}`))
	}
	built := newHarness(t, []testkit.Step{same("c1"), same("c2"), same("c3"), same("c4")}, polling)

	outcome := built.ask(t, "wait for the build")

	if polling.calls != 2 {
		t.Errorf("the shell tool ran %d times, and only the first two identical calls with identical results are run",
			polling.calls)
	}
	if outcome.Status != contract.StatusStopped {
		t.Errorf("the task ended %q, want stopped, because the model would not stop asking", outcome.Status)
	}
}

// pollingTool answers with a different line every time, the way polling a long
// command does.
type pollingTool struct {
	answers []string
	calls   int
}

// Spec is what the model is told about the polling tool.
func (tool *pollingTool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name:        contract.ToolShell,
		Description: "A tool the test scripted, which answers the way polling a long command does.",
		Classes:     []contract.PermissionClass{contract.ClassExecute},
	}
}

// Run answers with the next line the test gave it, and with the last one over
// again once they run out.
func (tool *pollingTool) Run(_ context.Context, _ json.RawMessage) (contract.ToolOutput, error) {
	said := tool.answers[min(tool.calls, len(tool.answers)-1)]
	tool.calls++
	return contract.ToolOutput{Text: said}, nil
}
