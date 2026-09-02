package main

import (
	"context"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// exampleCommand is a slash command a test registers.
func exampleCommand(name string) contract.Command {
	return contract.Command{
		Name: name,
		Help: "Does nothing, and says so.",
		Run: func(_ context.Context, _ string, _ contract.CommandContext) (string, error) {
			return "nothing happened", nil
		},
	}
}

func TestTheCommandRegistryKeepsCommandsInTheOrderTheyWereRegistered(t *testing.T) {
	registry := newCommandRegistry()

	for _, name := range []string{"status", "help", "tasks"} {
		if err := registry.Register(exampleCommand(name)); err != nil {
			t.Fatalf("registering %q failed: %v", name, err)
		}
	}

	all := registry.All()
	if len(all) != 3 {
		t.Fatalf("the registry holds %d commands, want 3", len(all))
	}
	if all[0].Name != "status" || all[2].Name != "tasks" {
		t.Errorf("the registry holds %q first and %q last, want the order they were registered", all[0].Name, all[2].Name)
	}
}

func TestTheCommandRegistryFindsACommandByItsName(t *testing.T) {
	registry := newCommandRegistry()
	if err := registry.Register(exampleCommand("status")); err != nil {
		t.Fatalf("registering failed: %v", err)
	}

	if _, found := registry.Lookup("status"); !found {
		t.Error("the registry could not find the command that was just registered")
	}
	if _, found := registry.Lookup("nothing"); found {
		t.Error("the registry found a command nobody registered")
	}
}

func TestTheCommandRegistryRefusesACommandItCannotUse(t *testing.T) {
	registry := newCommandRegistry()
	if err := registry.Register(exampleCommand("status")); err != nil {
		t.Fatalf("registering failed: %v", err)
	}

	tests := []struct {
		name    string
		command contract.Command
	}{
		{"a name that is already taken", exampleCommand("status")},
		{"no name at all", exampleCommand("")},
		{"a name with a slash on it", exampleCommand("/status")},
		{"no help line", contract.Command{Name: "tasks", Run: exampleCommand("tasks").Run}},
		{"nothing to run", contract.Command{Name: "tasks", Help: "Shows the tasks."}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := registry.Register(test.command); err == nil {
				t.Fatalf("a command with %s was accepted, want an error saying what is wrong", test.name)
			}
		})
	}
}
