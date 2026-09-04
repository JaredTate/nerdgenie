package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

func TestTheTerminalScreenSubcommandIsNamedAndDescribed(t *testing.T) {
	if tuiSubcommand.name != "tui" {
		t.Errorf("the subcommand is called %q, and the orchestrator registers it as \"tui\"", tuiSubcommand.name)
	}
	if tuiSubcommand.help == "" {
		t.Error("the subcommand has no help line, and the listing prints one for every subcommand")
	}
	if tuiSubcommand.run == nil {
		t.Error("the subcommand has nothing to run")
	}
}

func TestTheTerminalScreenRefusesArgumentsItDoesNotUnderstand(t *testing.T) {
	output := bytes.Buffer{}
	problems := bytes.Buffer{}

	code := tuiSubcommand.run([]string{"--wide"}, &output, &problems)

	if code != contract.ExitUsage {
		t.Errorf("an argument the screen does not take gave exit code %d, and it should be %d", code, contract.ExitUsage)
	}
	if !strings.Contains(problems.String(), "--wide") {
		t.Errorf("the message is %q, and it should name the argument that was not understood", problems.String())
	}
}

func TestTheTerminalScreenSaysSoWhenItCannotFindTheHomeFolder(t *testing.T) {
	t.Setenv("NERDGENIE_HOME", "")
	t.Setenv("HOME", "")
	output := bytes.Buffer{}
	problems := bytes.Buffer{}

	code := tuiSubcommand.run(nil, &output, &problems)

	if code == contract.ExitOK {
		t.Error("the screen said all was well with no home folder to find")
	}
	if problems.Len() == 0 {
		t.Error("the screen said nothing about not being able to find the home folder")
	}
}
