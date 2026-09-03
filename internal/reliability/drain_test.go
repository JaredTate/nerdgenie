package reliability_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/reliability"
	"github.com/JaredTate/coeus/internal/testkit"
)

// aDrain builds a drain marker on a temporary home and the clock the test moves.
func aDrain(t *testing.T) (*reliability.Drain, *testkit.FakeClock, contract.Home) {
	t.Helper()
	home := testkit.NewTempHome(t)
	clock := testkit.NewFakeClock(startOfTime)
	return reliability.NewDrain(home, clock), clock, home
}

// drainMarkerPath is where the marker file lives under a home.
func drainMarkerPath(home contract.Home) string {
	return filepath.Join(home.RunFolder(), reliability.DrainFileName)
}

func TestNoMarkerMeansTheLoopTakesNewTasks(t *testing.T) {
	drain, _, _ := aDrain(t)

	if drain.Requested() {
		t.Errorf("a home with no marker is draining, so the loop would take no work")
	}
}

func TestAskingForADrainStopsNewTasksAndCancellingStartsThemAgain(t *testing.T) {
	drain, _, home := aDrain(t)

	if err := drain.Request("an update is being installed"); err != nil {
		t.Fatalf("asking for a drain failed: %v", err)
	}
	if !drain.Requested() {
		t.Fatalf("the drain was asked for and the loop would still take new work")
	}
	written, err := os.ReadFile(drainMarkerPath(home))
	if err != nil {
		t.Fatalf("the marker was not written: %v", err)
	}
	if len(written) == 0 {
		t.Errorf("the marker is empty, so nobody finding it could say why it is there")
	}

	if err := drain.Cancel(); err != nil {
		t.Fatalf("cancelling the drain failed: %v", err)
	}
	if drain.Requested() {
		t.Errorf("the drain is still on after it was cancelled")
	}
	if err := drain.Cancel(); err != nil {
		t.Errorf("cancelling a drain that was never asked for failed: %v", err)
	}
}

func TestADrainMarkerRunsOutAfterThirtyMinutes(t *testing.T) {
	drain, clock, _ := aDrain(t)
	if err := drain.Request("an update is being installed"); err != nil {
		t.Fatalf("asking for a drain failed: %v", err)
	}

	clock.Advance(reliability.DrainExpiry - time.Minute)
	if !drain.Requested() {
		t.Errorf("the drain ran out before %s had passed", reliability.DrainExpiry)
	}

	clock.Advance(2 * time.Minute)
	if drain.Requested() {
		t.Errorf("the drain is still on more than %s after it was asked for", reliability.DrainExpiry)
	}
}

func TestAMarkerLeftBehindByAnEarlierBootIsIgnored(t *testing.T) {
	drain, _, home := aDrain(t)
	marker := `{"bootID":"a boot that is over","requestedAt":"` +
		startOfTime.Format(time.RFC3339Nano) + `","reason":"an update that finished long ago"}`
	if err := os.WriteFile(drainMarkerPath(home), []byte(marker), contract.DataFileMode); err != nil {
		t.Fatalf("writing the stale marker failed: %v", err)
	}

	if drain.Requested() {
		t.Errorf("a marker from a boot that is over still stops the loop from taking work")
	}
}

func TestADamagedMarkerIsTreatedAsADrain(t *testing.T) {
	drain, _, home := aDrain(t)
	if err := os.WriteFile(drainMarkerPath(home), []byte("half a mar"), contract.DataFileMode); err != nil {
		t.Fatalf("writing the damaged marker failed: %v", err)
	}

	if !drain.Requested() {
		t.Errorf("a damaged marker was ignored, and a marker nobody can read has to mean stop")
	}
}

func TestTheBootIdentifierIsTheOneTheMachineReports(t *testing.T) {
	first := reliability.BootID()

	if first != reliability.BootID() {
		t.Errorf("the boot identifier changed between two reads, so no marker could ever match it")
	}
	if first == "" {
		t.Skip("this machine does not report a boot identifier, so a marker is honoured whatever boot wrote it")
	}
	if len(first) < 8 {
		t.Errorf("the boot identifier is %q, which is too short to tell two boots apart", first)
	}
}
