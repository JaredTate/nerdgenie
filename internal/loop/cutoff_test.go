package loop_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// The tests in this file are from a real run on 3 September 2026 at 16:19. A
// turn limit cut a task off, the loop's ending was written under the same
// cancelled context, and the log said "cannot mark task 4 as failed: cannot
// save checkpoint 39 ... context canceled". The task then stood at running for
// ever as far as the program could see, the next message started a fresh task,
// and the model told the person there was no active work. A task cut off for
// any reason is marked and reported under a short context of the ending's own.

// cuttingTool cancels the context the task runs under while it runs, the way a
// turn limit or a shutdown does, and answers as if nothing had happened.
type cuttingTool struct {
	cut  context.CancelFunc
	then func()
	used int
}

// Spec is what the model is told about the cutting tool.
func (tool *cuttingTool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name:        "read",
		Description: "A read that ends the turn it runs in, so that a cut-off task can be watched ending.",
		Classes:     []contract.PermissionClass{contract.ClassRead},
	}
}

// Run cuts the turn off, does whatever else the test asked, and answers.
func (tool *cuttingTool) Run(context.Context, json.RawMessage) (contract.ToolOutput, error) {
	tool.used++
	tool.cut()
	if tool.then != nil {
		tool.then()
	}
	return contract.ToolOutput{Text: "the notes"}, nil
}

// aTaskThatIsCutOff is one round of work and an answer the turn never lives to
// give.
func aTaskThatIsCutOff() []testkit.Step {
	return []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("This answer is never given, because the turn was cut off."),
	}
}

func TestATaskCutOffMidTurnIsStillMarkedAndReported(t *testing.T) {
	tool := &cuttingTool{}
	built := newHarness(t, aTaskThatIsCutOff(), tool)
	turn, cut := context.WithCancel(t.Context())
	defer cut()
	tool.cut = cut

	outcome, err := built.loop.Run(turn, built.task("read the notes"))
	if err != nil {
		t.Fatalf("a task cut off mid-turn came back as an error, and its ending is written under a context of its own: %v", err)
	}

	if outcome.Status != contract.StatusFailed {
		t.Errorf("the task ended %q, want failed, because the turn was cut off under it", outcome.Status)
	}
	held := built.held(t, outcome.TaskID)
	if held.Header.Status != contract.StatusFailed {
		t.Errorf("the record stands at %q, want failed: a task the program cannot see the end of is a task it has lost", held.Header.Status)
	}
	if !sentSomethingLike(built.channel.Sent(), "could not finish") {
		t.Errorf("the user was sent %v, want the report of a task that could not be finished", built.channel.Sent())
	}
	if calls := len(built.model.Requests()); calls != 1 {
		t.Errorf("the model was called %d times, want one: a cut-off task asks the model nothing more", calls)
	}
}

// stuckLog is an event log that, once told to, answers every write by waiting
// until the caller gives up, the way a database that has stopped answering
// does.
type stuckLog struct {
	contract.Store
	stuck atomic.Bool
}

// Append waits for the caller to give up once the log is stuck.
func (log *stuckLog) Append(ctx context.Context, event contract.Event) (int64, error) {
	if !log.stuck.Load() {
		return log.Store.Append(ctx, event)
	}
	<-ctx.Done()
	return 0, ctx.Err()
}

// TestTheEndingOfACutOffTaskIsBoundedOnTheClock proves the context the ending
// writes under is short-lived: a log that will not answer holds the ending for
// WrapUpTime on the harness's clock and no longer, and the task comes back with
// the reason rather than hanging the loop.
func TestTheEndingOfACutOffTaskIsBoundedOnTheClock(t *testing.T) {
	log := &stuckLog{}
	tool := &cuttingTool{then: func() { log.stuck.Store(true) }}
	built := newHarness(t, aTaskThatIsCutOff(), tool)
	log.Store = built.store
	options := built.options()
	options.Store = log
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build the loop over the stuck log: %v", err)
	}
	turn, cut := context.WithCancel(t.Context())
	defer cut()
	tool.cut = cut

	finished := make(chan error, 1)
	go func() {
		_, err := made.Run(turn, built.task("read the notes"))
		finished <- err
	}()
	waitForASleeper(t, built.clock)
	built.clock.Advance(loop.WrapUpTime)

	select {
	case err := <-finished:
		if err == nil || !strings.Contains(err.Error(), "cannot mark task") {
			t.Errorf("the task came back with %v, want the ending's own complaint that the record could not be marked", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("the ending of a cut-off task is still waiting on a log that will not answer after %s of its own time", loop.WrapUpTime)
	}
}

// waitForASleeper waits until one caller is asleep on the fake clock, so that
// the test never moves the clock past a wait that has not begun.
func waitForASleeper(t *testing.T, clock *testkit.FakeClock) {
	t.Helper()
	for range 5000 {
		if clock.Sleepers() >= 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("nothing is waiting on the clock after five seconds, and the ending should be")
}
