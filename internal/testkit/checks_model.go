package testkit

import (
	"context"
	"errors"
	"fmt"

	"github.com/JaredTate/coeus/internal/contract"
)

// CheckModel asserts what every model promises: it has a name and a window, the
// deltas it streams join to the text it returns, and it reports why it stopped.
func CheckModel(ctx context.Context, model contract.Model) error {
	if model.Name() == "" {
		return errors.New("the model has no name, and the record and the cost line both print it")
	}
	if model.ContextLength() <= 0 {
		return fmt.Errorf("the model reports a window of %d tokens, and the working-context rule is sized from it", model.ContextLength())
	}

	streamed := ""
	reply, err := model.Send(ctx, contract.Request{
		SystemBlocks: []contract.SystemBlock{{
			Name:     "harness rules and persona",
			Text:     "You are the reasoning engine inside Coeus.",
			Boundary: contract.CacheBoundaryA,
		}},
		Messages:        []contract.Message{{Role: contract.RoleUser, Text: "Say anything at all."}},
		MaxOutputTokens: 64,
	}, func(delta string) { streamed += delta })
	if err != nil {
		return fmt.Errorf("one call to the model failed: %w", err)
	}

	if streamed != reply.Text {
		return fmt.Errorf("the deltas joined to %q and the reply is %q, and the two must be the same", streamed, reply.Text)
	}
	switch reply.Finish {
	case contract.FinishEnd, contract.FinishToolCalls, contract.FinishLength, contract.FinishStopped:
	default:
		return fmt.Errorf("the reply finished with %q, want one of the four reasons the contract names", reply.Finish)
	}
	if reply.Usage.CachedInputTokens > reply.Usage.InputTokens {
		return fmt.Errorf("the reply says %d of %d input tokens were cached, and the cached count is a part of the input count",
			reply.Usage.CachedInputTokens, reply.Usage.InputTokens)
	}
	return nil
}
