package contract

import (
	"context"
	"errors"
)

// CommandContext is the little the harness tells a slash command about where it
// was typed.
type CommandContext struct {
	// Channel is where to answer, and where to show a preview if the command
	// needs one.
	Channel Channel
	// TaskID is the task the user is looking at, or empty when there is none.
	TaskID string
}

// Command is one slash command, such as "/status". It is a value rather than an
// interface, because a command is a name, a help line, and a function, and there
// is nothing to implement.
//
// Each package exports the commands it owns as values of this type, and the
// orchestrator registers them in serve.go, so that no two workers ever edit the
// same registration file.
type Command struct {
	// Name is the command without its leading slash, such as "status".
	Name string
	// Help is the one line the "/help" listing prints.
	Help string
	// TerminalOnly refuses the command on any channel but the terminal, which is
	// how the vault stays out of Signal.
	TerminalOnly bool
	// Run does the work and returns the reply text.
	Run func(ctx context.Context, arguments string, where CommandContext) (string, error)
}

// ErrNoSuchCommand means the line named a slash command nobody registered, so
// the router can tell a mistyped command from one that ran and failed.
var ErrNoSuchCommand = errors.New("there is no such command, so type /help to see the ones there are")
