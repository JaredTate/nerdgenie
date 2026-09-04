//go:build integration

package update_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/log"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/update"
)

// aHomeInUse fills a temporary home with everything an update must not touch: a
// real database holding a task's events, the two memory files, a skill, a
// browser profile, the vault, and the configuration.
func aHomeInUse(t *testing.T, home contract.Home) {
	t.Helper()
	opened, err := log.Open(context.Background(), home.DatabaseFile())
	if err != nil {
		t.Fatalf("making the event log failed: %v", err)
	}
	for _, text := range []string{"take out the bins", "they are out"} {
		if _, err := opened.Append(context.Background(), contract.Event{
			Occurred: startOfTime, TaskID: "t17", Kind: contract.EventMessage, Body: []byte(`{"text":"` + text + `"}`),
		}); err != nil {
			t.Fatalf("writing an event failed: %v", err)
		}
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("closing the event log failed: %v", err)
	}

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("making the vault key failed: %v", err)
	}
	for path, written := range map[string]string{
		home.WorldFactsFile():                                    "- m1 [2026-09-02T14:00:00Z | task 17] the bins go out on Tuesday\n",
		home.UserFactsFile():                                     "- u1 [2026-09-02T14:00:00Z | task 17] the user likes short answers\n",
		filepath.Join(home.SkillFolder("bins"), "SKILL.md"):      "# Put the bins out\n\nOne step: put them out.\n",
		filepath.Join(home.BrowserProfile("default"), "Cookies"): "what the browser remembers",
		home.VaultFile():                                         "the encrypted vault",
		home.VaultKeyFile():                                      identity.String() + "\n",
		home.ConfigFile():                                        "default_model = \"local\"\n",
		filepath.Join(home.InboxFolder(), "photo.jpg"):           "a photograph",
	} {
		if err := os.MkdirAll(filepath.Dir(path), contract.HomeFolderMode); err != nil {
			t.Fatalf("making the folder for %s failed: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(written), contract.DataFileMode); err != nil {
			t.Fatalf("writing %s failed: %v", path, err)
		}
	}
}

// everythingKept reads every file in the home except the releases and the run
// folder, which are the two the updater is allowed to change.
func everythingKept(t *testing.T, home contract.Home) map[string]string {
	t.Helper()
	kept := map[string]string{}
	err := filepath.WalkDir(home.Root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		inside := strings.TrimPrefix(path, home.Root+string(filepath.Separator))
		if entry.IsDir() {
			if inside == "releases" || inside == "run" || inside == "backups" {
				return filepath.SkipDir
			}
			return nil
		}
		written, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		kept[inside] = string(written)
		return nil
	})
	if err != nil {
		t.Fatalf("reading the home folder failed: %v", err)
	}
	return kept
}

func TestAnUpdateOverALiveInstallKeepsEverythingTheAgentHas(t *testing.T) {
	home, _ := aMachineWithAService(t)
	aHomeInUse(t, home)
	anInstalledRelease(t, home, "0.6.0", aWorkingProgram)
	before := everythingKept(t, home)
	if len(before) < 8 {
		t.Fatalf("the home only holds %d files, so this test is not proving much", len(before))
	}
	address := t.TempDir()
	aRelease(t, address, "0.7.0", aWorkingProgram)
	clock := testkit.NewFakeClock(startOfTime)
	keepTheClockMoving(t, clock)

	outcome, err := anUpdater(t, home, clock, address, "0.6.0").Install(context.Background(), "")

	if err != nil {
		t.Fatalf("the update failed: %v", err)
	}
	if !outcome.Installed {
		t.Fatalf("the update reports %+v rather than having installed 0.7.0", outcome)
	}
	after := everythingKept(t, home)
	for path, written := range before {
		if after[path] != written {
			t.Errorf("%s is not what it was before the update:\nbefore: %q\nafter:  %q", path, written, after[path])
		}
	}
	for path := range after {
		if _, wasThere := before[path]; !wasThere {
			t.Errorf("the update left a new file at %s", path)
		}
	}
}

func TestTheEventsAreStillReadableAfterAnUpdate(t *testing.T) {
	home, _ := aMachineWithAService(t)
	aHomeInUse(t, home)
	anInstalledRelease(t, home, "0.6.0", aWorkingProgram)
	address := t.TempDir()
	aRelease(t, address, "0.7.0", aWorkingProgram)
	clock := testkit.NewFakeClock(startOfTime)
	keepTheClockMoving(t, clock)

	if _, err := anUpdater(t, home, clock, address, "0.6.0").Install(context.Background(), ""); err != nil {
		t.Fatalf("the update failed: %v", err)
	}

	opened, err := log.Open(context.Background(), home.DatabaseFile())
	if err != nil {
		t.Fatalf("opening the event log after the update failed: %v", err)
	}
	defer func() { _ = opened.Close() }()
	events, err := opened.ByTask(context.Background(), "t17")
	if err != nil {
		t.Fatalf("reading the task's events back failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("the task has %d events after the update rather than the two it had", len(events))
	}
	if err := update.CheckSchema(context.Background(), home.DatabaseFile()); err != nil {
		t.Errorf("the database is not one this program understands after the update: %v", err)
	}
}
