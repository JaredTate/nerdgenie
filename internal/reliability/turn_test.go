package reliability_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/reliability"
	"github.com/JaredTate/coeus/internal/testkit"
)

// turnThatWaits is a turn which says when it started and then waits to be let
// go, so that a test can hold one turn open while a second message arrives.
type turnThatWaits struct {
	started   chan struct{}
	letItGo   chan struct{}
	timesRun  atomic.Int64
	insideNow atomic.Int64
}

// newTurnThatWaits returns a turn that has not started yet.
func newTurnThatWaits() *turnThatWaits {
	return &turnThatWaits{started: make(chan struct{}), letItGo: make(chan struct{})}
}

// run is what the guard calls as the turn itself.
func (turn *turnThatWaits) run(context.Context) error {
	if turn.timesRun.Add(1) == 1 {
		close(turn.started)
	}
	turn.insideNow.Add(1)
	defer turn.insideNow.Add(-1)
	<-turn.letItGo
	return nil
}

func TestTwoMessagesOnOneSessionRunOneTurnAtATime(t *testing.T) {
	clock := testkit.NewFakeClock(startOfTime)
	guard, _ := aGuardWithClock(t, testkit.NewTempHome(t), testkit.NewFakeStore(), clock)
	first := newTurnThatWaits()
	held := make(chan error, 1)
	go func() { held <- guard.RunTurn(context.Background(), "signal", first.run) }()
	<-first.started

	// The second message arrives while the first turn is still running. It
	// waits for the lease and the wait runs out, because a turn that runs
	// beside the one holding the lease writes the same record twice.
	secondRan := atomic.Bool{}
	refused := make(chan error, 1)
	go func() {
		refused <- guard.RunTurn(context.Background(), "signal", func(context.Context) error {
			secondRan.Store(true)
			return nil
		})
	}()
	waitForSleepers(t, clock, 2)
	clock.Advance(reliability.LeaseWait + time.Millisecond)

	select {
	case err := <-refused:
		if !errors.Is(err, reliability.ErrTurnInProgress) {
			t.Fatalf("the second turn on one session came back with %v, want it refused because a turn is running", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("the second turn on one session never came back")
	}
	if secondRan.Load() {
		t.Errorf("the second turn ran beside the first, which is two turns writing one record")
	}
	if first.insideNow.Load() != 1 {
		t.Errorf("%d turns are inside the session at once, want the one holding the lease", first.insideNow.Load())
	}

	close(first.letItGo)
	if err := <-held; err != nil {
		t.Errorf("the turn that held the lease came back with %v, want nothing", err)
	}
	if !guard.MayStartTask() {
		t.Errorf("the guard will start no task after the turn released its lease")
	}
}

func TestATurnOnAnotherSessionRunsBesideTheOneAlreadyRunning(t *testing.T) {
	clock := testkit.NewFakeClock(startOfTime)
	guard, _ := aGuardWithClock(t, testkit.NewTempHome(t), testkit.NewFakeStore(), clock)
	onSignal := newTurnThatWaits()
	go func() { _ = guard.RunTurn(context.Background(), "signal", onSignal.run) }()
	<-onSignal.started
	defer close(onSignal.letItGo)

	onTheTerminal := make(chan error, 1)
	go func() {
		onTheTerminal <- guard.RunTurn(context.Background(), "terminal", func(context.Context) error { return nil })
	}()

	select {
	case err := <-onTheTerminal:
		if err != nil {
			t.Fatalf("a turn on another session came back with %v, want it to run", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("a turn on another session waited for a turn it shares nothing with")
	}
}

func TestATurnIsStoppedWhenItRunsPastTheTurnLimit(t *testing.T) {
	clock := testkit.NewFakeClock(startOfTime)
	guard, _ := aGuardWithClock(t, testkit.NewTempHome(t), testkit.NewFakeStore(), clock)
	whyItStopped := make(chan error, 1)

	turn := make(chan error, 1)
	go func() {
		turn <- guard.RunTurn(context.Background(), "terminal", func(ctx context.Context) error {
			<-ctx.Done()
			whyItStopped <- context.Cause(ctx)
			return context.Cause(ctx)
		})
	}()
	waitForSleepers(t, clock, 1)
	clock.Advance(contract.DefaultConfig().Caps.TimePerTurn)

	select {
	case cause := <-whyItStopped:
		if !errors.Is(cause, reliability.ErrDeadlineExpired) {
			t.Fatalf("the turn was stopped because %v, want our own turn limit", cause)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("the turn ran past the %s a turn is given and was never stopped", contract.DefaultConfig().Caps.TimePerTurn)
	}
	if err := <-turn; !errors.Is(err, reliability.ErrDeadlineExpired) {
		t.Errorf("running the turn came back with %v, want the turn limit it ran past", err)
	}
}
