package desktop

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

// The desktop worker drives this machine's screen through a native library that
// reads the accessibility tree, and on this machine one run of it turned GNOME's
// screen reader on and Orca began reading the screen aloud. Nothing in Coeus
// writes that setting, so the guard cannot be a check of Coeus's own code: it
// has to be a check of the machine, taken before and after this package's tests
// run. TestMain is where that fits, because it is the one place that wraps every
// test in the package, the tagged ones included.

// screenReaderSettings are the two desktop settings that turn the accessibility
// bus and the screen reader on. Coeus must leave both exactly as it found them.
var screenReaderSettings = [][2]string{
	{"org.gnome.desktop.a11y.applications", "screen-reader-enabled"},
	{"org.gnome.desktop.interface", "toolkit-accessibility"},
}

// screenReaderPrograms are the programs that read the screen aloud. None of them
// may start while these tests run.
var screenReaderPrograms = []string{"orca", "speech-dispatch", "speech-dispatcher"}

// settingsTimeout is how long the settings reader waits, because a machine with
// no session bus can leave the reader hanging rather than answering.
const settingsTimeout = 5 * time.Second

// TestMain records the machine's screen-reader settings and the screen readers
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
			"a screen reader started while these tests ran: %s; the desktop worker's driver must never turn the accessibility bus on, "+
				"so stop it with the desktop's own accessibility settings and treat this as a finding", strings.Join(started, ", ")))
	}
	if len(complaints) > 0 {
		for _, complaint := range complaints {
			fmt.Fprintln(os.Stderr, "the desktop tests changed this machine: "+complaint)
		}
		os.Exit(1)
	}
	os.Exit(code)
}

// changedSettings names every screen-reader setting that reads differently now
// from how it read before the tests ran.
func changedSettings(before map[string]string, after map[string]string) []string {
	complaints := []string{}
	for key, was := range before {
		now, stillKnown := after[key]
		if !stillKnown || now == was {
			continue
		}
		complaints = append(complaints, fmt.Sprintf(
			"the setting %s read %q before the tests and %q after them, so set it back with gsettings and treat this as a finding", key, was, now))
	}
	return complaints
}

// settingsAsTheyStand reads the two settings, leaving out any the machine cannot
// answer about, which is what a container with no desktop does.
func settingsAsTheyStand() map[string]string {
	standing := map[string]string{}
	for _, setting := range screenReaderSettings {
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

// variablesThatWakeTheScreenReader are the environment variables that let a
// program bring the desktop's accessibility bridge up. The session bus is on the
// list because a program handed it can ask the desktop to start the bridge, and
// on this machine that started Orca and it spoke aloud. None of them may reach
// the worker or the applications it launches.
var variablesThatWakeTheScreenReader = []string{
	"DBUS_SESSION_BUS_ADDRESS",
	"DBUS_SESSION_BUS_PID",
	"AT_SPI_BUS_ADDRESS",
	"GNOME_ACCESSIBILITY",
	"QT_LINUX_ACCESSIBILITY_ALWAYS_ON",
	"ACCESSIBILITY_ENABLED",
}

func TestTheWorkerIsHandedNothingThatWakesTheScreenReader(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	for _, name := range variablesThatWakeTheScreenReader {
		t.Setenv(name, "a value the worker must never see")
	}
	t.Setenv("GTK_MODULES", "gail:atk-bridge")
	t.Setenv("QT_ACCESSIBILITY", "1")

	handed := workerEnvironment()

	for _, line := range handed {
		name, _, _ := strings.Cut(line, "=")
		if slices.Contains(variablesThatWakeTheScreenReader, name) {
			t.Errorf("the desktop worker was handed %s, and a variable that lets a program wake the accessibility bridge must stay out of its"+
				" environment, because the driver it loads reads the accessibility tree and waking the bridge starts the screen reader", name)
		}
	}
	for _, want := range []string{"NO_AT_BRIDGE=1", "GTK_MODULES=", "QT_ACCESSIBILITY=0"} {
		if !slices.Contains(handed, want) {
			t.Errorf("the desktop worker was handed %v, want %q in it so that the toolkits it loads never bring the accessibility bridge up", handed, want)
		}
	}
}
