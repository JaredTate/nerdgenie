package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/vault"
)

// vaultInATemporaryHome opens a vault under a home the test framework throws
// away, and points the HOME variable at it, so that the askpass subcommand
// finds that vault and never the real one.
func vaultInATemporaryHome(t *testing.T) *vault.Vault {
	t.Helper()
	home := testkit.NewTempHome(t)
	opened, err := vault.Open(home, testkit.NewFakeClock(time.Unix(0, 0).UTC()))
	if err != nil {
		t.Fatalf("opening the vault in a temporary home failed: %v", err)
	}
	t.Cleanup(func() { _ = opened.Close() })
	return opened
}

func TestAskpassPrintsTheSudoPasswordWhenTheVaultHoldsOne(t *testing.T) {
	opened := vaultInATemporaryHome(t)
	if err := opened.Add(vault.Entry{Name: vault.SudoEntryName, Password: "the-machine-password"}); err != nil {
		t.Fatalf("adding the sudo entry failed: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("closing the vault failed: %v", err)
	}

	var output, problems bytes.Buffer
	code := askpassSubcommand.run(nil, &output, &problems)

	if code != contract.ExitOK {
		t.Errorf("askpass returned %d, want %d; it said %q", code, contract.ExitOK, problems.String())
	}
	if strings.TrimRight(output.String(), "\n") != "the-machine-password" {
		t.Errorf("askpass printed %q, want the sudo password on its own line", output.String())
	}
	if problems.String() != "" {
		t.Errorf("askpass complained about %q when it had a password to print", problems.String())
	}
}

func TestAskpassSaysWhatToDoWhenThereIsNoSudoPassword(t *testing.T) {
	vaultInATemporaryHome(t)

	var output, problems bytes.Buffer
	code := askpassSubcommand.run(nil, &output, &problems)

	if code == contract.ExitOK {
		t.Errorf("askpass said it worked when the vault holds no sudo password")
	}
	if output.String() != "" {
		t.Errorf("askpass printed %q on the output sudo reads, and it must print nothing at all", output.String())
	}
	if !strings.Contains(problems.String(), "/vault add sudo") {
		t.Errorf("askpass said %q and does not say how to add the sudo password", problems.String())
	}
}

func TestAskpassTakesNoArguments(t *testing.T) {
	opened := vaultInATemporaryHome(t)
	if err := opened.Add(vault.Entry{Name: vault.SudoEntryName, Password: "the-machine-password"}); err != nil {
		t.Fatalf("adding the sudo entry failed: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("closing the vault failed: %v", err)
	}

	var output, problems bytes.Buffer
	code := askpassSubcommand.run([]string{"a prompt sudo passed along"}, &output, &problems)

	if code != contract.ExitOK {
		t.Errorf("askpass returned %d when sudo passed it a prompt, want %d", code, contract.ExitOK)
	}
	if strings.TrimRight(output.String(), "\n") != "the-machine-password" {
		t.Errorf("askpass printed %q, want the sudo password on its own line", output.String())
	}
}

func TestAskpassSaysWhenTheVaultWillNotOpen(t *testing.T) {
	t.Setenv("HOME", "")

	var output, problems bytes.Buffer
	code := askpassSubcommand.run(nil, &output, &problems)

	if code == contract.ExitOK {
		t.Errorf("askpass said it worked with no home folder to read")
	}
	if output.String() != "" {
		t.Errorf("askpass printed %q on the output sudo reads when it could not open the vault", output.String())
	}
	if problems.String() == "" {
		t.Errorf("askpass said nothing about why it could not open the vault")
	}
}

func TestTheAskpassSubcommandIsNamedAndDescribed(t *testing.T) {
	if askpassSubcommand.name != "askpass" {
		t.Errorf("the subcommand is named %q, want askpass", askpassSubcommand.name)
	}
	if askpassSubcommand.help == "" {
		t.Errorf("the subcommand has no help line")
	}
	if askpassSubcommand.run == nil {
		t.Errorf("the subcommand has nothing to run")
	}
}
