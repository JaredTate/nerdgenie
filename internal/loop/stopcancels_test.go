package loop_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/testkit"
)

// howLongAStopMayTake is what the person is promised when they press Escape: the
// call in flight is cancelled and the task ends, inside a second.
const howLongAStopMayTake = time.Second

// modelThatWaits is a model that says nothing at all until its call is
// cancelled, which is what a model that has gone quiet looks like from here.
type modelThatWaits struct {
	// calling is closed once the call is really in flight, so that a test does
	// not ask for a stop before there is anything to stop.
	calling chan struct{}
}

// Name is the alias the record and the cost line print.
func (waiting *modelThatWaits) Name() string { return "waiting" }

// ContextLength is a window big enough that nothing is ever cut short.
func (waiting *modelThatWaits) ContextLength() int { return 24000 }

// Send waits for the call's own context to be cancelled and never answers.
func (waiting *modelThatWaits) Send(ctx context.Context, _ contract.Request, _ func(delta string)) (contract.Reply, error) {
	select {
	case <-waiting.calling:
	default:
		close(waiting.calling)
	}
	<-ctx.Done()
	return contract.Reply{}, ctx.Err()
}

func TestStopCancelsTheModelCallInFlightAndSaysTheTaskWasStopped(t *testing.T) {
	waiting := &modelThatWaits{calling: make(chan struct{})}
	built := newHarness(t, nil)
	made, err := loop.New(built.optionsOver(waiting))
	if err != nil {
		t.Fatalf("cannot build a loop over a model that waits: %v", err)
	}

	answered := make(chan loop.Outcome, 1)
	go func() {
		outcome, err := made.Run(context.Background(), loop.Task{
			Message: contract.Inbound{Text: "think about this for a long time", Channel: "terminal"},
			Channel: built.channel,
		})
		if err != nil {
			t.Errorf("the run gave back an error rather than a stopped task: %v", err)
		}
		answered <- outcome
	}()

	<-waiting.calling
	asked := time.Now()
	made.Stop()

	select {
	case outcome := <-answered:
		if waited := time.Since(asked); waited > howLongAStopMayTake {
			t.Errorf("the stop took %s, and the person is promised an answer inside %s",
				waited.Round(time.Millisecond), howLongAStopMayTake)
		}
		if outcome.Status != contract.StatusStopped {
			t.Errorf("the task ended as %q, want %q", outcome.Status, contract.StatusStopped)
		}
		if !strings.Contains(strings.ToLower(outcome.Report), "stopped") {
			t.Errorf("the report is %q, and it does not say the task was stopped", outcome.Report)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the run never came back, so the stop never reached the model call in flight")
	}
}

func TestATaskSendsOneRecordLineWhenItStartsAndWhenItEnds(t *testing.T) {
	built := newHarness(t, []testkit.Step{{
		Text:   "DigiByte launched on 10 January 2014.",
		Finish: contract.FinishEnd,
	}})

	built.ask(t, "when did DigiByte launch?")

	written := strings.Join(built.recordLines(), "\n")
	if !strings.Contains(written, "started") {
		t.Errorf("no record line says a task started:\n%s", written)
	}
	if !strings.Contains(written, "when did DigiByte launch") {
		t.Errorf("no record line names what the task is about:\n%s", written)
	}
	if !strings.Contains(written, string(contract.StatusDone)) {
		t.Errorf("no record line says where the task ended:\n%s", written)
	}
}
