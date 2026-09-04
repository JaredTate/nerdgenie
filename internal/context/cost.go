package context

import (
	"context"
	"errors"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// WriteCostLine writes what one call cost into the record's header, in the
// design's own words: "this turn: 6.1k tokens in, 5.2k of them cached, 0.4k
// out". The caller hands over the usage the provider reported as soon as the
// call comes back.
//
// The numbers are the provider's own and never this package's estimate. The
// estimate is what sizes the window before a call; only the provider knows what
// the call actually cost, and the user is shown that.
func WriteCostLine(ctx context.Context, keeper *record.Keeper, usage contract.Usage) error {
	if keeper == nil {
		return errors.New("the cost line has no record to be written into, so pass the keeper of the task being worked on")
	}
	return keeper.SetCost(ctx, contract.CostLine{
		InputTokens:       usage.InputTokens,
		CachedInputTokens: usage.CachedInputTokens,
		OutputTokens:      usage.OutputTokens,
	})
}
