// The switch and its rollback follow ZeroClaw's updater at
// ~/Code/zeroclaw/src/commands/update.rs, which keeps the program it is
// replacing, puts the new one in place with a rename, and puts the old one back
// when the new one does not run. What is different here is that nothing moves
// the program at all: the unit runs a link that never changes its name, so the
// switch and the rollback are each one rename of that link.

package update

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/reliability"
)

// The two numbers the sixty-second rule is made of.
const (
	// ReadyDeadline is how long the new version has to come up before the link
	// goes back to the version that was running.
	ReadyDeadline = 60 * time.Second
	// ReadyPoll is how often the service manager is asked whether it is up yet.
	ReadyPoll = 2 * time.Second
	// maxReadyAsks bounds the loop as well as the deadline does, so that a clock
	// which never moves cannot leave the updater asking forever.
	maxReadyAsks = 1000
)

// drainReason is written into the drain marker, so that whoever finds the file
// knows why the agent is finishing up and taking no new work.
const drainReason = "an update is being installed"

// switchTo points the current link at a binary and brings the service up on it,
// with the drain marker set so that the agent finishes the task it has rather
// than being cut off in the middle of one.
func (updater *Updater) switchTo(ctx context.Context, binary string) error {
	drain := reliability.NewDrain(updater.settings.Home, updater.settings.Clock)
	if err := drain.Request(drainReason); err != nil {
		return err
	}
	defer func() { _ = drain.Cancel() }()

	updater.say("waiting for the running task to finish")
	if err := updater.service.stop(ctx); err != nil {
		return err
	}
	if err := linkRelease(updater.settings.Home, binary); err != nil {
		return err
	}
	if err := updater.service.start(ctx); err != nil {
		return err
	}
	return updater.waitUntilReady(ctx)
}

// waitUntilReady gives the version now linked sixty seconds to answer the
// readiness check, asking every couple of seconds on the agent's own clock.
func (updater *Updater) waitUntilReady(ctx context.Context) error {
	clock := updater.settings.Clock
	deadline := clock.Now().Add(ReadyDeadline)
	var why error
	for range maxReadyAsks {
		if why = askIfReady(ctx, updater.settings.Home); why == nil {
			return nil
		}
		if !clock.Now().Before(deadline) {
			break
		}
		if err := clock.Sleep(ctx, ReadyPoll); err != nil {
			return fmt.Errorf("the wait for the new version to come up was cut short: %w", err)
		}
	}
	return fmt.Errorf("the new version did not answer %s within %s of being started, so it is not running properly: %w",
		ReadyCommand, ReadyDeadline, why)
}

// linkRelease points the current link at a binary, by making the new link beside
// it and renaming it over the old one, so that the link is never missing even
// for a moment.
func linkRelease(home contract.Home, binary string) error {
	if _, err := os.Stat(binary); err != nil {
		return fmt.Errorf("the program %s is not there, so the current link was left where it was: %w", binary, err)
	}
	link := home.CurrentReleaseLink()
	beside := filepath.Join(filepath.Dir(link), ".current-being-switched")
	if err := os.Remove(beside); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("the half-made link %s could not be cleared away: %w", beside, err)
	}
	if err := os.Symlink(binary, beside); err != nil {
		return fmt.Errorf("a link to %s could not be made in %s: %w", binary, filepath.Dir(link), err)
	}
	if err := os.Rename(beside, link); err != nil {
		_ = os.Remove(beside)
		return fmt.Errorf("the link %s could not be pointed at %s: %w", link, binary, err)
	}
	return nil
}

// currentBinary is the program the current link points at, and is empty when
// there is no link yet, which is what a machine looks like before "nerdgenie
// install" has run.
func currentBinary(home contract.Home) string {
	where, err := os.Readlink(home.CurrentReleaseLink())
	if err != nil {
		return ""
	}
	if filepath.IsAbs(where) {
		return where
	}
	return filepath.Join(filepath.Dir(home.CurrentReleaseLink()), where)
}

// rollBack puts the link back on the version that was running and starts the
// service again. Whatever it finds, it says so rather than leaving the machine
// with a link nobody can explain.
func (updater *Updater) rollBack(ctx context.Context, binary string, why error) error {
	updater.say("going back to " + binary)
	if binary == "" {
		return fmt.Errorf("%w, and there was no earlier version to go back to, so start the service by hand with \"systemctl --user start %s\"",
			why, updater.service.unit)
	}
	if err := linkRelease(updater.settings.Home, binary); err != nil {
		return fmt.Errorf("%w, and the link could not be put back either, so point %s at %s by hand: %w",
			why, updater.settings.Home.CurrentReleaseLink(), binary, err)
	}
	if err := updater.service.start(ctx); err != nil {
		return fmt.Errorf("%w, and the version that was working could not be started again: %w", why, err)
	}
	if err := updater.waitUntilReady(ctx); err != nil {
		return fmt.Errorf("%w, and the version that was working did not come up again either: %w", why, err)
	}
	return nil
}
