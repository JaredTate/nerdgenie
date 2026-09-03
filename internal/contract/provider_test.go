package contract_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

func TestThereAreFourProviderKindsAndTheLastTwoRunOnASubscription(t *testing.T) {
	kinds := contract.ProviderKinds()

	if len(kinds) != 4 {
		t.Fatalf("there are %d provider kinds, want four: the Anthropic API, the OpenAI-compatible API, a command-line program, and the Codex backend", len(kinds))
	}
	wanted := map[contract.ProviderKind]bool{
		contract.ProviderAnthropic:   true,
		contract.ProviderOpenAI:      true,
		contract.ProviderCommandLine: true,
		contract.ProviderCodex:       true,
	}
	for _, kind := range kinds {
		if !wanted[kind] {
			t.Errorf("the provider kind %q is not one of the four", kind)
		}
		if !contract.KnownProviderKind(kind) {
			t.Errorf("the provider kind %q is listed but not recognised", kind)
		}
	}
	if contract.KnownProviderKind("carrier pigeon") {
		t.Error("a provider kind nobody defined was recognised")
	}
}

func TestTheProviderKindsAreWrittenTheWayTheConfigurationFileWritesThem(t *testing.T) {
	tests := []struct {
		kind contract.ProviderKind
		want string
	}{
		{contract.ProviderAnthropic, "anthropic"},
		{contract.ProviderOpenAI, "openai"},
		{contract.ProviderCommandLine, "cli"},
		{contract.ProviderCodex, "codex"},
	}
	for _, test := range tests {
		if string(test.kind) != test.want {
			t.Errorf("the provider kind is written %q in config.toml, want %q", test.kind, test.want)
		}
	}
}

func TestACommandLineAliasNamesAProgramAndNeedsNoAddressOrKey(t *testing.T) {
	alias := contract.ModelAlias{
		Name:      "opus",
		Provider:  contract.ProviderCommandLine,
		Program:   contract.ClaudeProgram,
		ModelName: "opus",
	}

	if alias.Program != "claude" {
		t.Errorf("the Anthropic program is %q, want claude", alias.Program)
	}
	if contract.CodexProgram != "codex" {
		t.Errorf("the OpenAI program is %q, want codex", contract.CodexProgram)
	}
	if alias.BaseAddress != "" || alias.KeyReference != "" {
		t.Error("a command-line alias carries an address or a key, and it needs neither because it runs the vendor's own program")
	}
}

// TestACodexAliasNeedsNoAddressNoProgramAndNoKey pins the fourth provider kind:
// OpenAI's Codex backend on the ChatGPT subscription, reached with the login the
// codex program keeps. It has one backend, so there is no address to write; Coeus
// drives it with its own loop, so there is no program to run; and the login is
// the subscription's, so there is no key.
func TestACodexAliasNeedsNoAddressNoProgramAndNoKey(t *testing.T) {
	alias := contract.ModelAlias{
		Name:          "gpt",
		Provider:      contract.ProviderCodex,
		ModelName:     "gpt-5.6-sol",
		ContextLength: 400000,
	}

	if !contract.KnownProviderKind(alias.Provider) {
		t.Errorf("the provider kind %q is not recognised", alias.Provider)
	}
	if alias.BaseAddress != "" || alias.Program != "" || alias.KeyReference != "" {
		t.Error("a codex alias carries an address, a program, or a key, and it needs none of them because the backend is fixed and the login is the codex program's")
	}
}

func TestTheTextFormOfAToolCallIsTheOneShapeRepairMustRead(t *testing.T) {
	if contract.ToolCallOpenTag != "<tool_call>" {
		t.Errorf("a text tool call opens with %q, want <tool_call>", contract.ToolCallOpenTag)
	}
	if contract.ToolCallCloseTag != "</tool_call>" {
		t.Errorf("a text tool call closes with %q, want </tool_call>", contract.ToolCallCloseTag)
	}

	instruction := contract.ToolCallTextInstruction
	for _, wanted := range []string{contract.ToolCallOpenTag, contract.ToolCallCloseTag, `"name"`, `"arguments"`} {
		if !strings.Contains(instruction, wanted) {
			t.Errorf("the instruction sentence does not show %q:\n%s", wanted, instruction)
		}
	}
	if len(strings.Fields(instruction)) > 40 {
		t.Errorf("the instruction sentence is %d words, and it rides in every prompt, so keep it short:\n%s",
			len(strings.Fields(instruction)), instruction)
	}
}

func TestAReplyNamesTheModelThatAnsweredAndCarriesTheCostWhenKnown(t *testing.T) {
	reply := contract.Reply{Model: "claude-opus-4-8", Usage: contract.Usage{InputTokens: 10, OutputTokens: 5, CostUSD: 0.004}}
	if reply.Model != "claude-opus-4-8" {
		t.Errorf("the reply's model is %q, want the one that answered", reply.Model)
	}
	if reply.Usage.CostUSD != 0.004 {
		t.Errorf("the reply's cost is %v, want 0.004", reply.Usage.CostUSD)
	}
	if (contract.Usage{}).CostUSD != 0 {
		t.Error("an unknown cost should read as zero")
	}
}
