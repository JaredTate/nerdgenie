package provider

import (
	"slices"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// claudeAlias is a command-line alias for the claude program, thinking at the
// level given.
func claudeAlias(level contract.Think) contract.ModelAlias {
	return contract.ModelAlias{
		Name: "claude on a subscription", Provider: contract.ProviderCommandLine,
		Program: contract.ClaudeProgram, ModelName: "opus", ContextLength: 200000, Think: level,
	}
}

// codexAlias is a command-line alias for the codex program, thinking at the
// level given.
func codexAlias(level contract.Think) contract.ModelAlias {
	return contract.ModelAlias{
		Name: "codex on a subscription", Provider: contract.ProviderCommandLine,
		Program: contract.CodexProgram, ModelName: "gpt-5.5", ContextLength: 400000, Think: level,
	}
}

// TestTheLevelOnTheCallBeatsTheLevelInTheFile holds how the two halves of the
// setting meet: config.toml says how hard a model thinks, and the level the
// request carries, which is the one the person chose with "/think" this
// session, wins over it for as long as the session lasts.
func TestTheLevelOnTheCallBeatsTheLevelInTheFile(t *testing.T) {
	forEachCase := []struct {
		what      string
		inTheFile contract.Think
		onTheCall contract.Think
		want      contract.Think
	}{
		{"nothing anywhere", contract.ThinkDefault, contract.ThinkDefault, contract.ThinkDefault},
		{"only the file", contract.ThinkHigh, contract.ThinkDefault, contract.ThinkHigh},
		{"only the call", contract.ThinkDefault, contract.ThinkLow, contract.ThinkLow},
		{"the call over the file", contract.ThinkHigh, contract.ThinkOff, contract.ThinkOff},
	}
	for _, one := range forEachCase {
		t.Run(one.what, func(t *testing.T) {
			alias := contract.ModelAlias{Name: "local", Think: one.inTheFile}
			request := contract.Request{Think: one.onTheCall}
			if got := thinkFor(request, alias); got != one.want {
				t.Errorf("the call is made at %q, want %q", got, one.want)
			}
		})
	}
}

// TestTheClaudeProgramRefusesToStopThinking holds what one real run of the
// program proved: with MAX_THINKING_TOKENS=0 in its environment and --effort
// high, claude 2.1.259 still thought (319 and 379 thinking tokens against 450
// without it), so there is no way to switch its thinking off and the harness
// says so instead of pretending.
func TestTheClaudeProgramRefusesToStopThinking(t *testing.T) {
	err := CheckThink(claudeAlias(contract.ThinkDefault), contract.ThinkOff)
	if err == nil {
		t.Fatal("the claude program was allowed to think at \"off\", and it has no way to switch thinking off")
	}
	if !strings.Contains(err.Error(), string(contract.ThinkLow)) {
		t.Errorf("the refusal is %q, and it must name the level to use instead", err)
	}
	for _, level := range []contract.Think{contract.ThinkDefault, contract.ThinkLow, contract.ThinkMax} {
		if err := CheckThink(claudeAlias(contract.ThinkDefault), level); err != nil {
			t.Errorf("the claude program was refused the level %q, and it takes it: %v", level, err)
		}
	}
	if err := CheckThink(codexAlias(contract.ThinkDefault), contract.ThinkOff); err != nil {
		t.Errorf("the codex program was refused \"off\", and it takes it as no reasoning at all: %v", err)
	}
}

// TestAThinkLevelNobodyOffersIsRefusedByEveryProvider holds that a level which
// is not one of the six is caught before a call is made, with the good ones in
// the message.
func TestAThinkLevelNobodyOffersIsRefusedByEveryProvider(t *testing.T) {
	for _, alias := range []contract.ModelAlias{
		claudeAlias(contract.ThinkDefault),
		codexAlias(contract.ThinkDefault),
		{Name: "local", Provider: contract.ProviderOpenAI, ModelName: "local-coder", ContextLength: 4096},
	} {
		err := CheckThink(alias, contract.Think("hardest"))
		if err == nil {
			t.Fatalf("the model %q was allowed to think at \"hardest\", and nobody offers that level", alias.Name)
		}
		for _, level := range contract.ThinkLevels() {
			if !strings.Contains(err.Error(), string(level)) {
				t.Errorf("the refusal is %q, and it leaves out the level %q", err, level)
			}
		}
	}
}

// TestTheClaudeCommandLineCarriesTheEffort holds what the claude program takes:
// --effort with the level, which "claude --help" lists as low, medium, high,
// xhigh, and max.
func TestTheClaudeCommandLineCarriesTheEffort(t *testing.T) {
	for _, level := range []contract.Think{contract.ThinkLow, contract.ThinkMedium, contract.ThinkHigh, contract.ThinkXHigh, contract.ThinkMax} {
		model := &commandLineModel{alias: claudeAlias(level)}
		arguments, err := model.argumentsFor(t.TempDir(), "the system prompt", level)
		if err != nil {
			t.Fatalf("building the claude command line at %q failed: %v", level, err)
		}
		at := slices.Index(arguments, "--effort")
		if at < 0 || at+1 >= len(arguments) || arguments[at+1] != string(level) {
			t.Errorf("the claude command line does not ask for the effort %q: %v", level, arguments)
		}
	}

	model := &commandLineModel{alias: claudeAlias(contract.ThinkDefault)}
	arguments, err := model.argumentsFor(t.TempDir(), "the system prompt", contract.ThinkDefault)
	if err != nil {
		t.Fatalf("building the claude command line with no level failed: %v", err)
	}
	if slices.Contains(arguments, "--effort") {
		t.Errorf("the claude command line asks for an effort when nobody set one, and the program's own default must stand: %v", arguments)
	}
}

// TestTheCodexCommandLineCarriesTheReasoningEffort holds what the codex program
// takes: the reasoning effort as a configuration override. The values are the
// ones the program's own refusal named when it was asked for a bogus one:
// none, minimal, low, medium, high, xhigh, and max.
func TestTheCodexCommandLineCarriesTheReasoningEffort(t *testing.T) {
	forEachLevel := map[contract.Think]string{
		contract.ThinkOff:    "none",
		contract.ThinkLow:    "low",
		contract.ThinkMedium: "medium",
		contract.ThinkHigh:   "high",
		contract.ThinkXHigh:  "xhigh",
		contract.ThinkMax:    "max",
	}
	for level, wanted := range forEachLevel {
		model := &commandLineModel{alias: codexAlias(level)}
		arguments, err := model.argumentsFor(t.TempDir(), "the instructions", level)
		if err != nil {
			t.Fatalf("building the codex command line at %q failed: %v", level, err)
		}
		if !slices.Contains(arguments, "model_reasoning_effort="+wanted) {
			t.Errorf("the codex command line does not ask for the reasoning effort %q: %v", wanted, arguments)
		}
	}

	model := &commandLineModel{alias: codexAlias(contract.ThinkDefault)}
	arguments, err := model.argumentsFor(t.TempDir(), "the instructions", contract.ThinkDefault)
	if err != nil {
		t.Fatalf("building the codex command line with no level failed: %v", err)
	}
	for _, argument := range arguments {
		if strings.HasPrefix(argument, "model_reasoning_effort=") {
			t.Errorf("the codex command line sets a reasoning effort when nobody set one: %v", arguments)
		}
	}
}
