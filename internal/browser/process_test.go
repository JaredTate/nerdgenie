package browser

import (
	"bufio"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
