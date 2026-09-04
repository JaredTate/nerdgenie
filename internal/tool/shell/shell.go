// Handing back an id for a command that has not finished in ten seconds, rather
// than making the model wait, is OpenClaw's, at
// ~/Code/openclaw/src/agents/bash-tools.schemas.ts; the field that asks for
// administrator powers alongside the command is Codex's, at
// docs/reference/codex/shell_spec.rs. The Go here is written fresh.

package shell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// YieldAfter is how long the tool waits for a command before handing back an id
// instead. Ten seconds is long enough for nearly every command a model runs and
// short enough that a model never sits waiting on a build.
const YieldAfter = 10 * time.Second

// The bounds on this tool.
const (
	// MaxCommandBytes is the longest command the tool will run.
	MaxCommandBytes = 16 << 10
	// MaxRunning is how many commands may be running at once.
	MaxRunning = 8
	// MaxStreamBytes is how much of what a command wrote on each of its two
	// streams the model is shown.
	MaxStreamBytes = 20 << 10
	// MaxReasonRunes is the longest written reason for asking for administrator
	// powers.
	MaxReasonRunes = 500
)

// The four things one call can ask for.
const (
	// ActionRun runs a command.
	ActionRun = "run"
	// ActionPoll asks whether a running command has finished.
	ActionPoll = "poll"
	// ActionTail asks what a command has written.
	ActionTail = "tail"
	// ActionKill stops a running command.
	ActionKill = "kill"
)

// KnownAction says whether the action is one of the four.
func KnownAction(action string) bool {
	switch action {
	case ActionRun, ActionPoll, ActionTail, ActionKill:
		return true
	default:
		return false
	}
}

// Settings is what the shell tool needs to do its work.
type Settings struct {
	// Sandbox is the fence every ordinary command runs inside.
	Sandbox contract.Sandbox
	// Permission rules on a command that asks for administrator powers.
	Permission contract.Permission
	// Clock is where the ten seconds are counted.
	Clock contract.Clock
	// Home is the agent's home folder, where the askpass helper is written.
	Home contract.Home
	// CoeusProgram is the whole path of the nerdgenie binary, whose askpass
	// subcommand is what sudo reads the password from.
	CoeusProgram string
	// WorkingDirectory is where a command runs, and is inside a sandbox root.
	WorkingDirectory string
	// Timeout is how long one command may run before it is stopped.
	Timeout time.Duration
}

// Call is one call to this tool, read out of the JSON the model wrote.
type Call struct {
	// Command is the shell command to run.
	Command string `json:"command"`
	// Action is run, poll, tail, or kill. An empty action is a run.
	Action string `json:"action"`
	// ID names a command that is already running, for poll, tail, and kill.
	ID string `json:"id"`
	// Escalate asks for the command to run with administrator powers.
	Escalate bool `json:"escalate"`
	// Reason is the written reason for asking for them.
	Reason string `json:"reason"`
}

// Tool is the shell tool.
type Tool struct {
	settings Settings
	running  *table
}

// New returns the shell tool.
func New(settings Settings) *Tool {
	return &Tool{settings: settings, running: newTable()}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolShell,
		Description: "Runs a command in the sandbox. After ten seconds it hands back an id to poll, tail, or kill. " +
			"Set escalate with a written reason to ask for administrator powers. Use read and search for files.",
		Fields: []contract.ToolField{
			{Name: "command", Type: "string", Description: "The command to run, as you would type it in a terminal."},
			{Name: "action", Type: "string", Description: "One of run, poll, tail, or kill. Leave it out to run."},
			{Name: "id", Type: "string", Description: "The id of a running command, for poll, tail, and kill."},
			{Name: "escalate", Type: "boolean", Description: "True to ask for administrator powers for this command."},
			{Name: "reason", Type: "string", Description: "Why the command needs administrator powers, in one line."},
		},
		Classes: []contract.PermissionClass{contract.ClassExecute},
	}
}

// Run does what the call asks for: start a command, or ask after one that is
// already running.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked, err := ReadCall(ctx, written)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	switch asked.Action {
	case ActionPoll:
		return tool.running.poll(asked.ID, tool.now())
	case ActionTail:
		return tool.running.tail(asked.ID)
	case ActionKill:
		return tool.running.kill(asked.ID)
	default:
		return tool.start(ctx, asked)
	}
}

// ReadCall reads the model's arguments and refuses anything this tool could not
// act on. It is exported so that a fuzz test can throw any text at it.
func ReadCall(_ context.Context, written []byte) (Call, error) {
	asked := Call{}
	if len(written) > 0 {
		if err := json.Unmarshal(written, &asked); err != nil {
			return Call{}, fmt.Errorf("cannot read this call's arguments as JSON, so write an object with a command in it: %w", err)
		}
	}
	if asked.Action == "" {
		asked.Action = ActionRun
	}
	if !KnownAction(asked.Action) {
		return Call{}, fmt.Errorf("the action %q is not one this tool knows, so use run, poll, tail, or kill", asked.Action)
	}
	if asked.Action != ActionRun {
		if strings.TrimSpace(asked.ID) == "" {
			return Call{}, fmt.Errorf("a %s names the command to act on, so give the id the tool handed back", asked.Action)
		}
		return asked, nil
	}
	return asked, checkRun(asked)
}

// checkRun holds the rules a run must satisfy before anything is started.
func checkRun(asked Call) error {
	if strings.TrimSpace(asked.Command) == "" {
		return errors.New("this call has no command in it, so write the command as you would type it in a terminal")
	}
	if len(asked.Command) > MaxCommandBytes {
		return fmt.Errorf("the command is %d bytes and the cap is %d, so run it from a script file instead",
			len(asked.Command), MaxCommandBytes)
	}
	if asked.Escalate && strings.TrimSpace(asked.Reason) == "" {
		return errors.New("this command asks for administrator powers and gives no reason, so write in one line why it needs them")
	}
	if len([]rune(asked.Reason)) > MaxReasonRunes {
		return fmt.Errorf("the reason is %d characters and the cap is %d, so say it in one line",
			len([]rune(asked.Reason)), MaxReasonRunes)
	}
	return nil
}
