package provider

import (
	"fmt"
	"net/http"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The bounds every provider in this package works inside. The design says to
// bound everything: every wait has a timeout, every buffer a cap, and every
// loop a limit, so that a provider that goes wrong stops rather than growing.
const (
	// stallTimeout is how long a stream may send nothing before the call is
	// given up on and tried again.
	stallTimeout = 60 * time.Second
	// maxEventLineBytes caps one line of a streamed answer.
	maxEventLineBytes = 1 << 20
	// maxToolCallJSONBytes caps the arguments of one tool call as they arrive in
	// pieces.
	maxToolCallJSONBytes = 1 << 20
	// maxToolCallsPerReply caps how many tools one reply may ask for.
	maxToolCallsPerReply = 256
	// maxReplyTextBytes caps the text of one reply.
	maxReplyTextBytes = 8 << 20
	// maxErrorBodyBytes caps how much of a refusal's body is read back.
	maxErrorBodyBytes = 8 << 10
)

// anthropicPublicAddress is where the Anthropic Messages API lives when the
// configuration names no address of its own.
const anthropicPublicAddress = "https://api.anthropic.com"

// anthropicVersion is the value of the version header the Messages API wants on
// every call.
const anthropicVersion = "2023-06-01"

// Options are what every provider in this package needs from the harness. They
// are the same for all four kinds, so that the fallback chain can build a list
// of models from one set of them.
type Options struct {
	// OnReset is called by the retry wrapper and the fallback chain before a
	// new attempt's first word when the attempt before it had streamed text,
	// so that whoever shows the reply as it arrives can withdraw the part that
	// belonged to an attempt that was given up on. It is nil when nobody shows
	// deltas as they arrive.
	OnReset func()
	// Clock is where every wait in this package is measured, so that a test can
	// control it. There is no default, because a provider that read the real
	// clock would make its tests wait.
	Clock contract.Clock
	// APIKey is the key the Anthropic and OpenAI-compatible providers send,
	// already resolved from the vault by the caller. It is empty for a local
	// server that needs none, for the Codex backend, which is signed in
	// through the codex program's own file, and for a command-line program
	// signed in to a subscription.
	APIKey string
	// HTTPClient is the client the two wire protocols call through. When it is
	// nil the package makes its own, with no whole-call timeout of its own,
	// because the call's own deadline and the stall watch bound it instead.
	HTTPClient *http.Client
	// Home is the agent's home folder, whose run folder holds the empty scratch
	// folder a command-line program is run in.
	Home contract.Home
	// Log takes one line for every retry, every fallback, and every note a
	// provider has to leave. A nil value throws the lines away.
	Log func(line string)
	// Unseen is told, as the model writes, how many characters no delta
	// shows: its thinking, and the arguments of its tool calls. The screen's
	// count of a call in progress is drawn from these and the deltas together,
	// so that a model writing a file for two minutes is not shown as writing
	// nothing. A nil value counts nothing.
	Unseen func(characters int)
}

// wroteUnseen tells the options how much the model wrote that no delta shows,
// and does nothing when nobody asked or nothing was written.
func (options Options) wroteUnseen(characters int) {
	if options.Unseen != nil && characters > 0 {
		options.Unseen(characters)
	}
}

// note writes one line to the log, and does nothing when no log was given.
func (options Options) note(format string, parts ...any) {
	if options.Log == nil {
		return
	}
	options.Log(fmt.Sprintf(format, parts...))
}

// client returns the HTTP client to call through.
func (options Options) client() *http.Client {
	if options.HTTPClient != nil {
		return options.HTTPClient
	}
	return &http.Client{}
}

// callTimeout is how long one whole call may take when the caller's context
// carries no deadline of its own. It is the turn cap from the configuration's
// defaults, which is off, so a call has no whole-call limit unless a turn cap
// is set; a stream that goes quiet is still ended by the stall watch.
func callTimeout() time.Duration {
	return contract.DefaultConfig().Caps.TimePerTurn
}

// outputTokensFor is how many tokens the model may write on one call: what the
// request asked for, or the configuration's own cap when the request asked for
// nothing. A cap that is not a positive number is refused here, naming the
// model, because on the wire it comes back as a plain refusal that names nothing
// and is neither of the two sentinels the harness knows how to act on.
func outputTokensFor(request contract.Request, modelName string) (int, error) {
	allowed := request.MaxOutputTokens
	if allowed == 0 {
		allowed = contract.DefaultConfig().Caps.OutputTokensPerCall
	}
	if allowed <= 0 {
		return 0, fmt.Errorf("the request to the model %q caps the reply at %d tokens, and a call needs a positive cap, so put output_tokens_per_call right in config.toml",
			modelName, request.MaxOutputTokens)
	}
	return allowed, nil
}

// New returns the model one configuration alias names, ready to call. An alias
// for a local server on a loopback address is probed once here, which is the
// only work New does over the network.
func New(alias contract.ModelAlias, options Options) (contract.Model, error) {
	if err := checkAlias(alias, options); err != nil {
		return nil, err
	}
	switch alias.Provider {
	case contract.ProviderAnthropic:
		return newAnthropicModel(alias, options), nil
	case contract.ProviderOpenAI:
		return newOpenAIModel(alias, options)
	case contract.ProviderCodex:
		return newCodexModel(alias, options), nil
	case contract.ProviderCommandLine:
		return newCommandLineModel(alias, options)
	default:
		return nil, fmt.Errorf("the model alias %q names the provider kind %q, and the four kinds are %q, %q, %q, and %q",
			alias.Name, alias.Provider, contract.ProviderAnthropic, contract.ProviderOpenAI, contract.ProviderCodex, contract.ProviderCommandLine)
	}
}

// checkAlias refuses an alias that is missing something a call cannot be made
// without, naming the alias so that the user can find it in config.toml.
func checkAlias(alias contract.ModelAlias, options Options) error {
	if options.Clock == nil {
		return fmt.Errorf("the model alias %q was built with no clock, and every wait in the provider is measured on one", alias.Name)
	}
	if alias.Name == "" {
		return fmt.Errorf("a model alias has no name, so give it one in config.toml such as %q", contract.LocalModelAlias)
	}
	if alias.ModelName == "" {
		return fmt.Errorf("the model alias %q names no model, so add the name the server or the program calls it, such as \"local-coder\"", alias.Name)
	}
	if alias.ContextLength <= 0 {
		return fmt.Errorf("the model alias %q reports a window of %d tokens, so set its context length to how many tokens the model holds",
			alias.Name, alias.ContextLength)
	}
	return CheckThink(alias, alias.Think)
}
