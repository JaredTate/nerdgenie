package main

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// runTheThinkCommand runs "/think" with the arguments given and returns what it
// answered, failing when it refused and the test did not expect it to.
func runTheThinkCommand(t *testing.T, running *agent, arguments string) string {
	t.Helper()
	answer, err := running.thinkCommand().Run(context.Background(), arguments, contract.CommandContext{Channel: running.userChannel()})
	if err != nil {
		t.Fatalf("running /think %q failed: %v", arguments, err)
	}
	return answer
}

// TestTheThinkCommandOnItsOwnNamesTheLevelInUseAndTheOnesOnOffer holds what a
// person sees when they type "/think" with nothing after it.
func TestTheThinkCommandOnItsOwnNamesTheLevelInUseAndTheOnesOnOffer(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	answer := runTheThinkCommand(t, running, "")

	if !strings.Contains(answer, running.currentModel()) {
		t.Errorf("/think does not name the model in use %q:\n%s", running.currentModel(), answer)
	}
	for _, level := range contract.ThinkLevels() {
		if !strings.Contains(answer, string(level)) {
			t.Errorf("/think leaves out the level %q:\n%s", level, answer)
		}
	}
}

// TestTheThinkCommandSetsTheLevelForTheModelInUse holds the other half: the
// level is set for the rest of the session, the model alias the rest of the
// program reads carries it, and "/think" on its own says so afterwards.
func TestTheThinkCommandSetsTheLevelForTheModelInUse(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	answer := runTheThinkCommand(t, running, "medium")

	if !strings.Contains(answer, string(contract.ThinkMedium)) {
		t.Errorf("/think medium does not say what it set:\n%s", answer)
	}
	alias, found := aliasNamed(running.settings, running.currentModel())
	if !found {
		t.Fatalf("the configuration names no model called %q", running.currentModel())
	}
	if alias.Think != contract.ThinkMedium {
		t.Errorf("the model alias thinks at %q, want %q", alias.Think, contract.ThinkMedium)
	}
	if again := runTheThinkCommand(t, running, ""); !strings.Contains(again, string(contract.ThinkMedium)) {
		t.Errorf("/think does not say the level that was just set:\n%s", again)
	}
}

// TestTheThinkCommandTakesTheLevelHoweverItIsTyped holds that a level with
// spaces around it or capital letters in it is the level it looks like, because
// a person types what they mean rather than what a parser wants.
func TestTheThinkCommandTakesTheLevelHoweverItIsTyped(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	runTheThinkCommand(t, running, "  XHigh  ")

	alias, _ := aliasNamed(running.settings, running.currentModel())
	if alias.Think != contract.ThinkXHigh {
		t.Errorf("the model alias thinks at %q, want %q", alias.Think, contract.ThinkXHigh)
	}
}

// TestTheThinkCommandRefusesALevelNobodyOffers holds that a level that is not
// one of the six is refused with the six in the message and nothing is changed.
func TestTheThinkCommandRefusesALevelNobodyOffers(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	_, err := running.thinkCommand().Run(context.Background(), "hardest", contract.CommandContext{Channel: running.userChannel()})

	if err == nil {
		t.Fatal("/think hardest was accepted, and nobody offers that level")
	}
	for _, level := range contract.ThinkLevels() {
		if !strings.Contains(err.Error(), string(level)) {
			t.Errorf("the refusal is %q, and it leaves out the level %q", err, level)
		}
	}
	if alias, _ := aliasNamed(running.settings, running.currentModel()); alias.Think != contract.ThinkDefault {
		t.Errorf("the model alias thinks at %q after a refused level, and nothing should have changed", alias.Think)
	}
}

// TestTheThinkCommandIsRegisteredWithAHelpLine holds that the command is in the
// one registry, so that "/help" and the palette both show it.
func TestTheThinkCommandIsRegisteredWithAHelpLine(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	found, there := running.registry.Lookup(thinkName)
	if !there {
		t.Fatalf("/%s is not registered, and the person asked for it", thinkName)
	}
	if found.Help == "" {
		t.Errorf("/%s has no help line, and the palette shows one for every command", thinkName)
	}
}
