package shell

import (
	"context"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTheCommandLineTakesBashWhereThereIsOneAndSlashBinSlashShWhereThereIsNot
// is the other half of brief 6.6's shell finding. A machine with bash runs
// every command through it with pipefail set, so that a failing command in a
// pipe reports its own code; a machine without bash falls back to /bin/sh and
// says nothing about pipefail, because dash refuses the option and would fail
// every command instead of one.
func TestTheCommandLineTakesBashWhereThereIsOneAndSlashBinSlashShWhereThereIsNot(t *testing.T) {
	folder := t.TempDir()
	pretendBash := filepath.Join(folder, "bash")
	if err := os.WriteFile(pretendBash, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("cannot write the bash this test stands in for: %v", err)
	}
	looking := bashPlaces
	t.Cleanup(func() { bashPlaces = looking })

	bashPlaces = []string{filepath.Join(folder, "no-bash-here"), pretendBash}
	program, arguments := CommandLine("go test ./... | tail -40")
	if program != pretendBash {
		t.Errorf("a command runs through %q, want the bash at %q", program, pretendBash)
	}
	if strings.Join(arguments, " ") != "-o pipefail -c go test ./... | tail -40" {
		t.Errorf("bash was given %v, want -o pipefail -c and the command", arguments)
	}

	bashPlaces = []string{filepath.Join(folder, "no-bash-here"), folder}
	program, arguments = CommandLine("go test ./... | tail -40")
	if program != "/bin/sh" {
		t.Errorf("a command on a machine with no bash runs through %q, want /bin/sh", program)
	}
	if strings.Join(arguments, " ") != "-c go test ./... | tail -40" {
		t.Errorf("/bin/sh was given %v, want -c and the command with nothing else", arguments)
	}
}

// TestAServedCommandIsNotOnTheCommandTimeout: on run 25 every server the
// model started with serve died about ten minutes later, because a served
// command got the same timeout as an ordinary one and was killed when it
// ran out; the page checks at the next task's end then found nothing on the
// port. A serve is meant to keep running, so it gets ServeTimeout, which is
// long, and the model's kill or the end of the program is what stops it.
func TestAServedCommandIsNotOnTheCommandTimeout(t *testing.T) {
	running := newTable()
	served, err := running.add("npm start", ServeTimeout, time.Unix(1700000000, 0), func(ctx context.Context) (contract.SandboxResult, error) {
		<-ctx.Done()
		return contract.SandboxResult{}, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer served.stop()
	if served.timeout != ServeTimeout || ServeTimeout < 12*time.Hour {
		t.Errorf("a served command's timeout is %v, want ServeTimeout of at least half a day, not the command's", served.timeout)
	}
	if serveTimeoutFor(time.Minute) != ServeTimeout {
		t.Errorf("the serve action hands its command %v, want ServeTimeout", serveTimeoutFor(time.Minute))
	}
}
