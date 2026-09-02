package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/command"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// sayingCommand is a command that answers with a fixed word and remembers the
// arguments it was handed, which is how the registry tests see what reached the
// command.
func sayingCommand(name string, answer string, seen *string) contract.Command {
	return contract.Command{
		Name: name,
		Help: "Says " + answer + ".",
		Run: func(_ context.Context, arguments string, _ contract.CommandContext) (string, error) {
			if seen != nil {
				*seen = arguments
			}
			return answer, nil
		},
	}
}

// terminalChannel is a fake channel named the way the terminal is, which is the
// one channel a terminal-only command may run on.
func terminalChannel() *testkit.FakeChannel {
	return testkit.NewFakeChannel(contract.TerminalChannelName)
}

func TestRegistryFindsACommandItWasGiven(t *testing.T) {
	registry := command.NewRegistry()
	if err := registry.Register(sayingCommand("status", "all is well", nil)); err != nil {
		t.Fatalf("registering the status command failed: %v", err)
	}

	found, there := registry.Lookup("status")
	if !there {
		t.Fatalf("the registry did not find the command it was just given")
	}
	if found.Name != "status" {
		t.Errorf("the registry gave back the command %q rather than status", found.Name)
	}

	if _, there := registry.Lookup("/status"); !there {
		t.Errorf("the registry did not find status when it was asked with the leading slash")
	}
	if _, there := registry.Lookup("nothing-like-this"); there {
		t.Errorf("the registry found a command it was never given")
	}
}

func TestRegistryRefusesASecondCommandWithTheSameName(t *testing.T) {
	registry := command.NewRegistry()
	if err := registry.Register(sayingCommand("status", "the first one", nil)); err != nil {
		t.Fatalf("registering the first status command failed: %v", err)
	}

	err := registry.Register(sayingCommand("status", "the second one", nil))
	if err == nil {
		t.Fatalf("the registry took two commands called status, so one would have hidden the other")
	}
	if !strings.Contains(err.Error(), "status") {
		t.Errorf("the refusal does not name the command that clashed: %v", err)
	}

	answer, err := registry.Run(context.Background(), "/status", contract.CommandContext{Channel: terminalChannel()})
	if err != nil {
		t.Fatalf("running the surviving status command failed: %v", err)
	}
	if answer != "the first one" {
		t.Errorf("the second registration replaced the first: the answer was %q", answer)
	}
}

func TestRegistryRefusesACommandItCouldNotAnswerWith(t *testing.T) {
	working := sayingCommand("status", "all is well", nil)
	for _, one := range []struct {
		what    string
		command contract.Command
	}{
		{"no name", contract.Command{Help: working.Help, Run: working.Run}},
		{"a name with a slash in it", contract.Command{Name: "/status", Help: working.Help, Run: working.Run}},
		{"a name with a space in it", contract.Command{Name: "two words", Help: working.Help, Run: working.Run}},
		{"no help line", contract.Command{Name: "status", Run: working.Run}},
		{"nothing to run", contract.Command{Name: "status", Help: working.Help}},
	} {
		t.Run(one.what, func(t *testing.T) {
			if err := command.NewRegistry().Register(one.command); err == nil {
				t.Fatalf("the registry took a command with %s", one.what)
			}
		})
	}
}

func TestRegistryKeepsTheOrderItWasGiven(t *testing.T) {
	registry := command.NewRegistry()
	for _, name := range []string{"help", "status", "model"} {
		if err := registry.Register(sayingCommand(name, name, nil)); err != nil {
			t.Fatalf("registering %s failed: %v", name, err)
		}
	}

	names := []string{}
	for _, one := range registry.All() {
		names = append(names, one.Name)
	}
	if strings.Join(names, " ") != "help status model" {
		t.Errorf("the registry gave back %q rather than the order it was given", strings.Join(names, " "))
	}
}

func TestHelpListsEveryCommandInRegistrationOrder(t *testing.T) {
	registry := command.NewRegistry()
	for _, one := range []contract.Command{
		{Name: "help", Help: "Shows this list.", Run: sayingCommand("help", "", nil).Run},
		{Name: "status", Help: "Shows the model, the cost so far, the jobs, the answers waiting, and the health of each channel.", Run: sayingCommand("status", "", nil).Run},
		{Name: "vault", Help: "Lists and manages the logins Coeus holds. Terminal only.", TerminalOnly: true, Run: sayingCommand("vault", "", nil).Run},
	} {
		if err := registry.Register(one); err != nil {
			t.Fatalf("registering %s failed: %v", one.Name, err)
		}
	}

	testkit.Golden(t, "help.golden", []byte(registry.Help()))
}

func TestRunHandsTheArgumentsToTheCommand(t *testing.T) {
	seen := ""
	registry := command.NewRegistry()
	if err := registry.Register(sayingCommand("model", "done", &seen)); err != nil {
		t.Fatalf("registering the model command failed: %v", err)
	}

	answer, err := registry.Run(context.Background(), "  /model   claude 4  ", contract.CommandContext{Channel: terminalChannel()})
	if err != nil {
		t.Fatalf("running the model command failed: %v", err)
	}
	if answer != "done" {
		t.Errorf("the registry gave back %q rather than the command's own answer", answer)
	}
	if seen != "claude 4" {
		t.Errorf("the command was handed %q rather than the arguments after its name", seen)
	}
}

func TestRunRefusesATerminalOnlyCommandOnAnotherChannel(t *testing.T) {
	ran := false
	registry := command.NewRegistry()
	if err := registry.Register(contract.Command{
		Name:         "vault",
		Help:         "Lists and manages the logins Coeus holds. Terminal only.",
		TerminalOnly: true,
		Run: func(_ context.Context, _ string, _ contract.CommandContext) (string, error) {
			ran = true
			return "the vault holds nothing", nil
		},
	}); err != nil {
		t.Fatalf("registering the vault command failed: %v", err)
	}

	answer, err := registry.Run(context.Background(), "/vault list", contract.CommandContext{Channel: testkit.NewFakeChannel("signal")})
	if err != nil {
		t.Fatalf("the refusal came back as an error rather than as an answer: %v", err)
	}
	if ran {
		t.Fatalf("the vault command ran on Signal, where what is typed cannot be hidden")
	}
	if !strings.Contains(answer, "terminal") {
		t.Errorf("the refusal does not tell the user where to go: %q", answer)
	}
	if strings.Contains(strings.TrimSpace(answer), "\n") {
		t.Errorf("the refusal is more than one line: %q", answer)
	}

	if _, err := registry.Run(context.Background(), "/vault list", contract.CommandContext{Channel: terminalChannel()}); err != nil {
		t.Fatalf("the vault command was refused in the terminal too: %v", err)
	}
	if !ran {
		t.Errorf("the vault command did not run in the terminal, where it is allowed")
	}
}

func TestRunRefusesATerminalOnlyCommandWithNoChannelAtAll(t *testing.T) {
	registry := command.NewRegistry()
	if err := registry.Register(contract.Command{
		Name:         "vault",
		Help:         "Lists and manages the logins Coeus holds. Terminal only.",
		TerminalOnly: true,
		Run: func(_ context.Context, _ string, _ contract.CommandContext) (string, error) {
			return "", errors.New("the vault command should never have run here")
		},
	}); err != nil {
		t.Fatalf("registering the vault command failed: %v", err)
	}

	answer, err := registry.Run(context.Background(), "/vault", contract.CommandContext{})
	if err != nil {
		t.Fatalf("a command with no channel came back as an error rather than as an answer: %v", err)
	}
	if !strings.Contains(answer, "terminal") {
		t.Errorf("the refusal does not tell the user where to go: %q", answer)
	}
}

func TestRunSaysWhenThereIsNoSuchCommand(t *testing.T) {
	registry := command.NewRegistry()
	for _, line := range []string{"/nothing-like-this", "nothing-like-this arguments"} {
		_, err := registry.Run(context.Background(), line, contract.CommandContext{Channel: terminalChannel()})
		if !errors.Is(err, command.ErrNoSuchCommand) {
			t.Errorf("running %q gave back %v rather than the no-such-command error", line, err)
		}
	}
}

func TestRunSaysWhenTheLineHoldsNoCommand(t *testing.T) {
	registry := command.NewRegistry()
	for _, line := range []string{"", "   ", "/", "  /  "} {
		_, err := registry.Run(context.Background(), line, contract.CommandContext{Channel: terminalChannel()})
		if !errors.Is(err, command.ErrNoSuchCommand) {
			t.Errorf("running %q gave back %v rather than the no-such-command error", line, err)
		}
	}
}

func TestRunHandsBackWhatTheCommandItselfFailedWith(t *testing.T) {
	broken := errors.New("the jobs could not be listed, so check that the database is readable")
	registry := command.NewRegistry()
	if err := registry.Register(contract.Command{
		Name: "jobs",
		Help: "Lists every job.",
		Run: func(_ context.Context, _ string, _ contract.CommandContext) (string, error) {
			return "", broken
		},
	}); err != nil {
		t.Fatalf("registering the jobs command failed: %v", err)
	}

	if _, err := registry.Run(context.Background(), "/jobs", contract.CommandContext{Channel: terminalChannel()}); !errors.Is(err, broken) {
		t.Errorf("the registry hid the command's own failure: %v", err)
	}
}

func TestSplitLineReadsTheNameAndTheArgumentsApart(t *testing.T) {
	for _, one := range []struct {
		line      string
		name      string
		arguments string
	}{
		{"/status", "status", ""},
		{"status", "status", ""},
		{"  /model  claude  ", "model", "claude"},
		{"/deny 3 the post is wrong", "deny", "3 the post is wrong"},
		{"/tasks\t17 back 3", "tasks", "17 back 3"},
		{"", "", ""},
		{"/", "", ""},
		{"//double", "/double", ""},
	} {
		name, arguments := command.SplitLine(one.line)
		if name != one.name || arguments != one.arguments {
			t.Errorf("splitting %q gave %q and %q rather than %q and %q", one.line, name, arguments, one.name, one.arguments)
		}
	}
}
