package contract

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// LocalModelAlias is the name of the model alias Coeus ships with, which points
// at the llama-server daemon on the machine the agent runs on.
const LocalModelAlias = "local"

// ModelAlias is one model the user can name, with everything needed to reach it.
// Coeus ships with one, the local model; "coeus init" writes the cloud aliases
// after asking which provider the user wants.
type ModelAlias struct {
	// Name is what the user and the record call it, such as "local".
	Name string `toml:"name"`
	// Provider is how the model is reached: one of the two wire protocols, or
	// the vendor's own command-line program.
	Provider ProviderKind `toml:"provider"`
	// BaseAddress is the address of the server, for an OpenAI-compatible
	// provider. It is empty for the Anthropic provider, which has one address,
	// and for a command-line provider, which has none.
	BaseAddress string `toml:"base_address"`
	// Program is the command-line program to run, for a "cli" provider: either
	// ClaudeProgram or CodexProgram. It is empty for the other two.
	Program string `toml:"program"`
	// ModelName is what the server or the program calls the model, such as
	// "local-coder".
	ModelName string `toml:"model_name"`
	// ContextLength is how many tokens the model can hold, which is the one
	// number the working-context rule is sized from.
	ContextLength int `toml:"context_length"`
	// KeyReference is a "secret://name" reference to the API key, and is empty
	// for a local server that needs none.
	KeyReference string `toml:"key_reference"`
}

// Caps are the limits from the design that keep the agent from running away.
// Every one of them is a hard stop, not a target.
type Caps struct {
	// RoundsPerTask is the tool-round budget of a task. Default 100.
	RoundsPerTask int `toml:"rounds_per_task"`
	// TimePerTask is the wall-clock budget of a task. Default one hour.
	TimePerTask time.Duration `toml:"time_per_task"`
	// TimePerTool is how long one tool may run. Default seven minutes.
	TimePerTool time.Duration `toml:"time_per_tool"`
	// TimePerTurn is how long one turn may take. Default fifteen minutes.
	TimePerTurn time.Duration `toml:"time_per_turn"`
	// QueuedMessages is how many messages may wait in the queue. Default 100.
	QueuedMessages int `toml:"queued_messages"`
	// ToolOutputBytes is the cap on one tool result before the rest spills to a
	// file the result names. Default 30000.
	ToolOutputBytes int `toml:"tool_output_bytes"`
	// IdenticalCallWindow is how many recent tool calls the guard compares a new
	// call against. Default 20.
	IdenticalCallWindow int `toml:"identical_call_window"`
}

// MemoryCaps are the hard size limits on the two persona memory files, which is
// the rule borrowed from Hermes: a memory file with no limit grows until it
// crowds out the task.
type MemoryCaps struct {
	// WorldFactsBytes caps MEMORY.md. Default 8000.
	WorldFactsBytes int `toml:"world_facts_bytes"`
	// UserFactsBytes caps USER.md. Default 4000.
	UserFactsBytes int `toml:"user_facts_bytes"`
}

// Config is the whole of ~/.coeus/config.toml, read once at startup. Every field
// says its default in its doc comment, and DefaultConfig fills them in.
type Config struct {
	// Models are the model aliases the user may name.
	Models []ModelAlias `toml:"models"`
	// DefaultModel is the alias used when nothing else says otherwise. Default
	// "local".
	DefaultModel string `toml:"default_model"`
	// FallbackChain is the aliases to try, in order, when the default one fails.
	// Default empty, because a fresh install has one model.
	FallbackChain []string `toml:"fallback_chain"`
	// SignalAccount is the phone number the agent is linked to. Default empty,
	// which means Signal is off until "coeus signal link" runs.
	SignalAccount string `toml:"signal_account"`
	// BrowserProfilePath is the Chrome profile the agent drives. Default is the
	// "default" profile inside the home folder's browser folder.
	BrowserProfilePath string `toml:"browser_profile_path"`
	// SandboxRoots are the folders a sandboxed command may reach. Default is the
	// user's home directory, with the paths in ExcludedFromSandbox masked out.
	SandboxRoots []string `toml:"sandbox_roots"`
	// BackupPath is where the nightly encrypted archive is written. Default is
	// the backups folder inside the home folder.
	BackupPath string `toml:"backup_path"`
	// SearchServerAddress is the SearXNG instance the web tool searches through.
	// Default empty, which makes the web tool read the DuckDuckGo results page
	// instead, so that search works with no key and no server.
	SearchServerAddress string `toml:"search_server_address"`
	// HandoffTimeout is how long a browser or desktop handoff waits for the user
	// before giving up. Default thirty minutes.
	HandoffTimeout time.Duration `toml:"handoff_timeout"`
	// AskMeFirst is the list of shipped entries the user keeps on the
	// ask-me-first list, by name. Default is all three; the user can remove any
	// or empty the list.
	AskMeFirst []string `toml:"ask_me_first"`
	// PermissionRules are the user's own rules, applied after the shipped
	// entries, where the last match wins. Default empty.
	PermissionRules []PermissionRule `toml:"permission_rules"`
	// Caps are the limits from the design.
	Caps Caps `toml:"caps"`
	// MemoryCaps are the size limits on the two persona memory files.
	MemoryCaps MemoryCaps `toml:"memory_caps"`
}

// DefaultConfig returns the configuration a fresh install starts from, before
// anything in config.toml is read. Every value here is the default named in the
// doc comment of its field.
func DefaultConfig() Config {
	return Config{
		Models: []ModelAlias{{
			Name:          LocalModelAlias,
			Provider:      ProviderOpenAI,
			BaseAddress:   "http://127.0.0.1:19091/v1",
			ModelName:     "local-coder",
			ContextLength: 262144,
		}},
		DefaultModel:   LocalModelAlias,
		HandoffTimeout: 30 * time.Minute,
		AskMeFirst:     DefaultAskMeFirst(),
		Caps: Caps{
			RoundsPerTask:       100,
			TimePerTask:         time.Hour,
			TimePerTool:         7 * time.Minute,
			TimePerTurn:         15 * time.Minute,
			QueuedMessages:      100,
			ToolOutputBytes:     30000,
			IdenticalCallWindow: 20,
		},
		MemoryCaps: MemoryCaps{
			WorldFactsBytes: 8000,
			UserFactsBytes:  4000,
		},
	}
}

// ExcludedFromSandbox returns the paths that must never be reachable from inside
// the sandbox, given the user's home directory. They are the agent's own home
// folder, the vault, the browser profiles, and the user's SSH keys.
func ExcludedFromSandbox(userHome string) []string {
	home := NewHome(filepath.Join(userHome, HomeFolderName))
	return []string{
		home.Root,
		home.VaultFile(),
		home.BrowserFolder(),
		filepath.Join(userHome, ".ssh"),
	}
}

// DefaultSandboxRoots returns the folders a sandboxed command may reach on a
// fresh install: the user's home directory, with the excluded paths masked out
// from inside it.
func DefaultSandboxRoots(userHome string) []string {
	return []string{userHome}
}

// CheckSandboxRoot returns an error when a configured sandbox root is one of the
// excluded paths or sits inside one, because a root like that would put the
// vault or the SSH keys inside the fence.
func CheckSandboxRoot(root string, userHome string) error {
	if root == "" {
		return fmt.Errorf("a sandbox root is empty, so give it a full path such as %q", userHome)
	}
	if !filepath.IsAbs(root) {
		return fmt.Errorf("the sandbox root %q is not a full path, so write it starting from the root of the filesystem", root)
	}
	clean := filepath.Clean(root)
	for _, excluded := range ExcludedFromSandbox(userHome) {
		if clean == excluded || strings.HasPrefix(clean, excluded+string(filepath.Separator)) {
			return fmt.Errorf("the sandbox root %q is inside %q, which must stay outside the sandbox, so choose a root that does not contain it", root, excluded)
		}
	}
	return nil
}
