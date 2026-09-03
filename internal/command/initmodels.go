// The order of the menu follows ZeroClaw's quickstart at
// ~/Code/zeroclaw/crates/zeroclaw-runtime/src/quickstart/mod.rs, where a
// provider found on the machine is offered before one that needs a key, and a
// local provider is marked as local so the user can see that nothing leaves the
// machine. ZeroClaw reads a registry of dozens of providers; this looks for the
// six Coeus can actually reach and offers the ones that answered.

package command

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// The addresses and names "coeus init" knows the six models by. Each name is
// both a value of the --model flag and the alias written into config.toml.
const (
	// LMStudioAddress is where an LM Studio server answers on this machine.
	LMStudioAddress = "http://127.0.0.1:1234/v1"
	// OpenAIAddress is where the OpenAI API answers.
	OpenAIAddress = "https://api.openai.com/v1"
	// LMStudioAlias names a model served by LM Studio.
	LMStudioAlias = "lmstudio"
	// AnthropicAlias names the Anthropic API, reached with a key.
	AnthropicAlias = "anthropic"
	// OpenAIAlias names the OpenAI API, reached with a key.
	OpenAIAlias = "openai"
)

// The bounds on looking for a local model server. The server is on this machine,
// so an answer arrives at once or not at all.
const (
	// probeWait is how long a look for a local server waits for an answer.
	probeWait = 2 * time.Second
	// maxModelListBytes caps how much of a server's model list is read.
	maxModelListBytes = 1 << 20
)

// The context lengths written into a fresh configuration for the models whose
// window Coeus cannot ask for. Each one is a starting point with a comment in
// the file telling the user to set it to the window their model really has.
const (
	// cloudContextLength is the window written for the two cloud models.
	cloudContextLength = 200000
	// lmStudioContextLength is the window written for an LM Studio server.
	lmStudioContextLength = 32768
)

// modelChoice is one model "coeus init" can set up: the name the user types,
// the line the menu prints, whether it was found on this machine, whether it
// needs an API key, and the alias written into config.toml for it.
type modelChoice struct {
	name        string
	description string
	detected    bool
	needsKey    bool
	alias       contract.ModelAlias
}

// detectModels returns the six models in the order the menu offers them: the
// local daemon, an LM Studio server, the Claude subscription, the ChatGPT
// subscription, and then the two APIs that need a key.
func detectModels(ctx context.Context, setup Setup) []modelChoice {
	local := contract.DefaultConfig().Models[0]
	local.BaseAddress = setup.localAddress()
	lmStudio := setup.lmStudioAddress()
	lmStudioModel, lmStudioAnswered := firstModelName(ctx, lmStudio)

	return []modelChoice{{
		name:        contract.LocalModelAlias,
		description: "the local model daemon on this machine, at " + local.BaseAddress + ", which costs nothing and sends nothing away",
		detected:    serverAnswers(ctx, healthAddress(local.BaseAddress)),
		alias:       local,
	}, {
		name:        LMStudioAlias,
		description: "an LM Studio server on this machine, at " + lmStudio + ", serving " + lmStudioModel,
		detected:    lmStudioAnswered,
		alias: contract.ModelAlias{Name: LMStudioAlias, Provider: contract.ProviderOpenAI,
			BaseAddress: lmStudio, ModelName: lmStudioModel, ContextLength: lmStudioContextLength},
	}, {
		name:        contract.ClaudeProgram,
		description: "the Claude subscription you already pay for, through the claude program",
		detected:    onThePath(contract.ClaudeProgram),
		alias: contract.ModelAlias{Name: contract.ClaudeProgram, Provider: contract.ProviderCommandLine,
			Program: contract.ClaudeProgram, ModelName: "claude-opus-4-8", ContextLength: cloudContextLength},
	}, {
		name:        contract.CodexProgram,
		description: "the ChatGPT subscription you already pay for, through the codex program",
		detected:    onThePath(contract.CodexProgram),
		alias: contract.ModelAlias{Name: contract.CodexProgram, Provider: contract.ProviderCommandLine,
			Program: contract.CodexProgram, ModelName: "gpt-5.5", ContextLength: cloudContextLength},
	}, {
		name:        AnthropicAlias,
		description: "Anthropic, with an API key you pay for by the token",
		needsKey:    true,
		alias: contract.ModelAlias{Name: AnthropicAlias, Provider: contract.ProviderAnthropic,
			ModelName: "claude-opus-4-8", ContextLength: cloudContextLength,
			KeyReference: contract.SecretReferencePrefix + AnthropicAlias},
	}, {
		name:        OpenAIAlias,
		description: "OpenAI, with an API key you pay for by the token",
		needsKey:    true,
		alias: contract.ModelAlias{Name: OpenAIAlias, Provider: contract.ProviderOpenAI,
			BaseAddress: OpenAIAddress, ModelName: "gpt-5.5", ContextLength: cloudContextLength,
			KeyReference: contract.SecretReferencePrefix + OpenAIAlias},
	}}
}

// offered returns the models the menu shows: the ones found on this machine,
// and the two that need a key, which cannot be found by looking.
func offered(choices []modelChoice) []modelChoice {
	shown := []modelChoice{}
	for _, choice := range choices {
		if choice.detected || choice.needsKey {
			shown = append(shown, choice)
		}
	}
	return shown
}

// firstDetected is the model "coeus init --yes" takes: the first one found on
// this machine. It returns false when nothing was found, because a model that
// needs a key is never chosen without being asked for.
func firstDetected(choices []modelChoice) (modelChoice, bool) {
	for _, choice := range choices {
		if choice.detected {
			return choice, true
		}
	}
	return modelChoice{}, false
}

// byName finds one of the six by the name the --model flag gave.
func byName(choices []modelChoice, name string) (modelChoice, bool) {
	for _, choice := range choices {
		if choice.name == name {
			return choice, true
		}
	}
	return modelChoice{}, false
}

// choiceNames are the six names, for the message that says what --model accepts.
func choiceNames(choices []modelChoice) []string {
	names := make([]string, 0, len(choices))
	for _, choice := range choices {
		names = append(names, choice.name)
	}
	return names
}

// onThePath says whether a program is installed on this machine.
func onThePath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// healthAddress turns the base address of a llama-server daemon into the address
// of its health check, which sits beside the API rather than inside it.
func healthAddress(base string) string {
	trimmed := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(base), "/"), "/v1")
	return strings.TrimSuffix(trimmed, "/") + "/health"
}

// serverAnswers makes one read-only request and says whether something answered
// properly. Anything at all going wrong means the server is not there.
func serverAnswers(ctx context.Context, address string) bool {
	answer, done := askOnce(ctx, address)
	if answer == nil {
		return false
	}
	defer done()
	return answer.StatusCode == http.StatusOK
}

// firstModelName asks an OpenAI-compatible server what models it serves and
// takes the first, which is what LM Studio has loaded. It gives back a plain
// name and false when nothing answered, so that the alias still has a model
// name in it when the user picks LM Studio anyway.
func firstModelName(ctx context.Context, base string) (string, bool) {
	answer, done := askOnce(ctx, strings.TrimSuffix(base, "/")+"/models")
	if answer == nil {
		return "local-model", false
	}
	defer done()
	if answer.StatusCode != http.StatusOK {
		return "local-model", false
	}

	var served struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(answer.Body, maxModelListBytes)).Decode(&served); err != nil || len(served.Data) == 0 || served.Data[0].ID == "" {
		return "local-model", false
	}
	return served.Data[0].ID, true
}

// askOnce makes one bounded read-only request. It gives back nothing at all
// when the request could not be made, and a function to close what it opened.
func askOnce(ctx context.Context, address string) (*http.Response, func()) {
	asking, stop := context.WithTimeout(ctx, probeWait)
	request, err := http.NewRequestWithContext(asking, http.MethodGet, address, nil)
	if err != nil {
		stop()
		return nil, nil
	}
	answer, err := (&http.Client{Timeout: probeWait}).Do(request)
	if err != nil {
		stop()
		return nil, nil
	}
	return answer, func() {
		_ = answer.Body.Close()
		stop()
	}
}
