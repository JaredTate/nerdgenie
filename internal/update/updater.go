package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/JaredTate/coeus/internal/command"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/reliability"
)

// MigrateFlag is what the updater passes the newly installed program to have it
// bring the database forward. The new version owns its own list of migrations,
// so it is the only program that can run them, and it runs them only after it
// has come up and answered.
const MigrateFlag = "--migrate"

// migrateWait is how long the migration step may take. Migrations are a handful
// of statements on a small database, so ten minutes is already a wedged one.
const migrateWait = 10 * time.Minute

// Settings is everything one updater needs from the program around it.
type Settings struct {
	// Home is the folder holding the releases, the link, and the database.
	Home contract.Home
	// Clock is where the sixty-second deadline is read from.
	Clock contract.Clock
	// Version is the version this program was built as.
	Version string
	// Address is where releases are read from, and is DefaultAddress when it is
	// empty.
	Address string
	// Architecture is the machine to install for, and is this machine's when it
	// is empty.
	Architecture string
	// BackupFolder is where the backup before a migration is written, and is the
	// home's backups folder when it is empty.
	BackupFolder string
	// Announce says one line about what is happening, for a person watching the
	// sixty seconds go by. It may be nil.
	Announce func(line string)
}

// Outcome is what one update did, so that a person can be told and a test can
// check it.
type Outcome struct {
	// From is the version that was running before.
	From string
	// To is the version running now, which is From when nothing changed.
	To string
	// Installed says a newer version is now the live one.
	Installed bool
	// RolledBack says the new version did not come up and the link went back.
	RolledBack bool
	// Reason says why nothing was installed, or why the link went back.
	Reason string
}

// Available is what a release address is offering.
type Available struct {
	// Running is the version this program was built as.
	Running string
	// Offered is the version the address is serving.
	Offered string
	// Date is the day that version was built.
	Date string
	// Newer says the offered version is above the running one.
	Newer bool
}

// Updater installs a new version beside the running one and switches back when
// it does not come up.
type Updater struct {
	settings Settings
	source   Source
	service  service
}

// New builds the updater "coeus update" drives.
func New(settings Settings) (*Updater, error) {
	if settings.Home.Root == "" {
		return nil, errors.New("the updater needs the home folder, because that is where the releases and the link live")
	}
	if settings.Clock == nil {
		return nil, errors.New("the updater needs a clock, so pass clock.System() or the one the test controls")
	}
	if settings.Address == "" {
		settings.Address = DefaultAddress
	}
	if settings.Architecture == "" {
		settings.Architecture = runtime.GOARCH
	}
	if settings.Version == "" {
		settings.Version = DevelopmentVersion
	}
	return &Updater{
		settings: settings,
		source:   Source{Address: settings.Address},
		service:  service{unit: command.ServiceName, stopWait: reliability.DrainExpiry},
	}, nil
}

// Address is where this updater reads releases from, which is what the check
// prints so that a person knows where the answer came from.
func (updater *Updater) Address() string { return updater.settings.Address }

// Check reads the release manifest and says what is on offer, changing nothing.
func (updater *Updater) Check(ctx context.Context) (Available, error) {
	manifest, err := updater.source.Manifest(ctx)
	if err != nil {
		return Available{}, err
	}
	return Available{
		Running: updater.settings.Version,
		Offered: manifest.Version,
		Date:    manifest.Date,
		Newer:   Newer(updater.settings.Version, manifest.Version),
	}, nil
}

// Install puts the version the address offers in place of the running one, or
// the version named when one is named, and puts everything back as it was if the
// new version does not come up within the deadline.
func (updater *Updater) Install(ctx context.Context, wanted string) (Outcome, error) {
	running := updater.settings.Version
	outcome := Outcome{From: running, To: running}

	manifest, err := updater.source.Manifest(ctx)
	if err != nil {
		return outcome, err
	}
	if wanted != "" && manifest.Version != wanted {
		return outcome, fmt.Errorf("version %s was asked for and %s is what %s offers, so nothing was installed; point --from at the release you want",
			wanted, manifest.Version, updater.settings.Address)
	}
	if !Newer(running, manifest.Version) {
		outcome.Reason = fmt.Sprintf("version %s is already what is running, so there is nothing to install", manifest.Version)
		return outcome, nil
	}

	updater.say(fmt.Sprintf("installing version %s over %s", manifest.Version, running))
	binary, err := installRelease(ctx, updater.settings.Home, updater.source, manifest, updater.settings.Architecture)
	if err != nil {
		return outcome, err
	}
	return updater.putIntoService(ctx, outcome, manifest.Version, binary)
}

// putIntoService switches the link to the new binary, brings the service up on
// it, brings the database forward, and undoes all three when any of them fails.
func (updater *Updater) putIntoService(ctx context.Context, outcome Outcome, version string, binary string) (Outcome, error) {
	previous := currentBinary(updater.settings.Home)
	if err := updater.switchTo(ctx, binary); err != nil {
		return updater.undo(ctx, outcome, version, previous, err)
	}
	if err := updater.migrateWithTheNewProgram(ctx, binary); err != nil {
		return updater.undo(ctx, outcome, version, previous, err)
	}

	outcome.To = version
	outcome.Installed = true
	updater.say("version " + version + " is running")
	return outcome, pruneReleases(updater.settings.Home, KeptReleases, version, versionOf(previous))
}

// undo puts the link back on the version that was running and reports both what
// went wrong and what was done about it.
func (updater *Updater) undo(ctx context.Context, outcome Outcome, version string, previous string, why error) (Outcome, error) {
	outcome.RolledBack = true
	outcome.Reason = why.Error()
	failed := fmt.Errorf("version %s was installed but did not work: %w", version, why)
	if err := updater.rollBack(ctx, previous, failed); err != nil {
		return outcome, err
	}
	return outcome, fmt.Errorf("%w, so the link is back on %s and nothing else was changed", failed, versionOf(previous))
}

// Rollback puts the link back on the newest installed version below the one
// running, which is what a person asks for when a release is bad in a way the
// readiness check did not catch.
func (updater *Updater) Rollback(ctx context.Context) (Outcome, error) {
	outcome := Outcome{From: updater.settings.Version, To: updater.settings.Version}
	live := versionOf(currentBinary(updater.settings.Home))
	versions, err := installedVersions(updater.settings.Home)
	if err != nil {
		return outcome, err
	}
	wanted := ""
	for _, version := range versions {
		if live == "" || Newer(version, live) {
			wanted = version
		}
	}
	if wanted == "" {
		return outcome, fmt.Errorf("there is no version below %s in %s to go back to, so install one with \"coeus update\" instead",
			live, updater.settings.Home.ReleasesFolder())
	}

	binary := filepath.Join(updater.settings.Home.ReleaseFolder(wanted), BinaryName)
	updater.say(fmt.Sprintf("going back from %s to %s", live, wanted))
	// The binary to go back to is read before the switch, because after it the
	// link points at the version being tried, and undoing onto that one would
	// leave the machine on the very version that would not come up.
	previous := currentBinary(updater.settings.Home)
	if err := updater.switchTo(ctx, binary); err != nil {
		return updater.undo(ctx, outcome, wanted, previous, err)
	}
	outcome.To = wanted
	outcome.Reason = fmt.Sprintf("went back from version %s to version %s by hand", live, wanted)
	return outcome, nil
}

// migrateWithTheNewProgram asks the version that has just come up to bring the
// database forward. It is the new program rather than this one because the
// migrations belong to the version that needs them, and it happens after the
// new version has answered so that a link switched back always lands on a schema
// the older program understands.
func (updater *Updater) migrateWithTheNewProgram(ctx context.Context, binary string) error {
	within, stop := context.WithTimeout(ctx, migrateWait)
	defer stop()

	updater.say("bringing the database forward")
	running := exec.CommandContext(within, binary, "update", MigrateFlag)
	running.Env = append(os.Environ(), "COEUS_HOME="+updater.settings.Home.Root)
	said, err := running.CombinedOutput()
	if err == nil {
		return nil
	}
	if len(said) > maxServiceOutput {
		said = said[:maxServiceOutput]
	}
	return fmt.Errorf("the new version could not bring the database forward and said %q: %w", string(said), err)
}

// versionOf is the version a release binary belongs to, which is the name of the
// folder it sits in.
func versionOf(binary string) string {
	if binary == "" {
		return ""
	}
	return filepath.Base(filepath.Dir(binary))
}

// say passes one line to whoever is watching the update, and does nothing when
// nobody is.
func (updater *Updater) say(line string) {
	if updater.settings.Announce != nil {
		updater.settings.Announce(line)
	}
}
