package log

import (
	"context"
	"fmt"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// Replay hands every event in the log to a function, in sequence order, and
// stops at the first error the function returns. It is how the agent rebuilds
// what it was doing after a crash, and how a logged task is re-run as a test.
//
// The rows arrive one at a time rather than all at once, so a log of any length
// replays in the memory of a single event, and the loop ends with the table.
// After every event the context is looked at, so a replay of a long log gives up
// promptly when the caller stops caring.
func (eventLog *Log) Replay(ctx context.Context, hand func(event contract.Event) error) error {
	rows, err := eventLog.reader.QueryContext(ctx, selectColumns+" ORDER BY sequence")
	if err != nil {
		return fmt.Errorf("cannot start replaying the log at %s: %w", eventLog.path, err)
	}
	defer rows.Close()

	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return fmt.Errorf("cannot read a row of the log at %s: %w", eventLog.path, err)
		}
		if err := hand(event); err != nil {
			return fmt.Errorf("the replay stopped at event %d: %w", event.Sequence, err)
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("the replay of the log at %s gave up after event %d: %w", eventLog.path, event.Sequence, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot finish replaying the log at %s: %w", eventLog.path, err)
	}
	return nil
}
