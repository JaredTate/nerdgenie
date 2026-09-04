package browser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// This package starts a real Google Chrome, and it used to hand it the session
// message bus, through a DBUS_SESSION_BUS_ADDRESS entry in environmentPassedOn.
// Chrome, once its accessibility support is activated, brings the desktop's
// accessibility bridge up with it, and on this machine that started the screen
// reader and it spoke aloud through the user's speakers while this package's
// integration tests were running. That entry is gone now and the worker is handed
// NO_AT_BRIDGE=1 instead, which is what
// TestTheBrowserWorkerIsHandedTheBridgeSwitchedOffAndNoSessionBus below holds.
// Nothing in Nerd Genie writes the setting, so the guard cannot be a check of Nerd Genie's
// own code: it has to be a check of the machine, taken before and after this
// package's tests run. The same guard is in
// internal/desktop/accessibility_test.go, and it belongs in internal/testkit so
// that both packages share one copy rather than two.

// accessibilitySettings are the two desktop settings that turn the accessibility
// bus and the screen reader on. Starting a browser must leave both exactly as it
// found them.
var accessibilitySettings = [][2]string{
	{"org.gnome.desktop.a11y.applications", "screen-reader-enabled"},
	{"org.gnome.desktop.interface", "toolkit-accessibility"},
}

// screenReaderPrograms are the programs that read the screen aloud. None of them
// may start while these tests run.
var screenReaderPrograms = []string{"orca", "speech-dispatch", "speech-dispatcher"}

// settingsTimeout is how long the settings reader waits, because a machine with
// no session bus can leave the reader hanging rather than answering.
const settingsTimeout = 5 * time.Second

// TestMain records the machine's accessibility settings and the screen readers
// running on it, runs this package's tests, and then fails the package when
// either changed. It reads and never writes, and it never signals a process, so
// the guard itself cannot be the thing that turns a screen reader on or off.
func TestMain(tests *testing.M) {
	before := settingsAsTheyStand()
	beforeRunning := screenReadersRunning()

	code := tests.Run()

	complaints := changedSettings(before, settingsAsTheyStand())
	if started := startedSince(beforeRunning, screenReadersRunning()); len(started) > 0 {
		complaints = append(complaints, fmt.Sprintf(
			"a screen reader started while these tests ran: %s. The browser this package launches must leave the desktop's accessibility"+
				" settings exactly as it found them, so record this as a finding and turn the reader off from the desktop's own"+
				" accessibility settings rather than by killing anything", strings.Join(started, ", ")))
	}
	if len(complaints) > 0 {
		for _, complaint := range complaints {
			fmt.Fprintln(os.Stderr, "the browser tests changed this machine: "+complaint)
		}
		os.Exit(1)
	}
	os.Exit(code)
}

// changedSettings names every accessibility setting that reads differently now
// from how it read before the tests ran.
func changedSettings(before map[string]string, after map[string]string) []string {
	complaints := []string{}
	for key, was := range before {
		now, stillKnown := after[key]
		if !stillKnown || now == was {
			continue
		}
		complaints = append(complaints, fmt.Sprintf(
			"the setting %s read %q before the tests and %q after them, so set it back with gsettings and record this as a finding", key, was, now))
	}
	return complaints
}

// settingsAsTheyStand reads the two settings, leaving out any the machine cannot
// answer about, which is what a container with no desktop does.
func settingsAsTheyStand() map[string]string {
	standing := map[string]string{}
	for _, setting := range accessibilitySettings {
		if value, known := settingValue(setting[0], setting[1]); known {
			standing[setting[0]+" "+setting[1]] = value
		}
	}
	return standing
}

// settingValue reads one desktop setting, and says false when this machine has
// no gsettings, no such schema, or no session to read it from.
func settingValue(schema string, key string) (string, bool) {
	program, err := exec.LookPath("gsettings")
	if err != nil {
		return "", false
	}
	running := exec.Command(program, "get", schema, key)
	running.Env = os.Environ()
	said, err := runWithin(running, settingsTimeout)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(said), true
}

// runWithin runs a command and gives up on it after the timeout, so that a
// machine with no session bus cannot hang the whole test binary.
func runWithin(running *exec.Cmd, limit time.Duration) (string, error) {
	said := &strings.Builder{}
	running.Stdout = said
	if err := running.Start(); err != nil {
		return "", err
	}
	finished := make(chan error, 1)
	go func() { finished <- running.Wait() }()
	select {
	case err := <-finished:
		return said.String(), err
	case <-time.After(limit):
		_ = running.Process.Kill()
		<-finished
		return "", fmt.Errorf("reading a desktop setting took longer than %s", limit)
	}
}

// screenReadersRunning lists the process identifiers of every screen reader on
// this machine right now, read out of /proc. It finds a process by the name the
// kernel holds for it and never by a pattern, and it only ever reads.
func screenReadersRunning() []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	found := []int{}
	for _, entry := range entries {
		processID, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		name, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
		if err != nil {
			continue
		}
		if slices.Contains(screenReaderPrograms, strings.TrimSpace(string(name))) {
			found = append(found, processID)
		}
	}
	return found
}

// startedSince names the screen readers that were not running before the tests
// and are running now.
func startedSince(before []int, after []int) []string {
	started := []string{}
	for _, processID := range after {
		if slices.Contains(before, processID) {
			continue
		}
		name, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(processID), "comm"))
		if err != nil {
			started = append(started, "process "+strconv.Itoa(processID))
			continue
		}
		started = append(started, strings.TrimSpace(string(name))+" as process "+strconv.Itoa(processID))
	}
	return started
}

func TestTheBrowserWorkerIsHandedNothingThatTurnsTheAccessibilityBusOn(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	for _, name := range accessibilityVariables {
		t.Setenv(name, "1")
	}

	handed := workerEnvironment()

	for _, line := range handed {
		name, _, _ := strings.Cut(line, "=")
		if slices.Contains(accessibilityVariables, name) {
			t.Errorf("the browser worker was handed %s, and a variable that switches an accessibility bridge on must stay out of its"+
				" environment, because bringing the bridge up starts the screen reader on this machine", name)
		}
	}
}

// accessibilityVariables are the environment variables that switch a toolkit's
// accessibility bridge on. Chrome must be handed none of them.
var accessibilityVariables = []string{
	"GNOME_ACCESSIBILITY",
	"GTK_MODULES",
	"QT_ACCESSIBILITY",
	"QT_LINUX_ACCESSIBILITY_ALWAYS_ON",
	"AT_SPI_BUS_ADDRESS",
	"ACCESSIBILITY_ENABLED",
}

// TestTheBrowserWorkerIsHandedTheBridgeSwitchedOffAndNoSessionBus is the guard on
// the fix for the finding above. Chrome reaches the desktop's accessibility bus
// over the session message bus, and reaching that bus is what wrote
// org.gnome.desktop.interface toolkit-accessibility and started the screen reader
// on this machine. So the environment the process starter builds carries
// NO_AT_BRIDGE=1, which is how a toolkit is told to load no accessibility bridge
// at all, and carries none of the three names that tell a program where the
// session message bus and the accessibility bus are. That has to hold whatever
// the caller's own environment holds, so the test sets all four names first.
func TestTheBrowserWorkerIsHandedTheBridgeSwitchedOffAndNoSessionBus(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	t.Setenv("NO_AT_BRIDGE", "0")
	for _, name := range busVariablesNeverPassedOn {
		t.Setenv(name, "unix:path=/run/user/1000/bus")
	}

	handed := workerEnvironment()

	if !slices.Contains(handed, "NO_AT_BRIDGE=1") {
		t.Errorf("the browser worker was handed %v, and it must be handed NO_AT_BRIDGE=1 so that Chrome loads no accessibility bridge,"+
			" because bringing the bridge up turns the screen reader on and it speaks aloud through the user's speakers", handed)
	}
	for _, line := range handed {
		name, _, _ := strings.Cut(line, "=")
		if slices.Contains(busVariablesNeverPassedOn, name) {
			t.Errorf("the browser worker was handed %s, and a name that tells Chrome where the session message bus or the accessibility"+
				" bus is must stay out of its environment, because a program that reaches that bus can switch the accessibility bridge"+
				" on for the whole desktop", name)
		}
	}
}

// busVariablesNeverPassedOn are the names that tell a program where the session
// message bus and the accessibility bus are. Chrome is handed none of them.
var busVariablesNeverPassedOn = []string{
	"DBUS_SESSION_BUS_ADDRESS",
	"DBUS_SESSION_BUS_PID",
	"AT_SPI_BUS_ADDRESS",
}
