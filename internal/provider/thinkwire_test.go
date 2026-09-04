package provider_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/provider"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// thinkingAt returns the request every test in this file sends, made at one
// level.
func thinkingAt(level contract.Think) contract.Request {
	request := requestWithEverything()
	request.Think = level
	return request
}

// anthropicThinkingAt builds the Anthropic provider against the fake server and
// makes one call at the level given, returning the body that went on the wire.
func anthropicThinkingAt(t *testing.T, level contract.Think) map[string]any {
	t.Helper()
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("done"))
	defer server.Close()
	model, _, _ := anthropicAgainst(t, server)

	if _, err := model.Send(context.Background(), thinkingAt(level), nil); err != nil {
		t.Fatalf("one call to the Anthropic provider at %q failed: %v", level, err)
	}
	return bodyOfLastCallTo(t, server, testkit.AnthropicPath)
}

// TestTheMessagesAPIIsAskedForAdaptiveThinkingAndAnEffort holds the wire shape
// the Claude API reference gives for a thinking level: adaptive thinking with
// the effort beside it. The shape was read from
// /tmp/claude-1000/bundled-skills/2.1.259/e40adf0c7c495fe3df7108386b7e2dd8/claude-api/curl/examples.md,
// under "Extended Thinking".
func TestTheMessagesAPIIsAskedForAdaptiveThinkingAndAnEffort(t *testing.T) {
	for _, level := range []contract.Think{contract.ThinkLow, contract.ThinkMedium, contract.ThinkHigh, contract.ThinkXHigh, contract.ThinkMax} {
		body := anthropicThinkingAt(t, level)
		thinking, isObject := body["thinking"].(map[string]any)
		if !isObject || thinking["type"] != "adaptive" {
			t.Errorf("the request at %q carries %v as its thinking, want adaptive thinking", level, body["thinking"])
		}
		config, isObject := body["output_config"].(map[string]any)
		if !isObject || config["effort"] != string(level) {
			t.Errorf("the request at %q carries %v as its output config, want the effort %q", level, body["output_config"], level)
		}
	}
}

// TestTheMessagesAPIIsToldNothingAboutThinkingByDefault holds the behaviour
// Coeus had before this setting existed and keeps: with no level, and with the
// level "off", no thinking field and no effort is sent at all, so the model's
// own default stands.
func TestTheMessagesAPIIsToldNothingAboutThinkingByDefault(t *testing.T) {
	for _, level := range []contract.Think{contract.ThinkDefault, contract.ThinkOff} {
		body := anthropicThinkingAt(t, level)
		if _, there := body["thinking"]; there {
			t.Errorf("the request at %q carries a thinking field, and the default must send none: %v", level, body["thinking"])
		}
		if _, there := body["output_config"]; there {
			t.Errorf("the request at %q carries an output config, and the default must send none: %v", level, body["output_config"])
		}
	}
}

// TestTheChatCompletionsAPICarriesTheReasoningEffort holds what an
// OpenAI-compatible server is told: reasoning_effort with the level, for the
// five levels above off.
func TestTheChatCompletionsAPICarriesTheReasoningEffort(t *testing.T) {
	for _, level := range []contract.Think{contract.ThinkLow, contract.ThinkMedium, contract.ThinkHigh, contract.ThinkXHigh, contract.ThinkMax} {
		server := testkit.NewFakeProviderServer(scriptSayingOneThing("done"))
		model, _ := openAIAgainst(t, server)
		if _, err := model.Send(context.Background(), thinkingAt(level), nil); err != nil {
			t.Fatalf("one call to the OpenAI-compatible provider at %q failed: %v", level, err)
		}
		if got := bodyOfLastCallTo(t, server, testkit.OpenAIPath)["reasoning_effort"]; got != string(level) {
			t.Errorf("the request carries %v as its reasoning effort, want %q", got, level)
		}
		server.Close()
	}

	for _, level := range []contract.Think{contract.ThinkDefault, contract.ThinkOff} {
		server := testkit.NewFakeProviderServer(scriptSayingOneThing("done"))
		model, _ := openAIAgainst(t, server)
		if _, err := model.Send(context.Background(), thinkingAt(level), nil); err != nil {
			t.Fatalf("one call to the OpenAI-compatible provider at %q failed: %v", level, err)
		}
		if got, there := bodyOfLastCallTo(t, server, testkit.OpenAIPath)["reasoning_effort"]; there {
			t.Errorf("the request at %q carries the reasoning effort %v, and no effort belongs on it", level, got)
		}
		server.Close()
	}
}

// TestTheLocalServerIsToldToThinkOrNotToThink holds the llama-server hint: the
// server that answered the props probe is told to think for every level above
// off, and not to think for off and for the default, which is what Coeus sent
// before this setting existed.
func TestTheLocalServerIsToldToThinkOrNotToThink(t *testing.T) {
	forEachLevel := map[contract.Think]string{
		contract.ThinkDefault: `"chat_template_kwargs":{"enable_thinking":false}`,
		contract.ThinkOff:     `"chat_template_kwargs":{"enable_thinking":false}`,
		contract.ThinkLow:     `"chat_template_kwargs":{"enable_thinking":true}`,
		contract.ThinkMax:     `"chat_template_kwargs":{"enable_thinking":true}`,
	}
	for level, wanted := range forEachLevel {
		double := llamaServerSaying(262144)
		options, _ := testOptions(t, newTestClock())
		model, err := provider.New(localAliasAt(double.URL, 262144), options)
		if err != nil {
			t.Fatalf("building the local provider failed: %v", err)
		}
		if _, err := model.Send(context.Background(), thinkingAt(level), nil); err != nil {
			t.Fatalf("one call to the local provider at %q failed: %v", level, err)
		}
		if !strings.Contains(double.body(), wanted) {
			t.Errorf("the request at %q does not carry %s:\n%s", level, wanted, double.body())
		}
		double.Close()
	}
}

// TestAModelAliasSetToALevelNobodyOffersIsRefusedWhenItIsBuilt holds that a
// think level the file got wrong stops the model being built at all, so the
// person is told at startup rather than in the middle of a task.
func TestAModelAliasSetToALevelNobodyOffersIsRefusedWhenItIsBuilt(t *testing.T) {
	options, _ := testOptions(t, newTestClock())
	alias := contract.ModelAlias{
		Name: "opus", Provider: contract.ProviderAnthropic,
		ModelName: "claude-opus-4-8", ContextLength: 200000, Think: contract.Think("hardest"),
	}

	if _, err := provider.New(alias, options); err == nil {
		t.Fatal("a model alias asking to think at \"hardest\" was built, and nobody offers that level")
	}
}
