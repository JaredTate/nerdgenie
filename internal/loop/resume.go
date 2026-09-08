package loop

import (
	"context"
	"fmt"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// A task picked up again: its record is loaded from the log, its pins and the
// page it was on are taken back, and a stopped task gets a fresh budget.

// resume picks a waiting or stopped task up again from its last checkpoint,
// which is what the user's next message does even days later.
//
// The budget it picks up on is the record's own, so that a task cannot buy
// itself more rounds by waiting for an answer it asked for. A task that stopped
// because its budget ran out is the one exception, and it is the whole of
// carryOnFromAStop below.
func (running *run) resume(ctx context.Context) error {
	if running.task.ResumeID == "" {
		return nil
	}
	keeper, err := record.Load(ctx, running.theLoop.options.Store, contract.RecordTask, running.task.ResumeID)
	if err != nil {
		return fmt.Errorf("cannot pick task %s up again: %w", running.task.ResumeID, err)
	}
	keeper.SaveOncePerRound()
	running.keeper = keeper
	running.takeThePinsBackFromTheRecord(ctx)
	running.takeTheBrowserFactBackFromTheLog(ctx)
	header := keeper.Record().Header
	running.roundsAllowed, running.timeAllowed = 0, 0
	if !header.NoRoundBudget {
		running.roundsAllowed = header.RoundsLeft
	}
	if !header.NoTimeBudget {
		running.timeAllowed = time.Duration(header.MinutesLeft) * time.Minute
	}
	if err := running.carryOnFromAStop(ctx, header.Status); err != nil {
		return err
	}
	return keeper.SetStatus(ctx, contract.StatusRunning)
}

// carryOnFromAStop gives a stopped task a fresh budget, because a person who
// says to carry on is asking for more. A task whose budget ran out ends stopped
// with nothing left and a report saying to tell it how to carry on; picked up on
// the nothing it stopped with, it spent its two spare rounds writing that same
// ending again and told the person the budget was used up, which is the answer
// to a question they had already answered.
//
// A waiting task is left alone. There the model stopped the work to ask
// something, the budget was never what ran out, and a wait that bought rounds
// would be a way round the budget rather than an answer to the person.
func (running *run) carryOnFromAStop(ctx context.Context, standing contract.RecordStatus) error {
	if standing != contract.StatusStopped {
		return nil
	}
	running.roundsAllowed = budgetRounds(running.task, running.theLoop.options.Caps)
	running.timeAllowed = budgetTime(running.task, running.theLoop.options.Caps)
	given := describeBudget(running.roundsAllowed, running.timeAllowed)
	running.continuedFact = "the person asked this task to carry on, so it was given a fresh budget of " + given
	if given == "no budget" {
		running.continuedFact = "the person asked this task to carry on, and it runs with no budget"
	}
	if err := running.keeper.SetBudget(ctx, running.budgetLeft()); err != nil {
		return fmt.Errorf("cannot give task %s the fresh budget it was asked to carry on with: %w",
			running.task.ResumeID, err)
	}
	return nil
}

// pickedUpFromAStop says whether this run picked its task up after a stop,
// which carryOnFromAStop above writes down as the carry-on line of the
// situation and nothing else does. A task picked up on the answer to its own
// question was never stopped.
func (running *run) pickedUpFromAStop() bool {
	return running.continuedFact != ""
}
