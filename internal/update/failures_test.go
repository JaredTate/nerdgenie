package update_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/update"
)

func TestAServiceManagerThatRefusesLeavesTheLinkWhereItWas(t *testing.T) {
	home, service := aMachineWithAService(t)
	oldBinary := anInstalledRelease(t, home, "0.6.0", aWorkingProgram)
	address := t.TempDir()
	aRelease(t, address, "0.7.0", aWorkingProgram)
	service.refuse(t)
	clock := testkit.NewFakeClock(startOfTime)
	keepTheClockMoving(t, clock)

	_, err := anUpdater(t, home, clock, address, "0.6.0").Install(context.Background(), "")

	if err == nil {
		t.Fatalf("an update said it worked when the service manager refused to do anything")
	}
	if linkPointsAt(t, home) != oldBinary {
		t.Errorf("the current link points at %s rather than still at %s", linkPointsAt(t, home), oldBinary)
	}
	if !strings.Contains(err.Error(), "systemctl") {
		t.Errorf("the report does not say that the service manager is what refused: %v", err)
	}
}

func TestARollbackToAReleaseWithNoProgramInItSaysSo(t *testing.T) {
	home, _ := aMachineWithAService(t)
	if err := os.MkdirAll(home.ReleaseFolder("0.6.0"), contract.HomeFolderMode); err != nil {
		t.Fatalf("making the empty release folder failed: %v", err)
	}
	current := anInstalledRelease(t, home, "0.7.0", aWorkingProgram)
	clock := testkit.NewFakeClock(startOfTime)
	keepTheClockMoving(t, clock)

	_, err := anUpdater(t, home, clock, t.TempDir(), "0.7.0").Rollback(context.Background())

	if err == nil {
		t.Fatalf("a rollback to a release folder with no program in it said it worked")
	}
	if linkPointsAt(t, home) != current {
		t.Errorf("the current link points at %s rather than still at %s", linkPointsAt(t, home), current)
	}
}

func TestAnUpdateOnAMachineWithNoLinkYetSaysHowToStartTheServiceByHand(t *testing.T) {
	home, _ := aMachineWithAService(t)
	address := t.TempDir()
	aRelease(t, address, "0.7.0", aProgramThatExitsAtOnce)
	clock := testkit.NewFakeClock(startOfTime)
	keepTheClockMoving(t, clock)

	_, err := anUpdater(t, home, clock, address, "0.6.0").Install(context.Background(), "")

	if err == nil {
		t.Fatalf("a release that never came up was reported as installed")
	}
	if !strings.Contains(err.Error(), "systemctl --user start") {
		t.Errorf("the report does not say how to start the service by hand: %v", err)
	}
}

func TestAnUpdateSaysWhatItIsDoingAsItGoes(t *testing.T) {
	home, _ := aMachineWithAService(t)
	anInstalledRelease(t, home, "0.6.0", aWorkingProgram)
	address := t.TempDir()
	aRelease(t, address, "0.7.0", aWorkingProgram)
	clock := testkit.NewFakeClock(startOfTime)
	keepTheClockMoving(t, clock)
	said := []string{}

	updater, err := update.New(update.Settings{
		Home: home, Clock: clock, Address: address, Version: "0.6.0",
		Announce: func(line string) { said = append(said, line) },
	})
	if err != nil {
		t.Fatalf("building the updater failed: %v", err)
	}
	if _, err := updater.Install(context.Background(), ""); err != nil {
		t.Fatalf("the update failed: %v", err)
	}

	whole := strings.Join(said, "\n")
	for _, wanted := range []string{"installing version 0.7.0", "running task", "database", "0.7.0 is running"} {
		if !strings.Contains(whole, wanted) {
			t.Errorf("the update never said anything about %q:\n%s", wanted, whole)
		}
	}
}

func TestACheckOnAnAddressThatIsNotThereSaysSo(t *testing.T) {
	home, _ := aMachineWithAService(t)

	_, err := anUpdater(t, home, testkit.NewFakeClock(startOfTime), filepath.Join(t.TempDir(), "nothing"), "0.6.0").
		Check(context.Background())

	if err == nil {
		t.Fatalf("a check against an address with nothing on it said all was well")
	}
}

func TestARelativeCurrentLinkIsStillReadAsAProgram(t *testing.T) {
	home, _ := aMachineWithAService(t)
	anInstalledRelease(t, home, "0.6.0", aWorkingProgram)
	anInstalledRelease(t, home, "0.7.0", aWorkingProgram)
	if err := os.Remove(home.CurrentReleaseLink()); err != nil {
		t.Fatalf("taking the link away failed: %v", err)
	}
	if err := os.Symlink(filepath.Join("0.7.0", update.BinaryName), home.CurrentReleaseLink()); err != nil {
		t.Fatalf("making a relative link failed: %v", err)
	}
	clock := testkit.NewFakeClock(startOfTime)
	keepTheClockMoving(t, clock)

	outcome, err := anUpdater(t, home, clock, t.TempDir(), "0.7.0").Rollback(context.Background())

	if err != nil {
		t.Fatalf("the rollback failed: %v", err)
	}
	if outcome.To != "0.6.0" {
		t.Errorf("the rollback went to %q rather than 0.6.0", outcome.To)
	}
}

func TestASourceUsesTheClientItWasGiven(t *testing.T) {
	folder := t.TempDir()
	aRelease(t, folder, "0.7.0", aWorkingProgram)
	address := aReleaseServer(t, folder)
	asked := 0
	client := &http.Client{Transport: countingTransport{asked: &asked}}

	if _, err := (update.Source{Address: address, Client: client}).Manifest(context.Background()); err != nil {
		t.Fatalf("reading the manifest with a client of our own failed: %v", err)
	}

	if asked != 1 {
		t.Errorf("the source made %d requests through the client it was given rather than one", asked)
	}
}

// countingTransport counts the requests a source makes, so that a test can prove
// the client it handed over is the one that was used.
type countingTransport struct{ asked *int }

// RoundTrip counts one request and lets the standard transport do the work.
func (transport countingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	*transport.asked++
	return http.DefaultTransport.RoundTrip(request)
}
