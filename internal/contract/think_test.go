package contract_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// TestTheThinkLevelsReadEasiestFirst holds the ladder a person climbs: off,
// low, medium, high, xhigh, max. The order is the order "/think" prints them
// in, so a person can see which way is harder.
func TestTheThinkLevelsReadEasiestFirst(t *testing.T) {
	want := []contract.Think{
		contract.ThinkOff, contract.ThinkLow, contract.ThinkMedium,
		contract.ThinkHigh, contract.ThinkXHigh, contract.ThinkMax,
	}
	if got := contract.ThinkLevels(); !slices.Equal(got, want) {
		t.Errorf("the think levels are %v, want %v, easiest first", got, want)
	}
}

// TestKnownThinkTakesTheSixLevelsAndTheEmptyDefaultAndNothingElse holds the
// rule the configuration and the "/think" command are both checked against: a
// level is one of the six, or empty, which means the provider's own default.
func TestKnownThinkTakesTheSixLevelsAndTheEmptyDefaultAndNothingElse(t *testing.T) {
	for _, level := range contract.ThinkLevels() {
		if !contract.KnownThink(level) {
			t.Errorf("the think level %q was not recognised, and it is one of the six", level)
		}
	}
	if !contract.KnownThink(contract.ThinkDefault) {
		t.Error("the empty think level was not recognised, and it means the provider's own default")
	}
	for _, level := range []contract.Think{"hardest", "Medium", "medium ", "none", "on"} {
		if contract.KnownThink(level) {
			t.Errorf("the think level %q was recognised, and nobody offers it", level)
		}
	}
}

// TestTheThinkLevelsAreNamedInOneSentence holds that every message about a bad
// level can name the good ones, which is what the person needs to fix it.
func TestTheThinkLevelsAreNamedInOneSentence(t *testing.T) {
	sentence := contract.ThinkLevelsSentence()
	for _, level := range contract.ThinkLevels() {
		if !strings.Contains(sentence, string(level)) {
			t.Errorf("the sentence naming the levels is %q, and it leaves out %q", sentence, level)
		}
	}
}

// TestAModelAliasAndACallBothCarryAThinkLevel holds the two places the level
// travels: the alias, which is what config.toml writes, and the request, which
// is what the loop fills in for the call being made now.
func TestAModelAliasAndACallBothCarryAThinkLevel(t *testing.T) {
	alias := contract.ModelAlias{Name: "local", Think: contract.ThinkHigh}
	if alias.Think != contract.ThinkHigh {
		t.Errorf("the alias thinks at %q, want %q", alias.Think, contract.ThinkHigh)
	}
	request := contract.Request{Think: contract.ThinkMax}
	if request.Think != contract.ThinkMax {
		t.Errorf("the request thinks at %q, want %q", request.Think, contract.ThinkMax)
	}
}

// TestAFreshConfigurationNamesNoThinkLevel holds that the shipped alias keeps
// the behaviour Nerd Genie had before this setting existed: nothing is sent, and the
// provider's own default stands.
func TestAFreshConfigurationNamesNoThinkLevel(t *testing.T) {
	for _, alias := range contract.DefaultConfig().Models {
		if alias.Think != contract.ThinkDefault {
			t.Errorf("the shipped alias %q thinks at %q, want the provider's own default", alias.Name, alias.Think)
		}
	}
}
