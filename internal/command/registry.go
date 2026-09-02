// The one table of commands shared by every screen follows OpenCode's command
// registry at ~/Code/opencode/packages/tui/src/app.tsx, where the field to look
// for is slashName: one list of commands, each with a name, a line of help, and
// a function, and the same list serves the palette and the typed slash command.
// OpenCode builds its list inside the terminal client, so a command can only
// exist where there is a screen; here the list lives in the program, so a
// command means the same thing in the terminal and over Signal.

package command

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// ErrNoSuchCommand means the line named a command the registry does not hold,
// or held no command at all. The caller decides what to do with a line like
// that: the router sends it to the model, and a screen prints it.
var ErrNoSuchCommand = errors.New("there is no command by that name")

// terminalOnlyRefusal is the one line a terminal-only command answers with
// anywhere else. It is a reply rather than an error, because the user did
// nothing wrong: they asked in the wrong place.
const terminalOnlyRefusal = "the /%s command works only in the terminal, so open a terminal on the machine Coeus runs on and try there"

// Registry holds every slash command the program answers, in the order they
// were registered, which is the order the help listing prints them in.
//
// It is filled once at startup, in cmd/coeus/serve.go, and only read after
// that, so it needs no lock of its own.
type Registry struct {
	order  []string
	byName map[string]contract.Command
}

// NewRegistry returns a registry holding no commands.
func NewRegistry() *Registry {
	return &Registry{byName: map[string]contract.Command{}}
}

// Register puts one command in the registry, refusing anything the program
// could not answer with and refusing a name that is already taken, because a
// second command with the same name would silently hide the first.
func (registry *Registry) Register(command contract.Command) error {
	switch {
	case command.Name == "":
		return errors.New("a slash command has no name, so give it the word the user types after the slash")
	case strings.ContainsAny(command.Name, "/ \t\r\n"):
		return fmt.Errorf("the command name %q holds a slash or a space, so write it as the bare word, such as \"status\"", command.Name)
	case command.Help == "":
		return fmt.Errorf("the command %q has no help line, so write the one line the help listing prints", command.Name)
	case command.Run == nil:
		return fmt.Errorf("the command %q has nothing to run, so give it a function", command.Name)
	}
	if _, taken := registry.byName[command.Name]; taken {
		return fmt.Errorf("the command %q is registered twice, so one of the two packages must rename it", command.Name)
	}

	registry.order = append(registry.order, command.Name)
	registry.byName[command.Name] = command
	return nil
}

// Lookup finds one command by name, with or without its leading slash.
func (registry *Registry) Lookup(name string) (contract.Command, bool) {
	command, found := registry.byName[strings.TrimPrefix(strings.TrimSpace(name), "/")]
	return command, found
}

// All returns every command in the order they were registered.
func (registry *Registry) All() []contract.Command {
	all := make([]contract.Command, 0, len(registry.order))
	for _, name := range registry.order {
		all = append(all, registry.byName[name])
	}
	return all
}

// Help is the text the "/help" command answers with: one line per command, in
// registration order, with the names lined up so the listing reads as a table.
func (registry *Registry) Help() string {
	widest := 0
	for _, name := range registry.order {
		if len(name)+1 > widest {
			widest = len(name) + 1
		}
	}

	written := &strings.Builder{}
	written.WriteString("these are the commands Coeus answers:\n\n")
	for _, command := range registry.All() {
		fmt.Fprintf(written, "  %-*s  %s\n", widest, "/"+command.Name, command.Help)
	}
	return written.String()
}

// Run reads the command out of a line the user typed and runs it. A
// terminal-only command asked for anywhere but the terminal comes back as one
// plain line rather than as an error, and a line naming no command Coeus holds
// comes back as ErrNoSuchCommand, so that the caller can send it to the model
// instead.
func (registry *Registry) Run(ctx context.Context, line string, where contract.CommandContext) (string, error) {
	name, arguments := SplitLine(line)
	if name == "" {
		return "", fmt.Errorf("that line holds no command, so type /help for the list: %w", ErrNoSuchCommand)
	}

	command, found := registry.Lookup(name)
	if !found {
		return "", fmt.Errorf("there is no command called /%s, so type /help for the list: %w", name, ErrNoSuchCommand)
	}
	if command.TerminalOnly && !inTheTerminal(where) {
		return fmt.Sprintf(terminalOnlyRefusal, command.Name), nil
	}
	return command.Run(ctx, arguments, where)
}

// inTheTerminal says whether the command was typed on the one channel that can
// hide what is typed. A command context with no channel at all is not the
// terminal, because nothing has said that it is.
func inTheTerminal(where contract.CommandContext) bool {
	return where.Channel != nil && where.Channel.Name() == contract.TerminalChannelName
}

// SplitLine reads a typed line apart into the command's name, without its
// leading slash, and everything after it. A line with no command on it gives
// back two empty strings, and nothing here can panic, whatever the line holds.
func SplitLine(line string) (string, string) {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "/"))
	if trimmed == "" {
		return "", ""
	}

	cut := strings.IndexFunc(trimmed, func(letter rune) bool {
		return letter == ' ' || letter == '\t' || letter == '\n' || letter == '\r'
	})
	if cut < 0 {
		return trimmed, ""
	}
	return trimmed[:cut], strings.TrimSpace(trimmed[cut:])
}
