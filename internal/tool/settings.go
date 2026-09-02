package tool

import (
	"time"

	"github.com/JaredTate/coeus/internal/contract"
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
	// TaskID is the task or job the tools are running for, which names the
	// events they write and the files they spill.
	TaskID string
	// Note is where the registry says what it skipped and why. When it is nil
	// the line goes to the standard library's logger.
	Note func(line string)
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
