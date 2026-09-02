package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

func TestRunningWithNoArgumentsPrintsTheListAndSaysTheCommandLineWasWrong(t *testing.T) {
	var output, problems bytes.Buffer

	code := run(nil, &output, &problems)

	if code != contract.ExitUsage {
		t.Errorf("running with no arguments returned %d, want %d", code, contract.ExitUsage)
	}
	if !strings.Contains(output.String(), "version") {
		t.Errorf("the printed list does not hold the version subcommand:\n%s", output.String())
	}
}

func TestTheHelpSubcommandPrintsEverySubcommandWithItsHelpLine(t *testing.T) {
	var output, problems bytes.Buffer

	code := run([]string{"help"}, &output, &problems)

	if code != contract.ExitOK {
		t.Errorf("the help subcommand returned %d, want %d", code, contract.ExitOK)
	}
	for _, command := range subcommands() {
		if !strings.Contains(output.String(), command.name) {
			t.Errorf("the list does not hold %q:\n%s", command.name, output.String())
		}
		if !strings.Contains(output.String(), command.help) {
			t.Errorf("the list does not hold the help line for %q:\n%s", command.name, output.String())
		}
	}
}

func TestTheVersionSubcommandPrintsTheVersion(t *testing.T) {
	var output, problems bytes.Buffer

	code := run([]string{"version"}, &output, &problems)

	if code != contract.ExitOK {
		t.Errorf("the version subcommand returned %d, want %d", code, contract.ExitOK)
	}
	if strings.TrimSpace(output.String()) != version {
		t.Errorf("the version printed as %q, want %q", strings.TrimSpace(output.String()), version)
	}
	if version != "dev" {
		t.Errorf("the version defaults to %q, want dev until a release sets it with -ldflags", version)
	}
}

func TestASubcommandNobodyWroteSaysSoAndPointsAtTheList(t *testing.T) {
	var output, problems bytes.Buffer

	code := run([]string{"fly"}, &output, &problems)

	if code != contract.ExitUsage {
		t.Errorf("an unknown subcommand returned %d, want %d", code, contract.ExitUsage)
	}
	if !strings.Contains(problems.String(), "fly") {
		t.Errorf("the message %q does not name the subcommand that was asked for", problems.String())
	}
	if !strings.Contains(problems.String(), "coeus help") {
		t.Errorf("the message %q does not say how to see the list", problems.String())
	}
}

func TestEverySubcommandInTheTableHasANameAHelpLineAndSomethingToRun(t *testing.T) {
	seen := map[string]bool{}
	for _, command := range subcommands() {
		if command.name == "" || command.help == "" || command.run == nil {
			t.Errorf("the subcommand %+v is missing its name, its help line, or its function", command)
		}
		if seen[command.name] {
			t.Errorf("the subcommand %q is in the table twice", command.name)
		}
		seen[command.name] = true
	}
}
