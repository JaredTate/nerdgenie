package update_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/testkit"
)

func TestARollbackToAVersionThatDoesNotComeUpGoesBackToTheOneThatWasRunning(t *testing.T) {
	home, service := aMachineWithAService(t)
	anInstalledRelease(t, home, "0.6.0", aProgramThatExitsAtOnce)
	working := anInstalledRelease(t, home, "0.7.0", aWorkingProgram)
	clock := testkit.NewFakeClock(startOfTime)
	keepTheClockMoving(t, clock)

	outcome, err := anUpdater(t, home, clock, t.TempDir(), "0.7.0").Rollback(context.Background())

	if err == nil {
		t.Fatalf("a rollback to a version that never came up was reported as done")
	}
	if linkPointsAt(t, home) != working {
		t.Errorf("the current link points at %s rather than back at %s, which was working", linkPointsAt(t, home), working)
	}
	if !service.running(t) {
		t.Errorf("the version that was working was not started again:\n%s", service.told(t))
	}
	if strings.Contains(err.Error(), "did not come up again either") {
		t.Errorf("the report says the working version could not be started again, and it was never tried: %v", err)
	}
	if !strings.Contains(err.Error(), "0.7.0") {
		t.Errorf("the report does not name the version the machine is back on: %v", err)
	}
	if outcome.To != "0.7.0" {
		t.Errorf("the outcome says the machine is on %q rather than back on 0.7.0", outcome.To)
	}
}
