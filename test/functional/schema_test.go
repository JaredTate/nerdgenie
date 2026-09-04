// The whole-program test for the one thing an older binary must never do: open
// a database a newer Nerd Genie has already migrated. It stops rather than reading it
// wrong, and it stops in the way the service manager reads as "do not restart",
// because starting again every five seconds against a file it will never
// understand helps nobody.
package functional

import (
	"context"
	"database/sql"
	"os/exec"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/log"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/update"

	// The pure-Go SQLite driver, so that this test can write the one number it
	// needs into the file the agent will open.
	_ "modernc.org/sqlite"
)

func TestServeStopsOnADatabaseFromANewerNerdGenie(t *testing.T) {
	agent := startTheAgentWorkingIn(t, func(string) testkit.Script {
		return testkit.Script{Name: "local", ContextLength: 32768}
	}, func(home contract.Home, _ string) {
		makeADatabaseFromTheFuture(t, home)
	}, doNotWaitForTheSocket)

	// The code is asked for first, because asking waits for the program to end
	// and nothing it says is on disk until it has.
	if code := agent.exitCode(t); code != contract.ExitBadConfiguration {
		t.Errorf("nerdgenie serve left with %d, want %d, which is the code the service unit reads as do not restart",
			code, contract.ExitBadConfiguration)
	}
	if said := whatItSaid(agent.saidPath); !strings.Contains(said, "newer version") {
		t.Errorf("the agent did not say the database was written by a newer Nerd Genie:\n%s", said)
	}
}

// makeADatabaseFromTheFuture writes a real event log and then sets its schema
// version one above the one this binary knows, which is what a home folder looks
// like after a newer Nerd Genie has run on it.
func makeADatabaseFromTheFuture(t *testing.T, home contract.Home) {
	t.Helper()
	ctx := context.Background()
	made, err := log.Open(ctx, home.DatabaseFile())
	if err != nil {
		t.Fatalf("making the event log failed: %v", err)
	}
	if err := made.Close(); err != nil {
		t.Fatalf("closing the event log failed: %v", err)
	}

	database, err := sql.Open("sqlite", home.DatabaseFile())
	if err != nil {
		t.Fatalf("opening the database to age it failed: %v", err)
	}
	defer func() { _ = database.Close() }()
	if _, err := database.ExecContext(ctx, "UPDATE schema_version SET version = ?", update.SchemaVersion()+1); err != nil {
		t.Fatalf("writing the newer schema version failed: %v", err)
	}
}

// exitCode is the code the agent left with, for a test about a program that is
// meant to stop rather than serve.
func (agent runningAgent) exitCode(t *testing.T) int {
	t.Helper()
	err := agent.wait()
	if err == nil {
		t.Fatal("nerdgenie serve came up and kept serving on a database it cannot read")
	}
	quit, isExit := err.(*exec.ExitError)
	if !isExit {
		t.Fatalf("nerdgenie serve did not run at all: %v", err)
	}
	return quit.ExitCode()
}
