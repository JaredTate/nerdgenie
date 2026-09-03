package browser

import (
	"bufio"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A command with nothing in it, a profile that is not named, and a pacing that
// is neither of the two are all refused before anything is started.
func TestTheWorkerCommandIsCheckedBeforeAnythingStarts(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "profile")
	for _, refusal := range []struct {
		name    string
		command []string
		profile string
		pacing  Pacing
		says    string
	}{
		{name: "no command", command: nil, profile: profile, pacing: PacingHuman, says: "command is empty"},
		{name: "a blank program", command: []string{"  "}, profile: profile, pacing: PacingHuman, says: "command is empty"},
		{name: "no profile", command: []string{"node"}, profile: " ", pacing: PacingHuman, says: "its own profile folder"},
		{name: "an unknown pacing", command: []string{"node"}, profile: profile, pacing: "sleepy", says: "one of human or fast"},
	} {
		t.Run(refusal.name, func(t *testing.T) {
			_, err := ProcessStart(refusal.command, refusal.profile, refusal.pacing, nil)
			if err == nil || !strings.Contains(err.Error(), refusal.says) {
				t.Fatalf("that was refused with %v, and it should have said %q", err, refusal.says)
			}
		})
	}
}

// The profile folder is made before the first launch and is readable by nobody
// but this account, because its cookies are the agent's logins.
func TestTheProfileFolderIsMadePrivate(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "browser", "default")
	if err := makeProfileFolder(profile); err != nil {
		t.Fatalf("making the profile folder failed: %v", err)
	}
	about, err := os.Stat(profile)
	if err != nil {
		t.Fatalf("the profile folder was not made: %v", err)
	}
	if about.Mode().Perm() != fs.FileMode(0o700) {
		t.Fatalf("the profile folder is mode %v, and it must be 0700", about.Mode().Perm())
	}

	shared := filepath.Join(t.TempDir(), "shared")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatalf("making the folder to test with failed: %v", err)
	}
	if err := checkProfileIsPrivate(shared); err == nil {
		t.Fatal("a profile folder anybody could read should have been refused")
	}
	if err := checkProfileIsPrivate(filepath.Join(shared, "not there")); err == nil {
		t.Fatal("a profile folder that is not there should have been refused")
	}
}

// A worker started as a child process talks over its pipes, its own logging goes
// to this program's log, and it is stopped by the exact process identifier it
// was started with.
func TestAChildProcessTalksOverItsPipesAndIsStoppedByItsProcessID(t *testing.T) {
	notes := &noteBook{}
	profile := filepath.Join(t.TempDir(), "profile")
	start, err := ProcessStart([]string{"/bin/sh", "-c", "echo the worker started >&2; exec cat"},
		profile, PacingHuman, notes.write)
	if err != nil {
		t.Fatalf("building the start function failed: %v", err)
	}

	connection, err := start(context.Background())
	if err != nil {
		t.Fatalf("starting the child process failed: %v", err)
	}
	if connection.ProcessID <= 0 {
		t.Fatalf("the child process was started with process id %d, and it should be a real one", connection.ProcessID)
	}
	if _, err := connection.Requests.Write([]byte("a line the worker echoes\n")); err != nil {
		t.Fatalf("writing to the child process failed: %v", err)
	}
	line, err := bufio.NewReader(connection.Responses).ReadString('\n')
	if err != nil || line != "a line the worker echoes\n" {
		t.Fatalf("the child process answered %q and %v, and it should have echoed the line", line, err)
	}
	if err := connection.Stop(); err != nil {
		t.Fatalf("stopping the child process failed: %v", err)
	}
	waitUntil(t, "the worker's own log line to arrive", func() bool {
		return strings.Contains(notes.written(), "browser worker: the worker started")
	})
}

// A program that is not on the machine is reported by name rather than left to
// fail later.
func TestAWorkerProgramThatIsNotThereIsReportedByName(t *testing.T) {
	start, err := ProcessStart([]string{"a-program-that-is-not-installed"}, filepath.Join(t.TempDir(), "profile"), PacingFast, nil)
	if err != nil {
		t.Fatalf("building the start function failed: %v", err)
	}
	if _, err := start(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "a-program-that-is-not-installed") {
		t.Fatalf("starting a program that is not there said %v, and it should have named the program", err)
	}
}

// The worker is handed the display it draws on and nothing secret.
func TestTheWorkerIsHandedTheDisplayAndNothingSecret(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	t.Setenv("ANTHROPIC_API_KEY", "a-key-the-worker-has-no-business-holding")

	handed := workerEnvironment()
	found := false
	for _, line := range handed {
		if strings.HasPrefix(line, "ANTHROPIC_API_KEY=") {
			t.Fatalf("the worker was handed %q, and it has no business holding a key", line)
		}
		if line == "DISPLAY=:0" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the worker was handed %v, and it needs the display to draw its window on", handed)
	}
}

// A test run must not put a Chrome window on the screen of whoever is running
// it, so `make test-browser` sets COEUS_HEADLESS_TESTS and the worker is asked
// for a browser with no window. Nothing else sets that name, so an ordinary run
// still opens the visible window design section 11 asks for: a logged-in account
// is only safe in a window the user can see.
func TestChromeIsAskedToGoHeadlessOnlyWhenTheTestsAskForIt(t *testing.T) {
	command := []string{"node", "main.js"}
	profile := "/tmp/a-profile"

	t.Setenv(headlessVariable, "")
	ordinary := workerArguments(command, profile, PacingHuman)
	if slices.Contains(ordinary, headlessFlag) {
		t.Errorf("an ordinary run starts the worker with %v, and %s is in it; the browser the person uses has a window they can see",
			ordinary, headlessFlag)
	}

	t.Setenv(headlessVariable, "1")
	underTest := workerArguments(command, profile, PacingFast)
	if !slices.Contains(underTest, headlessFlag) {
		t.Errorf("with %s set the worker is started with %v, and %s is not in it, so a test run opens a Chrome window on the person's screen",
			headlessVariable, underTest, headlessFlag)
	}
	if underTest[0] != "node" || underTest[1] != "main.js" {
		t.Errorf("the worker is started with %v, and the program and its own arguments have to come first", underTest)
	}
}

// TestTheWorkerOutlivesTheCallThatStartedIt pins the fix for the browser worker
// dying with the first tool call: the worker lives across calls, so the context
// of the call that happened to start it must not end its process. Only Stop, by
// the exact process id, ends it.
func TestTheWorkerOutlivesTheCallThatStartedIt(t *testing.T) {
	notes := &noteBook{}
	start, err := ProcessStart([]string{"/bin/sh", "-c", "exec cat"}, filepath.Join(t.TempDir(), "profile"), PacingHuman, notes.write)
	if err != nil {
		t.Fatalf("building the start function failed: %v", err)
	}
	firstCall, callEnded := context.WithCancel(context.Background())
	connection, err := start(firstCall)
	if err != nil {
		t.Fatalf("starting the worker failed: %v", err)
	}
	callEnded()
	time.Sleep(100 * time.Millisecond)
	if err := syscall.Kill(connection.ProcessID, 0); err != nil {
		t.Fatalf("the worker with process id %d died when the call that started it ended (%v), and it must live until Stop", connection.ProcessID, err)
	}
	if _, err := connection.Requests.Write([]byte("still here\n")); err != nil {
		t.Fatalf("writing to the worker after the first call ended failed: %v", err)
	}
	if err := connection.Stop(); err != nil {
		t.Fatalf("stopping the worker failed: %v", err)
	}
	if syscall.Kill(connection.ProcessID, 0) == nil {
		t.Fatalf("the worker with process id %d is still alive after Stop", connection.ProcessID)
	}
}
