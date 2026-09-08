package contract

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// LocalModelAlias is the name of the model alias Nerd Genie ships with, which points
// at the llama-server daemon on the machine the agent runs on.
const LocalModelAlias = "local"

// ModelAlias is one model the user can name, with everything needed to reach it.
// Nerd Genie ships with one, the local model; "nerdgenie init" writes the cloud aliases
// after asking which provider the user wants.
type ModelAlias struct {
	// Name is what the user and the record call it, such as "local".
	Name string `toml:"name"`
	// Provider is how the model is reached: one of the two wire protocols, the
	// vendor's own command-line program, or OpenAI's Codex backend through that
	// program's login.
	Provider ProviderKind `toml:"provider"`
	// BaseAddress is the address of the server, for an OpenAI-compatible
	// provider. It is empty for the Anthropic provider, which has one address,
	// for a command-line provider, which has none, and for the codex provider,
	// whose backend is fixed.
	BaseAddress string `toml:"base_address"`
	// Program is the command-line program to run, for a "cli" provider: either
	// ClaudeProgram or CodexProgram. It is empty for the other three.
	Program string `toml:"program"`
	// ModelName is what the server or the program calls the model, such as
	// "local-coder".
	ModelName string `toml:"model_name"`
	// ContextLength is how many tokens the model can hold, which is the one
	// number the working-context rule is sized from.
	ContextLength int `toml:"context_length"`
	// Vision says the model reads pictures, so a screenshot or a picture file
	// is sent to it as an image rather than described as unseen.
	Vision bool `toml:"vision"`
	// KeyReference is a "secret://name" reference to the API key, and is empty
	// for a local server that needs none.
	KeyReference string `toml:"key_reference"`
	// Think is how hard this model is asked to think before it answers: one of
	// the six levels ThinkLevels names. It is empty by default, which leaves
	// the provider's own default alone.
	Think Think `toml:"think"`
}

// Caps are the limits from the design that keep the agent from running away.
// Every one of them is a hard stop, not a target. The three budgets, the rounds
// and the time of a task and the time of a turn, are off unless the user sets
// them: a zero, which is the default, means no limit, because Nerd Genie puts no cap
// on its own work unless the user asks for one. The rest are always on.
type Caps struct {
	// RoundsPerTask is the tool-round budget of a task. Default 0, which is no
	// limit; a number above zero stops the task after that many rounds.
	RoundsPerTask int `toml:"rounds_per_task"`
	// TimePerTask is the wall-clock budget of a task. Default 0, which is no
	// limit; a length of time above zero stops the task when it is used up.
	TimePerTask time.Duration `toml:"time_per_task"`
	// TimePerTool is how long one tool may run before it is killed. Default
	// seven minutes. It is a safety limit and not a budget, so it is never off.
	TimePerTool time.Duration `toml:"time_per_tool"`
	// TimePerTurn is how long one turn may take. Default 0, which is no limit;
	// a length of time above zero stops the turn when it is used up.
	TimePerTurn time.Duration `toml:"time_per_turn"`
	// QueuedMessages is how many messages may wait in the queue. Default 100.
	QueuedMessages int `toml:"queued_messages"`
	// ToolOutputBytes is the cap on one tool result before the rest spills to a
	// file the result names. Default 30000.
	ToolOutputBytes int `toml:"tool_output_bytes"`
	// IdenticalCallWindow is how many recent tool calls the guard compares a new
	// call against. Default 20.
	IdenticalCallWindow int `toml:"identical_call_window"`
	// OutputTokensPerCall caps what the model may write on one call. Default
	// 8192, which every provider sends, because a zero cap is refused on the
	// wire.
	OutputTokensPerCall int `toml:"output_tokens_per_call"`
	// BufferedBrowserEvents is how many of the person's own browser events are
	// held for a reader that has fallen behind before the oldest are dropped.
	// Default 256.
	BufferedBrowserEvents int `toml:"buffered_browser_events"`
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

// Config is the whole of ~/.nerdgenie/config.toml, read once at startup. Every field
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
	// which means Signal is off until "nerdgenie signal link" runs.
	SignalAccount string `toml:"signal_account"`
	// BrowserProfilePath is the Chrome profile the agent drives. Default is the
	// "default" profile inside the home folder's browser folder.
	BrowserProfilePath string `toml:"browser_profile_path"`
	// Sandbox says whether the shell and the file tools run straight on the
	// machine as the user, the way Hermes and OpenClaw run on the host, or
	// inside the sandbox (bwrap, Landlock, and the roots below). It is "off"
	// or "fence"; empty means off, which is the default: the only gate is
	// then the permission function and the ask-me-first list. The vault, the
	// browser profile, the backups, and the user's SSH keys stay out of reach
	// of the file tools either way.
	Sandbox string `toml:"sandbox"`
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
	// Yolo says whether the running agent starts with yolo on, so every call
	// that would ask first runs without asking until "/yolo off". Nil means on,
	// the default for an agent that runs unattended; set it false to be asked.
	// It is the serve's starting state only; "/yolo" changes it for the session.
	Yolo *bool `toml:"yolo"`
	// PermissionRules are the user's own rules, applied after the shipped
	// entries, where the last match wins. Default empty.
	PermissionRules []PermissionRule `toml:"permission_rules"`
	// Caps are the limits from the design.
	Caps Caps `toml:"caps"`
	// MemoryCaps are the size limits on the two persona memory files.
	MemoryCaps MemoryCaps `toml:"memory_caps"`
}

// YoloAtStart says whether the serve starts with yolo on. It defaults on when
// the config does not say, because the agent is built to run unattended; a
// user who wants to be asked sets "yolo = false".
func (config Config) YoloAtStart() bool {
	return config.Yolo == nil || *config.Yolo
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
		// The three budgets are left at zero, which is off: no rounds per
		// task, no time per task, and no time per turn until the user sets one.
		Caps: Caps{
			TimePerTool:           7 * time.Minute,
			QueuedMessages:        100,
			ToolOutputBytes:       30000,
			IdenticalCallWindow:   20,
			OutputTokensPerCall:   8192,
			BufferedBrowserEvents: 256,
		},
		MemoryCaps: MemoryCaps{
			WorldFactsBytes: 8000,
			UserFactsBytes:  4000,
		},
	}
}

// ExcludedFromSandbox returns the paths that must never be reachable from inside
// the sandbox, given the user's home directory and the agent's own home folder,
// which NERDGENIE_HOME may have moved anywhere. They are the agent's home, the
// vault, the browser profiles, and the user's SSH keys, followed by any path
// the caller names, such as a configured browser profile or the backup folder,
// with empty names skipped.
func ExcludedFromSandbox(userHome string, agentHome string, alsoOutside ...string) []string {
	if agentHome == "" {
		agentHome = filepath.Join(userHome, HomeFolderName)
	}
	home := NewHome(agentHome)
	excluded := []string{
		home.Root,
		home.VaultFile(),
		home.BrowserFolder(),
		filepath.Join(userHome, ".ssh"),
	}
	for _, named := range alsoOutside {
		if named != "" {
			excluded = append(excluded, filepath.Clean(named))
		}
	}
	return excluded
}

// resolvedPath follows symbolic links when the path exists, so that a link to
// an excluded path is judged by where it leads, and leaves a path that does not
// exist yet as it is.
func resolvedPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return resolved
}

// WorkFolderName is the folder under the user's home directory that a fresh
// install lets the agent work in.
const WorkFolderName = "nerdgenie"

// DefaultSandboxRoots returns the folders a sandboxed command may reach on a
// fresh install: one work folder under the user's home directory, which "nerdgenie
// init" creates. The whole home directory is never a root, because it holds the
// user's daily browser profile, cloud credentials, and keys, and the fence can
// only grant, never subtract.
func DefaultSandboxRoots(userHome string) []string {
	return []string{filepath.Join(userHome, WorkFolderName)}
}

// CheckSandboxRoot returns an error when a configured sandbox root is one of the
// excluded paths, sits inside one, or holds one, because a root like that would
// put the vault, the browser profile, or the SSH keys inside the fence. Links
// are followed on both sides, so a root that is a link to the home directory
// is judged as the home directory.
func CheckSandboxRoot(root string, userHome string, agentHome string, alsoOutside ...string) error {
	if root == "" {
		return fmt.Errorf("a sandbox root is empty, so give it a full path such as %q", filepath.Join(userHome, WorkFolderName))
	}
	if !filepath.IsAbs(root) {
		return fmt.Errorf("the sandbox root %q is not a full path, so write it starting from the root of the filesystem", root)
	}
	clean := resolvedPath(root)
	for _, excluded := range ExcludedFromSandbox(userHome, agentHome, alsoOutside...) {
		excluded = resolvedPath(excluded)
		if clean == excluded || strings.HasPrefix(clean, excluded+string(filepath.Separator)) {
			return fmt.Errorf("the sandbox root %q is inside %q, which must stay outside the sandbox, so choose a root that does not contain it", root, excluded)
		}
		if strings.HasPrefix(excluded, clean+string(filepath.Separator)) || clean == string(filepath.Separator) {
			return fmt.Errorf("the sandbox root %q holds %q, which must stay outside the sandbox, so choose a narrower root such as %q", root, excluded, filepath.Join(userHome, WorkFolderName))
		}
	}
	return nil
}

// The two values the sandbox setting takes.
const (
	// SandboxFence runs commands inside bwrap and Landlock, reaching only the
	// sandbox roots.
	SandboxFence = "fence"
	// SandboxOff runs commands straight on the machine as the user.
	SandboxOff = "off"
)

// KnownSandboxMode says whether the value is one the sandbox setting takes;
// empty is known because it means the default, off.
func KnownSandboxMode(value string) bool {
	return value == "" || value == SandboxFence || value == SandboxOff
}

// SandboxMode is the sandbox setting with its default filled in.
func (settings Config) SandboxMode() string {
	if settings.Sandbox == "" {
		return SandboxOff
	}
	return settings.Sandbox
}
