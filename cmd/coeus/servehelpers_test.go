package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/config"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/reliability"
	"github.com/JaredTate/coeus/internal/update"
)

// TestWriteWhatTheGuardFoundSaysOnlyWhatHappened holds that the startup report
// names every thing the last life left behind and says nothing when it left
// nothing, so a clean start is silent and a damaged one is explained.
func TestWriteWhatTheGuardFoundSaysOnlyWhatHappened(t *testing.T) {
	found := reliability.Startup{
		UncleanExit:      true,
		DatabaseMovedTo:  "/tmp/old.db",
		RestoredFrom:     "/tmp/archive.db",
		BreakerTripped:   true,
		RepliesSentAgain: 2,
	}
	written := &strings.Builder{}
	writeWhatTheGuardFound(written, found)
	said := written.String()
	for _, want := range []string{"did not exit cleanly", "/tmp/old.db", "/tmp/archive.db", "crashed several times", "2 replies"} {
		if !strings.Contains(said, want) {
			t.Errorf("the startup report %q does not mention %q", said, want)
		}
	}

	quiet := &strings.Builder{}
	writeWhatTheGuardFound(quiet, reliability.Startup{})
	if quiet.String() != "" {
		t.Errorf("a clean start still said %q, and it should say nothing", quiet.String())
	}
}

// TestWhichBinaryIsServingNamesTheProgram holds that the first line of the log
// says which build is running, so a person reading it knows a worktree binary
// from the installed one.
func TestWhichBinaryIsServingNamesTheProgram(t *testing.T) {
	line := whichBinaryIsServing()
	for _, want := range []string{"coeus", "commit", "serving from"} {
		if !strings.Contains(line, want) {
			t.Errorf("the serving line %q does not carry %q", line, want)
		}
	}
}

// TestExitCodeForStopsTheServiceOnlyForWhatARestartCannotFix holds that a bad
// configuration and a database from a newer Coeus stop the service, while
// everything else is a failure a restart may fix.
func TestExitCodeForStopsTheServiceOnlyForWhatARestartCannotFix(t *testing.T) {
	if got := exitCodeFor(config.Problem{Advice: "the rounds cap is not a number"}); got != contract.ExitBadConfiguration {
		t.Errorf("a bad configuration exited with %d, want %d so the service waits for a person", got, contract.ExitBadConfiguration)
	}
	if got := exitCodeFor(update.ErrDatabaseFromANewerCoeus); got != contract.ExitBadConfiguration {
		t.Errorf("a database from a newer Coeus exited with %d, want %d", got, contract.ExitBadConfiguration)
	}
	if got := exitCodeFor(errors.New("the socket could not be opened")); got != contract.ExitFailure {
		t.Errorf("a passing trouble exited with %d, want %d so the service tries again", got, contract.ExitFailure)
	}
}
