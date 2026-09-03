package reliability_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/reliability"
	"github.com/JaredTate/coeus/internal/testkit"
)

// aGuard builds the whole reliability layer over a home, a log, and a sender
// that writes down what the user was told. Starting a second guard on the same
// home and the same log is what a restart looks like.
func aGuard(t *testing.T, home contract.Home, store *testkit.FakeStore) (*reliability.Guard, *sender) {
	t.Helper()
	told := &sender{}
	guard, err := reliability.New(reliability.Settings{
		Home:  home,
		Clock: testkit.NewFakeClock(startOfTime),
		Store: store,
		Caps:  contract.DefaultConfig().Caps,
		Send:  told.send,
	})
	if err != nil {
		t.Fatalf("building the guard failed: %v", err)
	}
	return guard, told
}

func TestAGuardNeedsAHomeAClockALogAndAWayOfSending(t *testing.T) {
	home := testkit.NewTempHome(t)
	whole := reliability.Settings{
		Home:  home,
		Clock: testkit.NewFakeClock(startOfTime),
		Store: testkit.NewFakeStore(),
		Caps:  contract.DefaultConfig().Caps,
		Send:  (&sender{}).send,
	}
	for _, one := range []struct {
		missing  string
		settings reliability.Settings
	}{
		{"clock", reliability.Settings{Home: home, Store: whole.Store, Send: whole.Send}},
		{"log", reliability.Settings{Home: home, Clock: whole.Clock, Send: whole.Send}},
		{"sender", reliability.Settings{Home: home, Clock: whole.Clock, Store: whole.Store}},
	} {
		t.Run(one.missing, func(t *testing.T) {
			if _, err := reliability.New(one.settings); err == nil {
				t.Errorf("a guard was built with no %s", one.missing)
			}
		})
	}
	if _, err := reliability.New(whole); err != nil {
		t.Errorf("building a guard with everything it needs failed: %v", err)
	}
}

func TestTheFirstStartOnAFreshHomeFindsNothingWrong(t *testing.T) {
	guard, told := aGuard(t, testkit.NewTempHome(t), testkit.NewFakeStore())

	found, err := guard.Start(context.Background())
	if err != nil {
		t.Fatalf("starting failed: %v", err)
	}

	if found.UncleanExit || found.BreakerTripped || found.DatabaseMovedTo != "" {
		t.Errorf("a fresh home came back as %+v, want nothing wrong", found)
	}
	if !guard.MayStartTask() {
		t.Errorf("a fresh home may not start a task")
	}
	if !guard.Healthy() {
		t.Errorf("a fresh home is called unhealthy, so systemd would keep restarting it")
	}
	if len(told.sent) != 0 {
		t.Errorf("the user was told %q about a start with nothing wrong", told.sent)
	}
}

func TestAnUncleanExitIsSeenAndTheRepliesThatNeverArrivedAreSentAgain(t *testing.T) {
	home := testkit.NewTempHome(t)
	store := testkit.NewFakeStore()
	guard, _ := aGuard(t, home, store)
	if _, err := guard.Start(context.Background()); err != nil {
		t.Fatalf("the first start failed: %v", err)
	}
	if _, err := guard.Ledger().Record(context.Background(), "17", "signal", "the post is up"); err != nil {
		t.Fatalf("writing the reply down failed: %v", err)
	}

	// The program is killed here: nothing marks the reply delivered and nothing
	// takes the sentinel away, which is what the next start finds.
	afterTheCrash, told := aGuard(t, home, store)

	found, err := afterTheCrash.Start(context.Background())
	if err != nil {
		t.Fatalf("the start after the crash failed: %v", err)
	}

	if !found.UncleanExit {
		t.Errorf("the start after a crash was called clean")
	}
	if found.RepliesSentAgain != 1 {
		t.Fatalf("%d replies were sent again, want the one that never arrived", found.RepliesSentAgain)
	}
	if len(told.sent) != 1 || !strings.HasPrefix(told.sent[0].text, reliability.DuplicateMarker) {
		t.Errorf("the reply was sent again as %q, want it marked as a possible duplicate", told.sent)
	}
	if told.sent[0].channel != "signal" {
		t.Errorf("the reply was sent again on %q rather than on the channel it was written for", told.sent[0].channel)
	}
}

func TestEnoughCrashesInARowStopTheGuardFromStartingATask(t *testing.T) {
	home := testkit.NewTempHome(t)
	store := testkit.NewFakeStore()
	var found reliability.Startup
	var guard *reliability.Guard
	var told *sender

	// The first start is a clean one, and every start after it finds the
	// sentinel of the life that was killed, so it takes one more start than the
	// limit to trip the breaker.
	for range reliability.RestartLimit + 1 {
		var err error
		guard, told = aGuard(t, home, store)
		if found, err = guard.Start(context.Background()); err != nil {
			t.Fatalf("starting failed: %v", err)
		}
	}

	if !found.BreakerTripped {
		t.Fatalf("the breaker has not tripped after %d crashes in a row", reliability.RestartLimit)
	}
	if guard.MayStartTask() {
		t.Errorf("a guard whose breaker has tripped would still start a task")
	}
	if guard.Healthy() {
		t.Errorf("a guard whose breaker has tripped still says it is healthy, so systemd would never restart it")
	}
	if len(told.sent) != 1 || !strings.Contains(told.sent[0].text, "task") {
		t.Errorf("the user was told %q, want one message saying that no task will be started", told.sent)
	}
}

func TestTheUserIsToldOnceThatTheBreakerHasTripped(t *testing.T) {
	home := testkit.NewTempHome(t)
	store := testkit.NewFakeStore()
	said := []sentReply{}

	for range reliability.RestartLimit + 2 {
		guard, told := aGuard(t, home, store)
		if _, err := guard.Start(context.Background()); err != nil {
			t.Fatalf("starting failed: %v", err)
		}
		said = append(said, told.sent...)
	}

	if len(said) != 1 {
		t.Fatalf("the user was told %d times that the breaker tripped, want once: %q", len(said), said)
	}
	if !strings.Contains(said[0].text, "task") {
		t.Errorf("the message about the crash loop does not say what will not happen: %q", said[0].text)
	}
}

func TestADrainStopsNewTasksWithoutStoppingTheProgram(t *testing.T) {
	guard, _ := aGuard(t, testkit.NewTempHome(t), testkit.NewFakeStore())
	if _, err := guard.Start(context.Background()); err != nil {
		t.Fatalf("starting failed: %v", err)
	}

	if err := guard.Drain().Request("an update is being installed"); err != nil {
		t.Fatalf("asking for a drain failed: %v", err)
	}

	if guard.MayStartTask() {
		t.Errorf("a task would be started while the agent is draining")
	}
	if !guard.Healthy() {
		t.Errorf("a draining agent is called unhealthy, and it is finishing its work properly")
	}
}

func TestACleanStopLeavesNothingBehindForTheNextStart(t *testing.T) {
	home := testkit.NewTempHome(t)
	store := testkit.NewFakeStore()
	guard, _ := aGuard(t, home, store)
	if _, err := guard.Start(context.Background()); err != nil {
		t.Fatalf("starting failed: %v", err)
	}

	if err := guard.Stop(); err != nil {
		t.Fatalf("stopping failed: %v", err)
	}

	next, _ := aGuard(t, home, store)
	found, err := next.Start(context.Background())
	if err != nil {
		t.Fatalf("the start after a clean stop failed: %v", err)
	}
	if found.UncleanExit {
		t.Errorf("the start after a clean stop was called unclean")
	}
}

func TestTheGuardHandsTheLoopItsLeaseAndItsTwoDeadlines(t *testing.T) {
	guard, _ := aGuard(t, testkit.NewTempHome(t), testkit.NewFakeStore())

	lease, err := guard.AcquireTurn(context.Background(), "signal")
	if err != nil {
		t.Fatalf("taking the turn lease failed: %v", err)
	}
	defer lease.Release()

	if want := contract.DefaultConfig().Caps.TimePerTurn; guard.TurnDeadline().Remaining() != want {
		t.Errorf("the turn deadline is %s, want the %s the caps give a turn", guard.TurnDeadline().Remaining(), want)
	}
	if want := contract.DefaultConfig().Caps.TimePerTool; guard.ToolDeadline().Remaining() != want {
		t.Errorf("the tool deadline is %s, want the %s the caps give a tool", guard.ToolDeadline().Remaining(), want)
	}
	if guard.Ledger() == nil {
		t.Errorf("the guard hands the loop no ledger, so replies would be sent without being written down")
	}
}

func TestTheWatchdogFeedIsStartedThroughTheGuard(t *testing.T) {
	aNotifySocket(t, false)
	guard, _ := aGuard(t, testkit.NewTempHome(t), testkit.NewFakeStore())

	if err := guard.Ready(); err != nil {
		t.Errorf("saying the program is ready failed: %v", err)
	}
	if err := guard.FeedWatchdog(context.Background()); err != nil {
		t.Errorf("feeding a watchdog that systemd never asked for failed: %v", err)
	}
}
