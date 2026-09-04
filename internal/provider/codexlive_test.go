//go:build live

package provider_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/provider"
)

// liveCodexBackendModel is the model the Codex backend is asked for, which is
// the one the "codex" provider's example block in config.toml names.
const liveCodexBackendModel = "gpt-5.6-sol"

// TestGPTThroughTheCodexBackendAnswersOneWordAndReportsItsUsage is the proof
// that the "codex" provider reaches GPT on the ChatGPT subscription with the
// sign-in the codex program keeps: one short ask, the one word back, and the
// token counts the backend reported. A missing or expired sign-in is a failure
// that says to run codex once, never a skip.
func TestGPTThroughTheCodexBackendAnswersOneWordAndReportsItsUsage(t *testing.T) {
	options, recorder := liveOptions(t)
	model, err := provider.New(contract.ModelAlias{
		Name:          "gpt",
		Provider:      contract.ProviderCodex,
		ModelName:     liveCodexBackendModel,
		ContextLength: 400000,
	}, options)
	if err != nil {
		t.Fatalf("the Codex backend provider could not be built: %v", err)
	}

	ctx, giveUp := context.WithTimeout(context.Background(), liveCallTimeout)
	defer giveUp()
	request := oneToolRequest("Reply with the single word ok.")
	reply, streamed, err := sendAndCollect(ctx, model, request)
	if err != nil {
		t.Fatalf("the Codex backend failed on a one-word ask, so check that codex is signed in: %v", err)
	}
	if !strings.Contains(strings.ToLower(reply.Text), "ok") {
		t.Errorf("the Codex backend answered %q, want the single word ok", reply.Text)
	}
	if streamed != reply.Text {
		t.Errorf("the deltas joined to %q and the reply is %q", streamed, reply.Text)
	}
	if reply.Usage.InputTokens == 0 || reply.Usage.OutputTokens == 0 {
		t.Errorf("the Codex backend reported the usage as %+v, and both counts must come back", reply.Usage)
	}
	if reply.Usage.CostUSD != 0 {
		t.Errorf("the Codex backend reported a cost of %f, and a subscription has no price per call", reply.Usage.CostUSD)
	}
	reportUsage(t, "codex backend", reply, recorder)
}
