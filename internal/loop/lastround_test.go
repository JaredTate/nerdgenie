// The tests for a message that arrives during a task's last round. Messages
// delivered mid-task were read only after the tool calls of a round, so one
// that arrived while the model was writing its final answer, with no tool
// call after it, stayed queued and was read by the next task as a correction
// to work it had nothing to do with. A message that arrives before the task
// ends is the task's to read: a correction sends the model back for one more
// round with the person's words in front of it, and a stop stops the task.
package loop_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// modelThatDeliversOnOneCall plays a script, and on one of its calls, counted
// from one, first hands the loop a message from the person, the way one that
// arrives while the model is writing its reply does.
type modelThatDeliversOnOneCall struct {
	inner     *testkit.FakeModel
	deliverOn int
	deliver   func()
	guard     sync.Mutex
	calls     int
}

// Name is the alias the record and the cost line print.
func (delivering *modelThatDeliversOnOneCall) Name() string { return delivering.inner.Name() }

// ContextLength is the window the scripted model reports.
func (delivering *modelThatDeliversOnOneCall) ContextLength() int {
	return delivering.inner.ContextLength()
}

// Send delivers the message on the one call and then answers from the script.
func (delivering *modelThatDeliversOnOneCall) Send(ctx context.Context, request contract.Request, onDelta func(delta string)) (contract.Reply, error) {
	delivering.guard.Lock()
	delivering.calls++
	call := delivering.calls
	delivering.guard.Unlock()
	if call == delivering.deliverOn {
		delivering.deliver()
	}
	return delivering.inner.Send(ctx, request, onDelta)
}

// callsMade is how many times the model was called.
func (delivering *modelThatDeliversOnOneCall) callsMade() int {
	delivering.guard.Lock()
	defer delivering.guard.Unlock()
	return delivering.calls
}

// aLoopWhoseModelDeliversOnCall builds a loop over the harness's script whose
// model hands the loop the message given on the call given.
func aLoopWhoseModelDeliversOnCall(t *testing.T, built *harness, call int, said string) (*loop.Loop, *modelThatDeliversOnOneCall) {
	t.Helper()
	delivering := &modelThatDeliversOnOneCall{inner: built.model, deliverOn: call}
	made, err := loop.New(built.optionsOver(delivering))
	if err != nil {
		t.Fatalf("cannot build a loop over a model that delivers: %v", err)
	}
	delivering.deliver = func() {
		if err := made.Deliver(contract.Inbound{Text: said, Channel: "terminal"}); err != nil {
			t.Errorf("cannot hand the loop the message %q: %v", said, err)
		}
	}
	return made, delivering
}

// TestACorrectionDeliveredDuringTheLastRoundIsReadBeforeTheTaskEnds proves
// the person's words reach the task they were sent to: the model is called
// once more with the correction in front of it, the correction is in the
// record, and the report is the reply that read it. A task that took a
// correction is reviewed, which is one more call. It also proves nothing is
// left queued for the next task.
func TestACorrectionDeliveredDuringTheLastRoundIsReadBeforeTheTaskEnds(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("The post leads with the features. What changed: one draft. What is left: nothing."),
		answerStep("The post leads with the date now. What changed: one draft. What is left: nothing."),
		aReviewReply("Lead with the date."),
		answerStep("The second task is done."),
	}, scriptedTool("read", "the notes"))
	made, delivering := aLoopWhoseModelDeliversOnCall(t, built, 2, "no, lead with the date, not the features")

	outcome, err := made.Run(t.Context(), built.task("post the anniversary tweet"))
	if err != nil {
		t.Fatalf("the task did not finish: %v", err)
	}

	if outcome.Status != contract.StatusDone || !strings.HasPrefix(outcome.Report, "The post leads with the date now.") {
		t.Errorf("the task ended as %+v, want it done on the reply that read the correction", outcome)
	}
	if calls := delivering.callsMade(); calls != 4 {
		t.Errorf("the model was called %d times, want 4: the tool round, the reply the correction arrived during, the one more round that read it, and the review a correction earns", calls)
	}
	held := built.held(t, "1")
	if len(held.Rules.Corrections) != 1 || held.Rules.Corrections[0].Text != "no, lead with the date, not the features" {
		t.Errorf("the record's corrections are %+v, want the person's words, word for word", held.Rules.Corrections)
	}
	second, err := made.Run(t.Context(), built.task("do the second thing"))
	if err != nil {
		t.Fatalf("the next task did not finish: %v", err)
	}
	if second.Report != "The second task is done." {
		t.Errorf("the next task ended as %+v, and it must not read a message meant for the task before it", second)
	}
	if calls := delivering.callsMade(); calls != 5 {
		t.Errorf("the model was called %d times over both tasks, want 5: nothing was left queued to send the second task round again", calls)
	}
}

// TestAStopDeliveredDuringTheLastRoundStopsTheTask proves the other kind of
// message: the person said stop while the model was writing its answer, so
// the task ends stopped rather than done.
func TestAStopDeliveredDuringTheLastRoundStopsTheTask(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("The post is up. What changed: one post. What is left: nothing."),
	}, scriptedTool("read", "the notes"))
	made, delivering := aLoopWhoseModelDeliversOnCall(t, built, 2, "stop")

	outcome, err := made.Run(t.Context(), built.task("post the anniversary tweet"))
	if err != nil {
		t.Fatalf("the task did not end cleanly: %v", err)
	}

	if outcome.Status != contract.StatusStopped {
		t.Errorf("the task ended %q, want %q, because the person said stop before it ended", outcome.Status, contract.StatusStopped)
	}
	if calls := delivering.callsMade(); calls != 2 {
		t.Errorf("the model was called %d times, want 2: a stop asks the model nothing more", calls)
	}
}
