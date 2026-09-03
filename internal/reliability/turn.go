// The seams the rest of the program reaches the reliability layer through. Each
// one is a single call, because a mechanism that takes three calls to use is a
// mechanism a caller forgets to use: the turn lease, the turn deadline, and the
// delivery ledger were all built in wave 4 and all still had no caller when the
// wave 6 review read the code.

package reliability

import (
	"context"
)

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
