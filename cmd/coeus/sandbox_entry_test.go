package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/sandbox"
)

func TestTheSandboxEntrySubcommandIsNamedWhatTheSandboxExpects(t *testing.T) {
	if sandboxEntrySubcommand.name != sandbox.EntrySubcommandName {
		t.Errorf("the subcommand is named %q, and the sandbox starts %q", sandboxEntrySubcommand.name, sandbox.EntrySubcommandName)
	}
	if sandboxEntrySubcommand.help == "" || sandboxEntrySubcommand.run == nil {
		t.Error("the subcommand has no help line or nothing to run, and the registry refuses either")
	}
	if !strings.Contains(sandboxEntrySubcommand.help, "not") {
		t.Errorf("the help line is %q, and it must say that a person is not meant to type this", sandboxEntrySubcommand.help)
	}
}

func TestTheSandboxEntrySubcommandReportsWhatWentWrongAndAskedForTheRightExitCode(t *testing.T) {
	t.Setenv(sandbox.FenceMarkerVariable, "")
	output := &bytes.Buffer{}
	problems := &bytes.Buffer{}

	code := sandboxEntrySubcommand.run([]string{"--write", "/tmp", "--", "/bin/true"}, output, problems)

	if code != contract.ExitFailure {
		t.Errorf("the subcommand reported %d, want %d, because it could not do what it was asked", code, contract.ExitFailure)
	}
	if !strings.Contains(problems.String(), sandbox.EntrySubcommandName) {
		t.Errorf("the subcommand said %q, and it must name itself so that a reader knows what refused", problems)
	}
	if output.Len() != 0 {
		t.Errorf("the subcommand wrote %q to its ordinary output, and only the command it runs may write there", output)
	}
}
