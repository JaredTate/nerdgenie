package config_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/config"
	"github.com/JaredTate/nerdgenie/internal/contract"
)

// oneCodexAlias is a model reached through OpenAI's Codex backend on the ChatGPT
// subscription, written the way "nerdgenie init" writes it: no address, because the
// backend is fixed, and no program, because Nerd Genie's own loop drives the model.
const oneCodexAlias = `
default_model = "gpt"

[[models]]
name = "gpt"
provider = "codex"
model_name = "gpt-5.6-sol"
context_length = 400000
think = "medium"
`

// TestACodexAliasIsAcceptedWithNoAddressAndNoProgram holds the rule that the
// fourth provider kind needs only a model name and a context length, the way an
// openai alias needs those two and an address, because a person who wrote a
// base_address or a program for it would have nothing to point them at.
func TestACodexAliasIsAcceptedWithNoAddressAndNoProgram(t *testing.T) {
	settings, err := config.Load(writeConfig(t, oneCodexAlias))
	if err != nil {
		t.Fatalf("a codex alias with no address and no program was refused: %v", err)
	}
	if len(settings.Models) != 1 {
		t.Fatalf("the configuration has %d model aliases, want the one codex alias", len(settings.Models))
	}
	alias := settings.Models[0]
	if alias.Provider != contract.ProviderCodex {
		t.Errorf("the alias is reached through %q, want %q", alias.Provider, contract.ProviderCodex)
	}
	if alias.Program != "" {
		t.Errorf("the alias carries the program %q, and a codex alias runs none because Nerd Genie's own loop drives the model", alias.Program)
	}
	if alias.ModelName != "gpt-5.6-sol" || alias.ContextLength != 400000 || alias.Think != contract.ThinkMedium {
		t.Errorf("the alias was read as %+v, want the model name, the context length, and the think level as written", alias)
	}
	if settings.DefaultModel != "gpt" {
		t.Errorf("the default model is %q, want the codex alias", settings.DefaultModel)
	}
}

// TestAnUnknownProviderIsRefusedWithAllFourKindsNamed holds the rule that the
// message for a provider nobody has heard of lists every kind a person could
// write instead, and that the list now has the codex kind in it.
func TestAnUnknownProviderIsRefusedWithAllFourKindsNamed(t *testing.T) {
	problem := refuses(t, strings.Replace(oneGoodAlias, `provider = "openai"`, `provider = "carrier-pigeon"`, 1))
	if problem.Key != "models.0.provider" {
		t.Errorf("the problem names the key %q, want models.0.provider", problem.Key)
	}
	for _, kind := range []contract.ProviderKind{contract.ProviderAnthropic, contract.ProviderOpenAI, contract.ProviderCommandLine, contract.ProviderCodex} {
		if !strings.Contains(problem.Advice, `"`+string(kind)+`"`) {
			t.Errorf("the advice is %q, and it leaves out the provider kind %q a person could write instead", problem.Advice, kind)
		}
	}
}
