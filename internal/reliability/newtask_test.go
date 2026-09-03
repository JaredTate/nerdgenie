package reliability_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/reliability"
	"github.com/JaredTate/coeus/internal/testkit"
)

// tripTheBreaker starts the guard often enough, each time on a home whose
// sentinel was left behind, that the crash-loop breaker trips.
func tripTheBreaker(t *testing.T, home contract.Home, guard *reliability.Guard) reliability.Startup {
	t.Helper()
	found := reliability.Startup{}
	for range reliability.RestartLimit + 1 {
		var err error
		aNewLife(t, home)
		if found, err = guard.Start(context.Background()); err != nil {
			t.Fatalf("starting failed: %v", err)
		}
	}
	if !found.BreakerTripped {
		t.Fatalf("the breaker has not tripped after %d starts that followed a crash", reliability.RestartLimit+1)
	}
	return found
}

func TestATrippedBreakerSaysInWordsWhyNoTaskIsStarted(t *testing.T) {
	home := testkit.NewTempHome(t)
	guard, _ := aGuard(t, home, testkit.NewFakeStore())
	tripTheBreaker(t, home, guard)

	why := guard.WhyNoNewTask()

	if why == "" {
		t.Fatalf("the guard gives no reason for starting no task while the breaker is tripped")
	}
	if !strings.Contains(why, "task") {
		t.Errorf("the reason reads %q, want a sentence saying that no task will be started", why)
	}
	if guard.MayStartTask() {
		t.Errorf("a task would be started although the guard has a reason not to: %q", why)
	}
}

func TestATrippedBreakerKeepsTellingSystemdThatTheProgramIsAlive(t *testing.T) {
	listening := aNotifySocket(t, true)
	clock := testkit.NewFakeClock(startOfTime)
	home := testkit.NewTempHome(t)
	guard, _ := aGuardWithClock(t, home, testkit.NewFakeStore(), clock)
	tripTheBreaker(t, home, guard)

	feeding, stopFeeding := context.WithCancel(context.Background())
	defer stopFeeding()
	go func() { _ = guard.FeedWatchdog(feeding) }()

	// A tripped breaker means the agent answers the user and starts no task. If
	// the feed stopped here, systemd would kill the program and start it again,
	// the kill would leave the sentinel behind, the next start would count as
	// another crash, and the task that was killing the program would be picked
	// up once more: the breaker would make the loop worse rather than better.
	arrived := ""
	for range 100 {
		clock.Advance(watchdogInterval / 2)
		if arrived = whatArrived(t, listening, 50*time.Millisecond); arrived != "" {
			break
		}
	}
	if arrived != "WATCHDOG=1" {
		t.Errorf("systemd was told %q by an agent whose breaker has tripped, want WATCHDOG=1 so that it is left serving", arrived)
	}
	if guard.Healthy() {
		t.Errorf("a guard whose breaker has tripped calls itself healthy, and a screen would show nothing wrong")
	}
}

func TestADrainSaysWhyNoNewTaskIsStartedAndEndsWhenItIsCancelled(t *testing.T) {
	home := testkit.NewTempHome(t)
	guard, _ := aGuard(t, home, testkit.NewFakeStore())
	aNewLife(t, home)
	if _, err := guard.Start(context.Background()); err != nil {
		t.Fatalf("starting failed: %v", err)
	}

	if err := guard.Drain().Request("an update is being installed"); err != nil {
		t.Fatalf("asking for a drain failed: %v", err)
	}

	why := guard.WhyNoNewTask()
	if !strings.Contains(why, "an update is being installed") {
		t.Errorf("the reason reads %q, want the reason the drain was asked for", why)
	}
	if guard.MayStartTask() {
		t.Errorf("a new task would be started while the agent is draining")
	}
	if !guard.Healthy() {
		t.Errorf("a draining agent is called unhealthy, and it is finishing its work properly")
	}

	if err := guard.Drain().Cancel(); err != nil {
		t.Fatalf("cancelling the drain failed: %v", err)
	}
	if why := guard.WhyNoNewTask(); why != "" {
		t.Errorf("the guard still refuses a task after the drain was cancelled: %q", why)
	}
}

func TestADrainAskedForWithNoReasonStillSaysWhatIsHappening(t *testing.T) {
	guard, _ := aGuard(t, testkit.NewTempHome(t), testkit.NewFakeStore())
	if err := guard.Drain().Request(""); err != nil {
		t.Fatalf("asking for a drain failed: %v", err)
	}

	why := guard.WhyNoNewTask()

	if why == "" {
		t.Fatalf("a drain with no reason given stops tasks without a word to the user")
	}
	if !strings.Contains(why, "task") {
		t.Errorf("the reason reads %q, want a sentence saying that no new task will be started", why)
	}
}
