package main

import (
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// commandRegistry holds every slash command the program answers, in the order
// they were registered.
//
// Each package exports the commands it owns as contract.Command values, and the
// orchestrator registers them here in serve.go, which arrives in wave 3. Keeping
// registration in one file the orchestrator owns is what stops two workers of the
// same wave from editing the same lines.
type commandRegistry struct {
	order  []string
	byName map[string]contract.Command
}

// newCommandRegistry returns an empty registry.
func newCommandRegistry() *commandRegistry {
	return &commandRegistry{byName: map[string]contract.Command{}}
}

// Register puts one command in the registry, refusing anything the program could
// not answer with.
func (registry *commandRegistry) Register(command contract.Command) error {
	if command.Name == "" {
		return fmt.Errorf("a slash command has no name, so give it the word the user types after the slash")
	}
	if strings.ContainsAny(command.Name, "/ \t") {
		return fmt.Errorf("the command name %q holds a slash or a space, so write it as the bare word, such as \"status\"", command.Name)
	}
	if command.Help == "" {
		return fmt.Errorf("the command %q has no help line, so write the one line the help listing prints", command.Name)
	}
	if command.Run == nil {
		return fmt.Errorf("the command %q has nothing to run, so give it a function", command.Name)
	}
	if _, taken := registry.byName[command.Name]; taken {
		return fmt.Errorf("the command %q is registered twice, so one of the two packages must rename it", command.Name)
	}

	registry.order = append(registry.order, command.Name)
	registry.byName[command.Name] = command
	return nil
}

// Lookup finds one command by its name, without the slash.
func (registry *commandRegistry) Lookup(name string) (contract.Command, bool) {
	command, found := registry.byName[strings.TrimPrefix(name, "/")]
	return command, found
}

// All returns every command, in the order they were registered, which is the
// order the help listing prints them in.
func (registry *commandRegistry) All() []contract.Command {
	all := make([]contract.Command, 0, len(registry.order))
	for _, name := range registry.order {
		all = append(all, registry.byName[name])
	}
	return all
}
