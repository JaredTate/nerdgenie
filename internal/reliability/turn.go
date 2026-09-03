// The seams the rest of the program reaches the reliability layer through. Each
// one is a single call, because a mechanism that takes three calls to use is a
// mechanism a caller forgets to use: the turn lease, the turn deadline, and the
// delivery ledger were all built in wave 4 and all still had no caller when the
// wave 6 review read the code.

package reliability

import (
	"context"
	"fmt"
)

// WhyNoNewTask is the sentence to send whoever asked for work when the guard
// will not start a task, and is empty when it will. The caller asks it before it
// picks up a message and before it hands a job's task to the loop, and sends
// what it says back to the person who asked, so that an agent which is starting
// no task says so instead of going quiet.
func (guard *Guard) WhyNoNewTask() string {
	if tripped, err := guard.breaker.Tripped(); err == nil && tripped {
		return fmt.Sprintf(
			"Coeus stopped and started several times in a row, so it is answering you but starting no task until it has been quiet for %s. Ask again then, or restart Coeus yourself.",
			QuietPeriod)
	}
	if why := guard.drain.Why(); why != "" {
		return why + ", so it is finishing the task it has and starting no new one. Ask again in a few minutes."
	}
	return ""
}

// RunTurn runs one turn for a session under both of the things a turn needs: the
// lease, so that two messages arriving at once on one session cannot write the
// same record twice, and the turn deadline, so that a wedged turn stops instead
// of holding the session forever. The turn is given a context that is cancelled
// when the time runs out, with ErrDeadlineExpired as its cause. A turn that
// cannot take the lease is refused with ErrTurnInProgress and never run.
func (guard *Guard) RunTurn(ctx context.Context, session string, turn func(ctx context.Context) error) error {
	lease, err := guard.AcquireTurn(ctx, session)
	if err != nil {
		return err
	}
	defer lease.Release()

	bounded, stopWatching := guard.TurnDeadline().Watch(ctx)
	defer stopWatching()
	return turn(bounded)
}

// Deliver is how every reply reaches the user: written into the event log
// first, sent second, marked delivered third. A reply that was written down and
// never marked is sent again by the next start with the duplicate marker in
// front of it, so a crash between the writing and the sending costs the user a
// repeated message rather than the answer they paid a turn for. A send that
// fails leaves the reply in the ledger on purpose and says why.
func (guard *Guard) Deliver(ctx context.Context, taskID string, channel string, text string) error {
	reply, err := guard.ledger.Record(ctx, taskID, channel, text)
	if err != nil {
		return err
	}
	if err := guard.settings.Send(ctx, channel, text); err != nil {
		return fmt.Errorf("the reply was written down and could not be sent, so Coeus will send it again when it next starts: %w", err)
	}
	return guard.ledger.MarkDelivered(ctx, reply)
}
