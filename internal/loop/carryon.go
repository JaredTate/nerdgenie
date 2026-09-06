package loop

import (
	"context"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// theWaysAReplyOffersToCarryOn is the short fixed list of ways a last line asks
// for permission to go on rather than asking anything about the work. It is a
// fixed list for the same reason theWaysAReplyAsks is one.
var theWaysAReplyOffersToCarryOn = []string{
	"want me to", "keep rolling", "keep going", "carry on", "shall i continue", "should i continue",
	"proceed", "move on to", "next up", "where to next", "what next", "what's next",
}

// onlyOffersToCarryOn says whether a job's task ended on an offer to go on
// with the work rather than a question about it. A live job stopped at one of
// eleven tasks done because its task ended "Want me to keep rolling?" over a
// done list with nothing marked: the question mark read as a question for the
// person and the whole job was put down on it. Inside a job the person has
// already said to go on, so the offer is no question, and an unproven done
// list sends the model back to work. A person's own task is theirs to steer,
// so the same words on one still wait for the answer.
func (running *run) onlyOffersToCarryOn(text string) bool {
	if running.task.FromJob == nil {
		return false
	}
	return offersToCarryOn(lastLine(text))
}

// offersToCarryOn says whether this last line is one of the fixed ways of
// offering to go on.
func offersToCarryOn(last string) bool {
	last = strings.ToLower(last)
	for _, offer := range theWaysAReplyOffersToCarryOn {
		if strings.Contains(last, offer) {
			return true
		}
	}
	return false
}

// closeTheRecord writes where the task ended under the wrap-up time of its own,
// before the review is asked, so that the review's model call is not counted
// against the ten seconds the bookkeeping gets, and the report after the review
// starts with the ten seconds whole. The live game build lost its stopped
// report this way: the review on the local model was cut off at ten seconds,
// the fallback chain tried the two other models with the same cancelled
// context, and the report never reached the log.
func (running *run) closeTheRecord(ctx context.Context, status contract.RecordStatus) error {
	ctx, done := running.timeToWrapUp(ctx)
	defer done()
	return running.setStatus(ctx, status)
}
