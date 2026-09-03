package update_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/update"
)

// startOfTime is the moment every test's fake clock begins at.
var startOfTime = time.Date(2026, 9, 2, 3, 0, 0, 0, time.UTC)

// anInstalledRelease puts one version under the releases folder and points the
// current link at it, which is what a machine looks like after "coeus install".
func anInstalledRelease(t *testing.T, home contract.Home, version string, program string) string {
	t.Helper()
	folder := home.ReleaseFolder(version)
	if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
		t.Fatalf("making the release folder failed: %v", err)
	}
	binary := filepath.Join(folder, update.BinaryName)
	if err := os.WriteFile(binary, []byte(program), 0o755); err != nil {
		t.Fatalf("writing the installed program failed: %v", err)
	}
	_ = os.Remove(home.CurrentReleaseLink())
	if err := os.Symlink(binary, home.CurrentReleaseLink()); err != nil {
		t.Fatalf("pointing the current link at %s failed: %v", binary, err)
	}
	return binary
}

// anUpdater builds the updater a test drives, with the fake clock, the release
// folder it is to read, and the version the running program says it is.
func anUpdater(t *testing.T, home contract.Home, clock contract.Clock, address string, version string) *update.Updater {
	t.Helper()
	updater, err := update.New(update.Settings{
		Home: home, Clock: clock, Address: address, Version: version,
	})
	if err != nil {
		t.Fatalf("building the updater failed: %v", err)
	}
	return updater
}

// keepTheClockMoving advances the fake clock whenever something is waiting on
// it, so that a test never waits on the real one.
func keepTheClockMoving(t *testing.T, clock *testkit.FakeClock) {
	t.Helper()
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			if clock.Sleepers() > 0 {
				clock.Advance(update.ReadyPoll)
			}
			time.Sleep(time.Millisecond)
		}
	}()
}

// linkPointsAt is where the current link points now.
func linkPointsAt(t *testing.T, home contract.Home) string {
	t.Helper()
	where, err := os.Readlink(home.CurrentReleaseLink())
	if err != nil {
		t.Fatalf("reading the current link failed: %v", err)
	}
	return where
}

func TestAnUpdateInstallsTheNewVersionAndPointsTheLinkAtIt(t *testing.T) {
	home, service := aMachineWithAService(t)
	anInstalledRelease(t, home, "0.6.0", aWorkingProgram)
	address := t.TempDir()
	aRelease(t, address, "0.7.0", aWorkingProgram)
	clock := testkit.NewFakeClock(startOfTime)
	keepTheClockMoving(t, clock)

	outcome, err := anUpdater(t, home, clock, address, "0.6.0").Install(context.Background(), "")

	if err != nil {
		t.Fatalf("the update failed: %v", err)
	}
	if !outcome.Installed || outcome.To != "0.7.0" {
		t.Errorf("the update reports %+v rather than having installed 0.7.0", outcome)
	}
	if linkPointsAt(t, home) != filepath.Join(home.ReleaseFolder("0.7.0"), update.BinaryName) {
		t.Errorf("the current link points at %s rather than the new release", linkPointsAt(t, home))
	}
	if !service.running(t) {
		t.Errorf("the service was not left running:\n%s", service.told(t))
	}
	told := service.told(t)
	if !strings.Contains(told, "stop") || !strings.Contains(told, "start") {
		t.Errorf("the service was not stopped and started again:\n%s", told)
	}
	if !strings.Contains(told, "drained") {
		t.Errorf("the drain marker was not set before the service was stopped:\n%s", told)
	}
	if _, err := os.Stat(filepath.Join(home.RunFolder(), "drain.json")); !os.IsNotExist(err) {
		t.Errorf("the drain marker was left behind after the update")
	}
}

func TestAnUpdateRunsTheMigrationStepWithTheNewProgram(t *testing.T) {
	home, _ := aMachineWithAService(t)
	anInstalledRelease(t, home, "0.6.0", aWorkingProgram)
	address := t.TempDir()
	aRelease(t, address, "0.7.0", aWorkingProgram)
	clock := testkit.NewFakeClock(startOfTime)
	keepTheClockMoving(t, clock)

	if _, err := anUpdater(t, home, clock, address, "0.6.0").Install(context.Background(), ""); err != nil {
		t.Fatalf("the update failed: %v", err)
	}

	if !strings.Contains(programTold(t), update.MigrateFlag) {
		t.Errorf("the new program was never asked to bring the database forward:\n%s", programTold(t))
	}
}

func TestAReleaseThatDoesNotComeUpGoesBackToTheVersionThatWasRunning(t *testing.T) {
	home, service := aMachineWithAService(t)
	oldBinary := anInstalledRelease(t, home, "0.6.0", aWorkingProgram)
	address := t.TempDir()
	aRelease(t, address, "0.7.0", aProgramThatExitsAtOnce)
	clock := testkit.NewFakeClock(startOfTime)
	keepTheClockMoving(t, clock)

	outcome, err := anUpdater(t, home, clock, address, "0.6.0").Install(context.Background(), "")

	if err == nil {
		t.Fatalf("a release that never came up was reported as installed")
	}
	if !outcome.RolledBack {
		t.Errorf("the update reports %+v rather than a rollback", outcome)
	}
	if linkPointsAt(t, home) != oldBinary {
		t.Errorf("the current link points at %s rather than back at %s", linkPointsAt(t, home), oldBinary)
	}
	if !service.running(t) {
		t.Errorf("the version that was working was not started again:\n%s", service.told(t))
	}
	if waited := clock.Now().Sub(startOfTime); waited > update.ReadyDeadline+update.ReadyPoll {
		t.Errorf("the rollback took %s of the agent's time, which is past the %s deadline", waited, update.ReadyDeadline)
	}
	if !strings.Contains(err.Error(), "0.7.0") {
		t.Errorf("the report does not name the version that failed: %v", err)
	}
}

func TestNothingHappensWhenTheVersionOnOfferIsTheOneRunning(t *testing.T) {
	home, service := aMachineWithAService(t)
	anInstalledRelease(t, home, "0.7.0", aWorkingProgram)
	address := t.TempDir()
	aRelease(t, address, "0.7.0", aWorkingProgram)

	outcome, err := anUpdater(t, home, testkit.NewFakeClock(startOfTime), address, "0.7.0").
		Install(context.Background(), "")

	if err != nil {
		t.Fatalf("checking an address that offers the running version failed: %v", err)
	}
	if outcome.Installed {
		t.Errorf("the update installed %s over the version already running", outcome.To)
	}
	if service.told(t) != "" {
		t.Errorf("the service was disturbed when there was nothing to install:\n%s", service.told(t))
	}
}

func TestACheckSaysWhatIsOnOfferWithoutChangingAnything(t *testing.T) {
	home, service := aMachineWithAService(t)
	anInstalledRelease(t, home, "0.6.0", aWorkingProgram)
	address := t.TempDir()
	aRelease(t, address, "0.7.0", aWorkingProgram)

	available, err := anUpdater(t, home, testkit.NewFakeClock(startOfTime), address, "0.6.0").
		Check(context.Background())

	if err != nil {
		t.Fatalf("the check failed: %v", err)
	}
	if available.Offered != "0.7.0" || !available.Newer {
		t.Errorf("the check reports %+v rather than 0.7.0 being newer", available)
	}
	if available.Running != "0.6.0" {
		t.Errorf("the check says the running version is %q rather than 0.6.0", available.Running)
	}
	if service.told(t) != "" {
		t.Errorf("a check touched the service:\n%s", service.told(t))
	}
	if _, err := os.Stat(home.ReleaseFolder("0.7.0")); !os.IsNotExist(err) {
		t.Errorf("a check installed something")
	}
}

func TestAPinnedVersionTheAddressDoesNotOfferIsRefused(t *testing.T) {
	home, _ := aMachineWithAService(t)
	anInstalledRelease(t, home, "0.6.0", aWorkingProgram)
	address := t.TempDir()
	aRelease(t, address, "0.7.0", aWorkingProgram)

	_, err := anUpdater(t, home, testkit.NewFakeClock(startOfTime), address, "0.6.0").
		Install(context.Background(), "0.9.0")

	if err == nil {
		t.Fatalf("a version the address does not offer was installed")
	}
	if !strings.Contains(err.Error(), "0.9.0") || !strings.Contains(err.Error(), "0.7.0") {
		t.Errorf("the refusal does not say what was asked for and what is on offer: %v", err)
	}
}

func TestARollbackGoesToThePreviousInstalledRelease(t *testing.T) {
	home, service := aMachineWithAService(t)
	previous := anInstalledRelease(t, home, "0.6.0", aWorkingProgram)
	anInstalledRelease(t, home, "0.7.0", aWorkingProgram)
	clock := testkit.NewFakeClock(startOfTime)
	keepTheClockMoving(t, clock)

	outcome, err := anUpdater(t, home, clock, t.TempDir(), "0.7.0").Rollback(context.Background())

	if err != nil {
		t.Fatalf("the rollback failed: %v", err)
	}
	if outcome.To != "0.6.0" {
		t.Errorf("the rollback went to %q rather than 0.6.0", outcome.To)
	}
	if linkPointsAt(t, home) != previous {
		t.Errorf("the current link points at %s rather than %s", linkPointsAt(t, home), previous)
	}
	if !service.running(t) {
		t.Errorf("the service was not started again after the rollback:\n%s", service.told(t))
	}
}

func TestARollbackWithNothingToGoBackToSaysSo(t *testing.T) {
	home, _ := aMachineWithAService(t)
	anInstalledRelease(t, home, "0.7.0", aWorkingProgram)

	_, err := anUpdater(t, home, testkit.NewFakeClock(startOfTime), t.TempDir(), "0.7.0").
		Rollback(context.Background())

	if err == nil {
		t.Fatalf("a rollback with only one installed release said it worked")
	}
	if !strings.Contains(err.Error(), home.ReleasesFolder()) {
		t.Errorf("the refusal does not say where the releases are: %v", err)
	}
}

func TestAnUpdaterNeedsAHomeAndAClock(t *testing.T) {
	if _, err := update.New(update.Settings{Clock: testkit.NewFakeClock(startOfTime)}); err == nil {
		t.Errorf("an updater with no home folder was built")
	}
	if _, err := update.New(update.Settings{Home: contract.NewHome("/tmp/nothing")}); err == nil {
		t.Errorf("an updater with no clock was built")
	}
}

func TestAnUpdaterFallsBackToThePublishedAddress(t *testing.T) {
	home, _ := aMachineWithAService(t)
	updater, err := update.New(update.Settings{
		Home: home, Clock: testkit.NewFakeClock(startOfTime), Version: "0.6.0",
	})
	if err != nil {
		t.Fatalf("building the updater failed: %v", err)
	}

	if updater.Address() != update.DefaultAddress {
		t.Errorf("an updater with no address of its own reads %q rather than %q", updater.Address(), update.DefaultAddress)
	}
}
