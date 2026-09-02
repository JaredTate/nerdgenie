package contract_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

func TestThereAreThreeProviderKindsAndTheThirdRunsAProgram(t *testing.T) {
	kinds := contract.ProviderKinds()

	if len(kinds) != 3 {
		t.Fatalf("there are %d provider kinds, want three: the Anthropic API, the OpenAI-compatible API, and a command-line program", len(kinds))
	}
	wanted := map[contract.ProviderKind]bool{
		contract.ProviderAnthropic:   true,
		contract.ProviderOpenAI:      true,
		contract.ProviderCommandLine: true,
	}
	for _, kind := range kinds {
		if !wanted[kind] {
			t.Errorf("the provider kind %q is not one of the three", kind)
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
