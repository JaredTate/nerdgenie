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
	"slices"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// YieldAfter is how long the tool waits for a command before handing back an id
// instead. Ten seconds is long enough for nearly every command a model runs and
// short enough that a model never sits waiting on a build.
const YieldAfter = 10 * time.Second

// PollWaitsFor is how long a poll waits for a running command to finish before
// saying it is still running. A poll that answered at once was a round spent
// for nothing: the fresh game build's play-test task polled its own script
// every five seconds, fourteen rounds for one run, each round a full prompt.
const PollWaitsFor = 20 * time.Second

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

// The six things one call can ask for.
const (
	// ActionRun runs a command.
	ActionRun = "run"
	// ActionServe starts a command that is meant to keep running, such as a
	// server, and answers the moment it listens on a port.
	ActionServe = "serve"
	// ActionCheck asks whether a port on this machine answers.
	ActionCheck = "check"
	// ActionPoll asks whether a running command has finished.
	ActionPoll = "poll"
	// ActionTail asks what a command has written.
	ActionTail = "tail"
	// ActionKill stops a running command.
	ActionKill = "kill"
)

// KnownAction says whether the action is one of the six.
func KnownAction(action string) bool {
	switch action {
	case ActionRun, ActionServe, ActionCheck, ActionPoll, ActionTail, ActionKill:
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
	// NerdGenieProgram is the whole path of the nerdgenie binary, whose askpass
	// subcommand is what sudo reads the password from.
	NerdGenieProgram string
	// WorkingDirectory is where a command runs, and is inside a sandbox root.
	WorkingDirectory string
	// Timeout is how long one command may run before it is stopped.
	Timeout time.Duration
	// SocketTables are the kernel's socket tables a serve watches for a new
	// listening port; nil means /proc/net/tcp and /proc/net/tcp6.
	SocketTables []string
}

// Call is one call to this tool, read out of the JSON the model wrote.
type Call struct {
	// Command is the shell command to run.
	Command string `json:"command"`
	// Action is run, serve, check, poll, tail, or kill. An empty action is a run.
	Action string `json:"action"`
	// ID names a command that is already running, for poll, tail, and kill.
	ID string `json:"id"`
	// Escalate asks for the command to run with administrator powers.
	Escalate bool `json:"escalate"`
	// Reason is the written reason for asking for them.
	Reason string `json:"reason"`
	// Port is the port a check asks after, or the one a serve waits for.
	Port int `json:"port"`
	// Path is what a check fetches over HTTP from the port, or nothing.
	Path string `json:"path"`
}

// Tool is the shell tool.
type Tool struct {
	settings Settings
	running  *table
	past     pastRuns
}

// New returns the shell tool.
func New(settings Settings) *Tool {
	return &Tool{settings: settings, running: newTable()}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolShell,
		Description: "Runs a command in the sandbox; after ten seconds it hands back an id to poll, tail, or kill. " +
			"Serve starts a server and names its port; check asks whether a port answers. Escalate with a reason for administrator powers.",
		Fields: []contract.ToolField{
			{Name: "command", Type: "string", Description: "The command to run, as you would type it in a terminal. Kill a process by its exact id, never by a name pattern."},
			{Name: "action", Type: "string", Description: "One of run, serve, check, poll, tail, or kill. Leave it out to run."},
			{Name: "id", Type: "string", Description: "The id of a running command, for poll, tail, and kill."},
			{Name: "escalate", Type: "boolean", Description: "True to ask for administrator powers for this command."},
			{Name: "reason", Type: "string", Description: "Why the command needs administrator powers, in one line."},
			{Name: "expect", Type: "string", Description: "What the result should show, checked by the harness: tests 62 to 63, all passing, 2 failing, exit 0, or contains X."},
			{Name: "port", Type: "number", Description: "The port a check asks after, or the one a serve waits for."},
			{Name: "path", Type: "string", Description: "For a check, the path to fetch over HTTP, such as /index.html."},
		},
		Classes: []contract.PermissionClass{contract.ClassExecute},
	}
}

// Run does what the call asks for: start a command, or ask after one that is
// already running, or after a port.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked, err := ReadCall(ctx, written)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	switch asked.Action {
	case ActionPoll:
		return tool.pollWaiting(ctx, asked.ID)
	case ActionTail:
		return tool.running.tail(asked.ID)
	case ActionKill:
		return tool.running.kill(asked.ID)
	case ActionServe:
		return tool.serve(ctx, asked)
	case ActionCheck:
		return tool.check(ctx, asked)
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
		return Call{}, fmt.Errorf("the action %q is not one this tool knows, so use run, serve, check, poll, tail, or kill", asked.Action)
	}
	switch asked.Action {
	case ActionRun:
		return asked, checkRun(asked)
	case ActionServe:
		return asked, checkServe(asked)
	case ActionCheck:
		return asked, checkPort(asked.Port)
	default:
		if strings.TrimSpace(asked.ID) == "" {
			return Call{}, fmt.Errorf("a %s names the command to act on, so give the id the tool handed back", asked.Action)
		}
		return asked, nil
	}
}

// theKillsByNamePattern are the programs that kill or list processes by a
// pattern matched against every command line on the machine, which is a
// pattern the shell running them matches too. Each is looked for as a word at
// the start of a command or after a shell separator, so that a mention in
// passing is not one.
var theKillsByNamePattern = []string{"pkill", "killall", "pgrep"}

// theWordsBeforeACommand are the shell words a command may stand after and
// still be the command: the wrappers, the keywords, and the negation. The
// fifth game build's play-test task polled its own script with "if ! pgrep
// -f", which the rule read as a mention in passing.
var theWordsBeforeACommand = []string{"sudo", "exec", "then", "do", "if", "!", "while", "until", "elif", "time", "nohup"}

// killsByNamePattern says whether a command kills or lists processes by a
// name pattern. On the fifth game build the model ran pkill -f on its own
// server and got exit code 143, which is its own shell killed: the shell's
// command line held the pattern too. The rule is this machine's oldest, from
// the day an agent closed every terminal on the desktop the same way.
func killsByNamePattern(command string) bool {
	for _, piece := range strings.FieldsFunc(command, func(letter rune) bool {
		return letter == ';' || letter == '&' || letter == '|' || letter == '\n' || letter == '(' || letter == '`'
	}) {
		words := strings.Fields(piece)
		for at, word := range words {
			if slices.Contains(theKillsByNamePattern, word) && (at == 0 || slices.Contains(theWordsBeforeACommand, words[at-1])) {
				return true
			}
		}
	}
	return false
}

// checkRun holds the rules a run must satisfy before anything is started.
func checkRun(asked Call) error {
	if strings.TrimSpace(asked.Command) == "" {
		return errors.New("this call has no command in it, so write the command as you would type it in a terminal")
	}
	if killsByNamePattern(asked.Command) {
		return errors.New("this command kills or lists processes by a name pattern, and the shell running it matches the pattern through its own command line and dies with it (exit code 143). " +
			"Find the exact process id first, with ss -ltnp for a port or from the p-number this tool handed back, and kill that id alone")
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
