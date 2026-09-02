//go:build integration

package sandbox

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// onAThreadThatIsThrownAway runs one piece of work on an operating-system thread
// of its own and never gives that thread back, so the Go runtime ends it when the
// work is done.
//
// This is what makes the two restrictions testable at all. Landlock, the
// no-new-privileges flag, and a seccomp filter each apply to the thread that asks
// for them and can never be undone, so applying them on any thread the test
// framework will reuse would cripple the rest of the run. A thread that is thrown
// away carries them out of the process with it.
func onAThreadThatIsThrownAway(t *testing.T, work func() error) error {
	t.Helper()
	answer := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		answer <- work()
	}()
	return <-answer
}

// setnsWithABadFile calls setns with a file number that is not one. Without the
// filter the kernel answers "bad file number"; with it, "operation not
// permitted". Nothing on the machine changes either way, which is why this is
// the call the filter is proved with.
func setnsWithABadFile(t *testing.T) syscall.Errno {
	t.Helper()
	for _, call := range deniedSystemCalls {
		if call.name != "setns" {
			continue
		}
		_, _, errorNumber := syscall.Syscall(uintptr(call.number), ^uintptr(0), 0, 0)
		return errorNumber
	}
	t.Fatal("the deny list has no setns in it, and this test proves the filter through it")
	return 0
}

func TestTheRestrictionsHoldOnTheThreadThatAsksForThemAndNowhereElse(t *testing.T) {
	work := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("readable before the fence"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the fixture file: %v", err)
	}

	err := onAThreadThatIsThrownAway(t, func() error {
		if before := setnsWithABadFile(t); before != syscall.EBADF {
			return errors.New("setns with a bad file number did not answer with a bad file number before the filter was on")
		}
		if err := setNoNewPrivileges(); err != nil {
			return err
		}
		if _, err := applyLandlock([]string{"/usr", "/bin", "/lib", "/lib64", "/etc"}, []string{work}); err != nil {
			return err
		}
		if err := applySeccomp(buildSeccompProgram(seccompArchitecture, deniedSystemCalls, unshareSystemCall)); err != nil {
			return err
		}
		return checkTheFenceHolds(t, work, outside)
	})
	if err != nil {
		t.Fatalf("the restrictions did not do what they promise: %v", err)
	}

	if _, err := os.ReadFile(outside); err != nil {
		t.Errorf("the rest of this program lost its access to %s, and the restriction was meant for one thread only: %v", outside, err)
	}
}

// checkTheFenceHolds is what the thrown-away thread asserts once it is fenced in:
// it cannot read outside, it can write inside, and a denied system call comes
// back as "operation not permitted".
func checkTheFenceHolds(t *testing.T, work string, outside string) error {
	if _, err := os.ReadFile(outside); err == nil {
		return errors.New("a fenced thread read a file outside every folder it was allowed")
	}
	if err := os.WriteFile(filepath.Join(work, "inside.txt"), []byte("written"), contract.DataFileMode); err != nil {
		return err
	}
	if after := setnsWithABadFile(t); after != syscall.EPERM {
		return errors.New("setns came back with something other than \"operation not permitted\", so the filter is not on")
	}
	return nil
}

func TestTheHelperAppliesEverythingAndThenTriesToBecomeTheCommand(t *testing.T) {
	t.Setenv(FenceMarkerVariable, fenceMarkerValue)
	work := t.TempDir()
	said := &strings.Builder{}

	err := onAThreadThatIsThrownAway(t, func() error {
		return Entry([]string{
			"--read", "/usr", "--read", "/bin", "--read", "/lib", "--read", "/lib64", "--read", "/etc",
			"--write", work,
			"--", filepath.Join(work, "no-such-program"),
		}, said)
	})

	if err == nil {
		t.Fatal("the helper reported that it became a program that is not on disk")
	}
	if !strings.Contains(err.Error(), "no-such-program") {
		t.Errorf("the helper said %q, and it must name the program it could not become", err)
	}
	if !strings.Contains(said.String(), "landlock version") {
		t.Errorf("the helper wrote %q to its error output, and when it fails it must say in one line how far it got", said)
	}
}

func TestTheHelperSaysNothingAtAllWhenItHasNothingToReport(t *testing.T) {
	t.Setenv(FenceMarkerVariable, fenceMarkerValue)
	work := t.TempDir()
	said := &strings.Builder{}

	// The helper cannot be watched succeeding from in here, because on success it
	// becomes another program and never comes back. What can be checked is that
	// nothing is written before that moment, which is what keeps a shell result
	// free of a line the model would have to read on every single call.
	err := onAThreadThatIsThrownAway(t, func() error {
		return Entry([]string{"--write", work, "--", filepath.Join(work, "no-such-program")}, said)
	})
	if err == nil {
		t.Fatal("the helper reported that it became a program that is not on disk")
	}

	wrote := said.String()
	if strings.Count(wrote, "\n") != 1 {
		t.Errorf("the helper wrote %q, want the one line it writes only when it could not become the command", wrote)
	}
	if strings.Contains(wrote, "running ") {
		t.Errorf("the helper wrote %q, and it must not announce a command it never started", wrote)
	}
}

func TestTheHelperSaysSoWhenTheFoldersItWasGivenCannotBeOpened(t *testing.T) {
	t.Setenv(FenceMarkerVariable, fenceMarkerValue)

	err := onAThreadThatIsThrownAway(t, func() error {
		return Entry([]string{"--read", "/proc/self/mem/not-a-folder", "--", "/bin/true"}, io.Discard)
	})
	if err == nil {
		t.Fatal("the helper accepted a folder it cannot open")
	}
}
