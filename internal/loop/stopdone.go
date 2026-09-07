package loop

import (
	"context"
	"fmt"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// The two readings of a stop the harness makes for itself: a stop that lands
// on a done list already proved is a finish, and a stop the harness's own
// guard made is one a job may pick up once by itself.

// stopOnTheGuard stops the task because the harness's own guard fired, the
// same call over and over or no progress twice over, and says so in the
// outcome, so that a job may pick the task up once by itself.
func (running *run) stopOnTheGuard(ctx context.Context, line string) (Outcome, error) {
	outcome, err := running.stopHere(ctx, line)
	outcome.ByTheGuard = outcome.Status == contract.StatusStopped
	return outcome, err
}

// everyDoneLineIsProved says the record may close: it has a done list, and
// every line of it points at the result or the reply that proves it.
func (running *run) everyDoneLineIsProved() bool {
	return running.keeper != nil && record.DoneCheck(running.keeper.Record()) == nil
}

// theStopTakenAsTheFinish is the report of a task whose stop line fired with
// every done line already proved.
func theStopTakenAsTheFinish(line string) string {
	return fmt.Sprintf("The task is done: every line of the done list points at the result that proves it. "+
		"The stop line %q fired as well and was taken as the finish rather than a stop, "+
		"because a task whose done list is all proved has nothing to stop for.", line)
}
