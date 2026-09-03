package loop_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/testkit"
)

// TestAToolPastItsDeadlineIsStoppedAndTheModelIsTold proves the seam the
// reliability guard hangs its deadline on: every tool call runs under the
// deadline the guard gives it, a tool that is still running when it passes is
// stopped, and the model is told in words it can act on rather than left waiting.
func TestAToolPastItsDeadlineIsStoppedAndTheModelIsTold(t *testing.T) {
	stubborn := &stubbornTool{released: make(chan struct{})}
	defer close(stubborn.released)
	built := newHarness(t, []testkit.Step{
		callStep("Nothing is read yet. I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("The read did not finish. Shall I try a smaller one?"),
	}, stubborn)

	options := built.options()
	options.ToolDeadline = aDeadlineThatHasPassed
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop with a tool deadline: %v", err)
	}

	outcome, err := made.Run(t.Context(), built.task("read the notes"))
	if err != nil {
		t.Fatalf("the loop could not run the task: %v", err)
	}

	if outcome.Status != contract.StatusWaiting {
		t.Errorf("the task ended %q, and a tool that ran out of time is a result the model acts on", outcome.Status)
	}
	said := requestsJoined(built.model.Requests())
	if !strings.Contains(said, "did not finish before the deadline") {
		t.Error("the model was never told that the tool it asked for was stopped at its deadline")
	}
	if strings.Contains(said, "context deadline exceeded") {
		t.Error("the model was told the Go error and not what went wrong and what to do about it")
	}
}

// TestWithNoDeadlineNothingChanges proves the seam is off when nobody sets it,
// which is what every test and every machine without the reliability guard runs
// with.
func TestWithNoDeadlineNothingChanges(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("Nothing is read yet. I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("The notes are read. Shall I go on?"),
	}, scriptedTool("read", "the notes"))

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusWaiting {
		t.Errorf("the task ended %q, want waiting, because the model asked a question", outcome.Status)
	}
	if strings.Contains(requestsJoined(built.model.Requests()), "deadline") {
		t.Error("a loop with no tool deadline told the model about one")
	}
}

// aDeadlineThatHasPassed is a tool deadline that is already up when the call
// starts, which is what the reliability guard hands over when a task has spent
// the time it was given.
func aDeadlineThatHasPassed(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithDeadline(ctx, time.Now().Add(-time.Second))
}

// stubbornTool never comes back on its own, the way a tool waiting on a machine
// that has stopped answering does. It returns only when the test releases it.
type stubbornTool struct {
	released chan struct{}
}

// Spec is what the model is told about the stubborn tool.
func (tool *stubbornTool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name:        "read",
		Description: "A tool the test made stubborn, so that a deadline can be seen stopping one.",
		Classes:     []contract.PermissionClass{contract.ClassRead},
	}
}

// Run waits until the test lets it go, whatever its context says.
func (tool *stubbornTool) Run(_ context.Context, _ json.RawMessage) (contract.ToolOutput, error) {
	<-tool.released
	return contract.ToolOutput{Text: "the notes, at last"}, nil
}
