package main

// What the wave 6 security review found on the sudo path. Design section 11
// says a shell call with the escalate field and an approved preview runs
// outside the sandbox with the sudo password from the vault, and the askpass
// subcommand is how that password reaches sudo.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/config"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/vault"
)

// aVaultHoldingTheSudoPassword opens a vault at one home, puts the sudo password
// in it, and closes it again, so that the subcommand has something to find.
func aVaultHoldingTheSudoPassword(t *testing.T, home contract.Home) {
	t.Helper()
	if err := os.MkdirAll(home.Root, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the home folder %s: %v", home.Root, err)
	}
	opened, err := vault.Open(home, testkit.NewFakeClock(time.Unix(0, 0).UTC()))
	if err != nil {
		t.Fatalf("opening the vault failed: %v", err)
	}
	if err := opened.Add(vault.Entry{Name: vault.SudoEntryName, Password: "the-machine-password"}); err != nil {
		t.Fatalf("adding the sudo entry failed: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("closing the vault failed: %v", err)
	}
}

// TestAskpassReadsTheHomeTheRestOfCoeusReads holds the rule that COEUS_HOME
// moves the whole home folder. Every other subcommand asks config.HomeFolder for
// its paths; askpass calls contract.DefaultHome, which reads only HOME.
func TestAskpassReadsTheHomeTheRestOfCoeusReads(t *testing.T) {
	moved := contract.NewHome(filepath.Join(t.TempDir(), "moved-home"))
	aVaultHoldingTheSudoPassword(t, moved)
	t.Setenv("HOME", t.TempDir())
	t.Setenv(config.HomeVariable, moved.Root)

	var output, problems bytes.Buffer
	if code := askpassSubcommand.run(nil, &output, &problems); code != contract.ExitOK {
		t.Errorf("askpass left with %d and said %q; %s moved the home folder and askpass looked under HOME instead,"+
			" so an approved sudo command finds no password on any machine where the home has been moved",
			code, problems.String(), config.HomeVariable)
	}
}

// TestAskpassFindsTheVaultUnderTheEnvironmentSudoIsGiven holds the rule for the
// one caller there is. internal/tool/shell/escalate.go runWithSudo sets
// HOME to the agent's own home folder before it runs sudo, so the askpass helper
// inherits that HOME and looks for the vault one folder deeper than it is.
func TestAskpassFindsTheVaultUnderTheEnvironmentSudoIsGiven(t *testing.T) {
	home := testkit.NewTempHome(t)
	aVaultHoldingTheSudoPassword(t, home)
	t.Setenv("HOME", home.Root)

	var output, problems bytes.Buffer
	code := askpassSubcommand.run(nil, &output, &problems)

	stray := filepath.Join(home.Root, contract.HomeFolderName, "vault.key")
	if _, err := os.Stat(stray); err == nil {
		t.Errorf("askpass made a second age private key at %s; it opened a vault that was not there rather than saying it could not find one,"+
			" so a key nothing manages is left on disk every time sudo asks", stray)
	}
	if code != contract.ExitOK {
		t.Errorf("askpass left with %d and said %q under the environment the shell tool hands sudo, so no approved command with"+
			" administrator powers can ever get the password", code, problems.String())
	}
}
