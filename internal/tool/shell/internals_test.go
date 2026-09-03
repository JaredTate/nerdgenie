package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
