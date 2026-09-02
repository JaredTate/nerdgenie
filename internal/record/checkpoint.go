// The design of numbered snapshots with a step back comes from Prime Agent's
// refinement state at
// ~/Code/prime-agent/packages/coding-agent/src/core/refinement/refinement.ts,
// where every change is appended to a history with an identifier and rolling one
// back means restoring the state it recorded. The Go here is written fresh, and
// the history is the event log rather than a file of its own.

package record

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/JaredTate/coeus/internal/contract"
)

// MaxCheckpoints is the most checkpoints of one record this package will read
// back. A task of a hundred rounds saves a few hundred, so anything past this is
// a log that has gone wrong rather than a record.
const MaxCheckpoints = 10000

// Load reloads a record from its latest checkpoint. A task waiting on the user
// holds nothing in any model's memory in the meantime; this is how it picks up
// again, days later and possibly on another model.
func Load(ctx context.Context, store contract.Store, recordID string) (*Keeper, error) {
	saved, err := checkpointsOf(ctx, store, recordID)
	if err != nil {
		return nil, err
	}
	return holdCheckpoint(store, saved[len(saved)-1])
}

// LoadCheckpoint reloads one numbered checkpoint of a record, which is what a
// replay of a failed task starts from.
func LoadCheckpoint(ctx context.Context, store contract.Store, recordID string, number int) (*Keeper, error) {
	saved, err := checkpointsOf(ctx, store, recordID)
	if err != nil {
		return nil, err
	}
	for _, one := range saved {
		if one.Number == number {
			return holdCheckpoint(store, one)
		}
	}
	return nil, fmt.Errorf("there is no checkpoint numbered %d of record %s, which has %d of them",
		number, recordID, len(saved))
}

// Back reloads the checkpoint the given number of steps before the latest and
// saves it as a new checkpoint, which is what "/tasks 17 back 3" does. Nothing in
// the log is lost: the checkpoints in between stay where they are, and the model
// carries on from an earlier moment down another path.
func Back(ctx context.Context, store contract.Store, recordID string, steps int) (*Keeper, error) {
	if steps < 1 {
		return nil, fmt.Errorf("winding back %d steps is not a step at all, so pass one or more", steps)
	}
	saved, err := checkpointsOf(ctx, store, recordID)
	if err != nil {
		return nil, err
	}
	latest := saved[len(saved)-1].Number
	wanted := latest - steps
	if wanted < 1 {
		return nil, fmt.Errorf("record %s stands at checkpoint %d, so %d steps back is before it began: %w",
			recordID, latest, steps, ErrBeforeTheFirstCheckpoint)
	}

	keeper, err := LoadCheckpoint(ctx, store, recordID, wanted)
	if err != nil {
		return nil, err
	}
	keeper.checkpoint = latest
	if err := keeper.save(ctx); err != nil {
		return nil, err
	}
	return keeper, nil
}

// holdCheckpoint reads one saved checkpoint back into a keeper.
func holdCheckpoint(store contract.Store, saved Checkpoint) (*Keeper, error) {
	held, err := Parse([]byte(saved.Text))
	if err != nil {
		return nil, fmt.Errorf("checkpoint %d does not read as a record: %w", saved.Number, err)
	}
	return hold(store, held, saved.Number)
}

// checkpointsOf reads every checkpoint of one record out of the log, in the order
// they were saved, and says so plainly when there are none.
func checkpointsOf(ctx context.Context, store contract.Store, recordID string) ([]Checkpoint, error) {
	if store == nil {
		return nil, errors.New("a record is loaded from an event log, so pass the store")
	}
	if recordID == "" {
		return nil, errors.New("a record is loaded by its number, so pass the number of the task or job")
	}
	events, err := store.ByTask(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the log of record %s: %w", recordID, err)
	}

	saved := []Checkpoint{}
	for _, event := range events {
		if event.Kind != contract.EventCheckpoint {
			continue
		}
		one := Checkpoint{}
		if err := json.Unmarshal(event.Body, &one); err != nil {
			return nil, fmt.Errorf("event %d of record %s says it is a checkpoint and does not read as one: %w",
				event.Sequence, recordID, err)
		}
		saved = append(saved, one)
		if len(saved) > MaxCheckpoints {
			return nil, fmt.Errorf("record %s has more than %d checkpoints, which is more than a record ever saves",
				recordID, MaxCheckpoints)
		}
	}
	if len(saved) == 0 {
		return nil, fmt.Errorf("there is no checkpoint of record %s in the log, so check the number you asked for", recordID)
	}
	return saved, nil
}
