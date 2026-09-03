package tool

import (
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/tool/browserhandoff"
	"github.com/JaredTate/coeus/internal/tool/browserlogin"
	"github.com/JaredTate/coeus/internal/tool/read"
	"github.com/JaredTate/coeus/internal/tool/task"
)

// Settings is everything the registry and the built-in tools need from the rest
// of the program. It is filled in one place, where the program is wired
// together, so that no tool has to reach for anything on its own. A field left
// empty turns the tools that need it into tools that refuse to run with a line
// saying what is missing, rather than tools the model cannot see: the design
// shows every model all eighteen names on every call.
type Settings struct {
	// Configuration is the whole of config.toml, read once at startup. The caps,
	// the sandbox roots, and the search server address come from it.
	Configuration contract.Config
	// Home is the agent's home folder, which is where the tools folder and the
	// spill folder live.
	Home contract.Home
	// UserHome is the user's own home directory, which is what the paths that
	// must stay outside the sandbox are worked out from.
	UserHome string
	// TaskID is the task or job the tools are running for, which names the
	// events they write and the files they spill.
	TaskID string
	// Note is where the registry says what it skipped and why. When it is nil
	// the line goes to the standard library's logger.
	Note func(line string)

	// Log is the event log the write and edit tools record a file's prior
	// contents in.
	Log contract.Store
	// Sandbox is the fence every shell command runs inside.
	Sandbox contract.Sandbox
	// Permission rules on a command that asks for administrator powers.
	Permission contract.Permission
	// Memory is what the agent knows across tasks.
	Memory contract.Memory
	// Skills is the store of saved procedures.
	Skills contract.Skill
	// Jobs is the store of jobs and their task lists.
	Jobs contract.Job
	// Browser is the worker driving the agent's own Chrome.
	Browser contract.BrowserWorker
	// Desktop is the worker driving the screen, the mouse, and the keyboard.
	Desktop contract.Desktop
	// Clock is where every wait in a tool is counted.
	Clock contract.Clock

	// Records is the record of the task running now, which the task tool writes
	// through and the read tool reads a past result from.
	Records task.Records
	// Results is where the whole text of a past result such as r7 is kept.
	Results read.Stored
	// Reports is where the whole text of a job's report such as j4.2 is kept.
	Reports read.Stored
	// Credentials is where the login tool gets what to type, and wave five wires
	// the vault behind it.
	Credentials browserlogin.Credentials
	// TwoFactorCode is where the login tool gets a second code.
	TwoFactorCode browserlogin.TwoFactorCode
	// AskUser is how the handoff tool reaches the user.
	AskUser browserhandoff.AskUser

	// CoeusProgram is the whole path of the coeus binary, whose askpass
	// subcommand is what sudo reads the administrator password from. Empty means
	// this program.
	CoeusProgram string
	// WorkingDirectory is where a shell command runs, and is inside a sandbox
	// root. Empty means the first root.
	WorkingDirectory string
	// Ripgrep is the path of the ripgrep program. Empty means look for it on the
	// PATH, and search on its own when it is not there.
	Ripgrep string
	// AllowedHosts are the hosts the web tool may reach whatever their number
	// is, which is how a search server of the user's own on this machine stays
	// reachable.
	AllowedHosts []string
}

// toolTimeout is how long one tool may run, from the configuration, with the
// shipped default when the configuration says nothing.
func (settings Settings) toolTimeout() time.Duration {
	if settings.Configuration.Caps.TimePerTool > 0 {
		return settings.Configuration.Caps.TimePerTool
	}
	return contract.DefaultConfig().Caps.TimePerTool
}

// outputCap is how many bytes of one result the model sees before the rest
// spills to a file, from the configuration, with the shipped default when the
// configuration says nothing.
func (settings Settings) outputCap() int {
	if settings.Configuration.Caps.ToolOutputBytes > 0 {
		return settings.Configuration.Caps.ToolOutputBytes
	}
	return contract.DefaultConfig().Caps.ToolOutputBytes
}

// workingDirectory is where a shell command runs: the folder the settings name,
// or the first folder the agent may work in.
func (settings Settings) workingDirectory() string {
	if settings.WorkingDirectory != "" {
		return settings.WorkingDirectory
	}
	if len(settings.Configuration.SandboxRoots) > 0 {
		return settings.Configuration.SandboxRoots[0]
	}
	return settings.Home.Root
}
