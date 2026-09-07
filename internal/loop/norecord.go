package loop

import (
	"context"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// answerWithNoRecord ends a task that never needed a tool, which is the one
// kind of task that makes no record at all.
func (running *run) answerWithNoRecord(ctx context.Context, text string) (Outcome, error) {
	proof, err := running.proveTheJobsDoneLines(ctx)
	if err != nil {
		return Outcome{}, err
	}
	ctx, done := running.timeToWrapUp(ctx)
	defer done()
	if err := running.send(ctx, text); err != nil {
		return Outcome{}, err
	}
	return Outcome{Status: contract.StatusDone, Report: text, JobProof: proof}, nil
}
