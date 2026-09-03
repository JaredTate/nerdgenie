package reliability_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/reliability"
	"github.com/JaredTate/coeus/internal/testkit"
)

// takenLease is what a goroutine waiting for a lease hands back to the test.
type takenLease struct {
	lease *reliability.Lease
	err   error
}

// acquireInTheBackground asks for a lease from a goroutine and sends back what
// it got, so that the test can move the clock while the caller waits.
func acquireInTheBackground(leases *reliability.Leases, session string) <-chan takenLease {
	answers := make(chan takenLease, 1)
	go func() {
		lease, err := leases.Acquire(context.Background(), session)
		answers <- takenLease{lease: lease, err: err}
	}()
	return answers
}

// answerWithin reads the goroutine's answer, or fails the test when it never
// comes.
func answerWithin(t *testing.T, answers <-chan takenLease) takenLease {
	t.Helper()
	select {
	case answer := <-answers:
		return answer
	case <-time.After(2 * time.Second):
		t.Fatalf("the caller waiting for the lease never came back")
		return takenLease{}
	}
}

func TestOneTurnAtATimeHoldsTheLeaseForItsSession(t *testing.T) {
	leases := reliability.NewLeases(testkit.NewFakeClock(startOfTime))

	first, err := leases.Acquire(context.Background(), "signal")
	if err != nil {
		t.Fatalf("taking the lease on a free session failed: %v", err)
	}
	if leases.Held() != 1 {
		t.Errorf("%d leases are held after one turn started, want 1", leases.Held())
	}

	onAnother, err := leases.Acquire(context.Background(), "terminal")
	if err != nil {
		t.Fatalf("taking the lease on another session failed: %v", err)
	}
	onAnother.Release()

	first.Release()
	if leases.Held() != 0 {
		t.Errorf("%d leases are held after every turn finished, want none", leases.Held())
	}

	again, err := leases.Acquire(context.Background(), "signal")
	if err != nil {
		t.Fatalf("taking the lease again after it was released failed: %v", err)
	}
	again.Release()
}

func TestASecondTurnWaitsFiveSecondsAndIsThenRejected(t *testing.T) {
	clock := testkit.NewFakeClock(startOfTime)
	leases := reliability.NewLeases(clock)
	held, err := leases.Acquire(context.Background(), "signal")
	if err != nil {
		t.Fatalf("taking the first lease failed: %v", err)
	}
	defer held.Release()

	answers := acquireInTheBackground(leases, "signal")
	waitForSleepers(t, clock, 1)

	clock.Advance(reliability.LeaseWait - time.Millisecond)
	waitForSleepers(t, clock, 1)
	select {
	case answer := <-answers:
		t.Fatalf("the second turn was answered with %v before it had waited %s", answer.err, reliability.LeaseWait)
	default:
	}

	clock.Advance(2 * time.Millisecond)
	answer := answerWithin(t, answers)

	if !errors.Is(answer.err, reliability.ErrTurnInProgress) {
		t.Fatalf("the second turn came back with %v, want it rejected because a turn is running", answer.err)
	}
	if answer.lease != nil {
		t.Errorf("the second turn was given a lease as well as a rejection")
	}
	if !strings.Contains(answer.err.Error(), "signal") {
		t.Errorf("the rejection does not name the session it was refused on: %v", answer.err)
	}
}

func TestASecondTurnTakesTheLeaseAsSoonAsTheFirstReleasesIt(t *testing.T) {
	clock := testkit.NewFakeClock(startOfTime)
	leases := reliability.NewLeases(clock)
	first, err := leases.Acquire(context.Background(), "signal")
	if err != nil {
		t.Fatalf("taking the first lease failed: %v", err)
	}

	answers := acquireInTheBackground(leases, "signal")
	waitForSleepers(t, clock, 1)
	first.Release()
	clock.Advance(time.Second)

	answer := answerWithin(t, answers)
	if answer.err != nil {
		t.Fatalf("the waiting turn was rejected although the lease was free: %v", answer.err)
	}
	answer.lease.Release()
	if leases.Held() != 0 {
		t.Errorf("%d leases are held after the waiting turn released, want none", leases.Held())
	}
}

func TestGivingUpOnAWaitLeavesTheHolderAlone(t *testing.T) {
	clock := testkit.NewFakeClock(startOfTime)
	leases := reliability.NewLeases(clock)
	held, err := leases.Acquire(context.Background(), "signal")
	if err != nil {
		t.Fatalf("taking the first lease failed: %v", err)
	}
	defer held.Release()
	givenUp, stopWaiting := context.WithCancel(context.Background())

	answers := make(chan takenLease, 1)
	go func() {
		lease, err := leases.Acquire(givenUp, "signal")
		answers <- takenLease{lease: lease, err: err}
	}()
	waitForSleepers(t, clock, 1)
	stopWaiting()

	answer := answerWithin(t, answers)
	if !errors.Is(answer.err, context.Canceled) {
		t.Errorf("a caller that gave up came back with %v, want the context's own error", answer.err)
	}
	if leases.Held() != 1 {
		t.Errorf("%d leases are held after a waiter gave up, want the holder's one", leases.Held())
	}
}

func TestReleasingTwiceOrLateNeverFreesSomebodyElsesTurn(t *testing.T) {
	leases := reliability.NewLeases(testkit.NewFakeClock(startOfTime))
	first, err := leases.Acquire(context.Background(), "signal")
	if err != nil {
		t.Fatalf("taking the first lease failed: %v", err)
	}
	first.Release()
	first.Release()

	second, err := leases.Acquire(context.Background(), "signal")
	if err != nil {
		t.Fatalf("taking the lease after the first turn finished failed: %v", err)
	}

	first.Release()

	if leases.Held() != 1 {
		t.Errorf("a stale release freed a newer turn's lease, and %d are held", leases.Held())
	}
	second.Release()
}

func TestALeaseIsRefusedWithoutASession(t *testing.T) {
	leases := reliability.NewLeases(testkit.NewFakeClock(startOfTime))

	lease, err := leases.Acquire(context.Background(), "")
	if err == nil {
		lease.Release()
		t.Fatalf("a lease was given out for no session at all")
	}
	if !strings.Contains(err.Error(), "session") {
		t.Errorf("the failure does not say what was missing: %v", err)
	}
}

func TestThereIsALimitOnHowManySessionsHoldALeaseAtOnce(t *testing.T) {
	leases := reliability.NewLeases(testkit.NewFakeClock(startOfTime))
	for held := range reliability.MaxLeases {
		if _, err := leases.Acquire(context.Background(), sessionNumber(held)); err != nil {
			t.Fatalf("taking lease %d failed: %v", held, err)
		}
	}

	_, err := leases.Acquire(context.Background(), "one too many")
	if err == nil {
		t.Fatalf("the registry handed out more than the %d leases it caps itself at", reliability.MaxLeases)
	}
	if !strings.Contains(err.Error(), "turn") {
		t.Errorf("the failure does not say what could not be started: %v", err)
	}
}

// sessionNumber names one of the many sessions the cap test opens.
func sessionNumber(number int) string {
	return "session-" + strconv.Itoa(number)
}
