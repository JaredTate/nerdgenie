package provider_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// requestWithOutputCap is the standard request with the cap on the reply set to
// one number, which is the only thing these tests vary.
func requestWithOutputCap(outputCap int) contract.Request {
	request := requestWithEverything()
	request.MaxOutputTokens = outputCap
	return request
}

func TestARequestThatNamesNoOutputCapGetsTheOneFromTheConfiguration(t *testing.T) {
	wanted := float64(contract.DefaultConfig().Caps.OutputTokensPerCall)

	t.Run("the Anthropic API", func(t *testing.T) {
		server := testkit.NewFakeProviderServer(scriptSayingOneThing("done"))
		defer server.Close()
		model, _, _ := anthropicAgainst(t, server)

		if _, err := model.Send(context.Background(), requestWithOutputCap(0), nil); err != nil {
			t.Fatalf("a call whose request named no output cap failed: %v", err)
		}

		if got := bodyOfLastCallTo(t, server, testkit.AnthropicPath)["max_tokens"]; got != wanted {
			t.Errorf("the request caps the output at %v, want the configuration's %v, because this API refuses a zero", got, wanted)
		}
	})

	t.Run("the OpenAI-compatible API", func(t *testing.T) {
		server := testkit.NewFakeProviderServer(scriptSayingOneThing("done"))
		defer server.Close()
		model, _ := openAIAgainst(t, server)

		if _, err := model.Send(context.Background(), requestWithOutputCap(0), nil); err != nil {
			t.Fatalf("a call whose request named no output cap failed: %v", err)
		}

		if got := bodyOfLastCallTo(t, server, testkit.OpenAIPath)["max_completion_tokens"]; got != wanted {
			t.Errorf("the request caps the output at %v, want the configuration's %v", got, wanted)
		}
	})
}

func TestARequestWhoseOutputCapIsNotAPositiveNumberIsRefusedByName(t *testing.T) {
	t.Run("the Anthropic API", func(t *testing.T) {
		server := testkit.NewFakeProviderServer(scriptSayingOneThing("never reached"))
		defer server.Close()
		model, _, _ := anthropicAgainst(t, server)

		_, err := model.Send(context.Background(), requestWithOutputCap(-1), nil)

		assertRefusedTheImpossibleCap(t, err, "opus")
		if calls := callsTo(server, testkit.AnthropicPath); calls != 0 {
			t.Errorf("the model was called %d times, and a cap the API cannot use is caught before the wire", calls)
		}
	})

	t.Run("the OpenAI-compatible API", func(t *testing.T) {
		server := testkit.NewFakeProviderServer(scriptSayingOneThing("never reached"))
		defer server.Close()
		model, _ := openAIAgainst(t, server)

		_, err := model.Send(context.Background(), requestWithOutputCap(-1), nil)

		assertRefusedTheImpossibleCap(t, err, contract.LocalModelAlias)
		if calls := callsTo(server, testkit.OpenAIPath); calls != 0 {
			t.Errorf("the model was called %d times, and a cap the API cannot use is caught before the wire", calls)
		}
	})
}

// assertRefusedTheImpossibleCap checks that a request whose cap on the reply is
// not a positive number came back as a refusal naming the model, rather than as
// a bare four hundred from the server that names nothing.
func assertRefusedTheImpossibleCap(t *testing.T, err error, modelName string) {
	t.Helper()
	if err == nil {
		t.Fatal("a request capping the reply at less than one token was sent to the model anyway")
	}
	if !strings.Contains(err.Error(), modelName) {
		t.Errorf("the refusal does not name the model it was for: %v", err)
	}
	if !strings.Contains(err.Error(), "output_tokens_per_call") {
		t.Errorf("the refusal does not say which setting to put right: %v", err)
	}
}
