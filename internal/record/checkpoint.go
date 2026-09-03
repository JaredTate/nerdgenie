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
// back. A task saves one per round and a hundred rounds is its budget, so
// anything past this is a log that has gone wrong rather than a record.
const MaxCheckpoints = 10000

// Load reloads a record from its latest checkpoint. A task waiting on the user
// holds nothing in any model's memory in the meantime; this is how it picks up
// again, days later and possibly on another model.
func Load(ctx context.Context, store contract.Store, kind contract.RecordKind, recordID string) (*Keeper, error) {
	saved, err := checkpointsOf(ctx, store, kind, recordID)
	if err != nil {
		return nil, err
	}
	return holdCheckpoint(store, kind, saved, saved[len(saved)-1])
}

// LoadCheckpoint reloads one numbered checkpoint of a record, which is what a
// replay of a failed task starts from.
func LoadCheckpoint(ctx context.Context, store contract.Store, kind contract.RecordKind, recordID string, number int) (*Keeper, error) {
	saved, err := checkpointsOf(ctx, store, kind, recordID)
	if err != nil {
		return nil, err
	}
	for _, one := range saved {
		if one.Number == number {
			return holdCheckpoint(store, kind, saved, one)
		}
	}
	return nil, fmt.Errorf("there is no checkpoint numbered %d of %s %s, which has %d of them",
		number, kind, recordID, len(saved))
}

// Back reloads the checkpoint the given number of steps before the latest and
// saves it as a new checkpoint, which is what "/tasks 17 back 3" does. A task
// saves one checkpoint per round, so a step back is a round of work. Nothing in
// the log is lost: the checkpoints in between stay where they are, and the model
// carries on from an earlier moment down another path.
func Back(ctx context.Context, store contract.Store, kind contract.RecordKind, recordID string, steps int) (*Keeper, error) {
	if steps < 1 {
		return nil, fmt.Errorf("winding back %d steps is not a step at all, so pass one or more", steps)
	}
	saved, err := checkpointsOf(ctx, store, kind, recordID)
	if err != nil {
		return nil, err
	}
	latest := saved[len(saved)-1].Number
	wanted := latest - steps
	if wanted < 1 {
		return nil, fmt.Errorf("%s %s stands at checkpoint %d, so %d steps back is before it began: %w",
			kind, recordID, latest, steps, ErrBeforeTheFirstCheckpoint)
	}

	keeper, err := LoadCheckpoint(ctx, store, kind, recordID, wanted)
	if err != nil {
		return nil, err
	}
	keeper.checkpoint = latest
	if err := keeper.save(ctx); err != nil {
		return nil, err
	}
	return keeper, nil
}

// holdCheckpoint reads one saved checkpoint back into a keeper, with the ask put
// back from the checkpoint that carries it, and says no when what it holds is not
// the kind of record that was asked for.
func holdCheckpoint(store contract.Store, kind contract.RecordKind, saved []Checkpoint, one Checkpoint) (*Keeper, error) {
	held, err := one.Read(theAskAmong(saved, one))
	if err != nil {
		return nil, err
	}
	if held.Header.Kind != kind {
		return nil, fmt.Errorf("checkpoint %d holds a %s and a %s was asked for, so the log has them under one key: %w",
			one.Number, held.Header.Kind, kind, ErrWrongKind)
	}
	return hold(store, held, one.Number, askCarriedBy(one))
}

// theAskAmong is the user's ask read out of the checkpoint this one names for
// it, and is empty when this checkpoint carries its own or when the one it names
// is not there.
func theAskAmong(saved []Checkpoint, one Checkpoint) string {
	if one.CarriesTheAsk() {
		return ""
	}
	for _, carrying := range saved {
		if carrying.Number != one.AskFrom {
			continue
		}
		held, err := Parse([]byte(carrying.Text))
		if err != nil {
			return ""
		}
		return held.Goal.Ask
	}
	return ""
}

// askCarriedBy is the number of the checkpoint whose text holds the ask, seen
// from this one: its own number when it carries the ask, and the one it names
// otherwise.
func askCarriedBy(one Checkpoint) int {
	if one.CarriesTheAsk() {
		return one.Number
	}
	return one.AskFrom
}

// checkpointsOf reads every checkpoint of one record out of the log, in the order
// they were saved, and says so plainly when there are none.
func checkpointsOf(ctx context.Context, store contract.Store, kind contract.RecordKind, recordID string) ([]Checkpoint, error) {
	if store == nil {
		return nil, errors.New("a record is loaded from an event log, so pass the store")
	}
	if recordID == "" {
		return nil, errors.New("a record is loaded by its number, so pass the number of the task or job")
	}
	logKey := contract.RecordLogKey(kind, recordID)
	events, err := store.ByTask(ctx, logKey)
	if err != nil {
		return nil, fmt.Errorf("cannot read the log under %s: %w", logKey, err)
	}

	saved := []Checkpoint{}
	for _, event := range events {
		if event.Kind != contract.EventCheckpoint {
			continue
		}
		one := Checkpoint{}
		if err := json.Unmarshal(event.Body, &one); err != nil {
			return nil, fmt.Errorf("event %d under %s says it is a checkpoint and does not read as one: %w",
				event.Sequence, logKey, err)
		}
		saved = append(saved, one)
		if len(saved) > MaxCheckpoints {
			return nil, fmt.Errorf("%s %s has more than %d checkpoints, which is more than a record ever saves",
				kind, recordID, MaxCheckpoints)
		}
	}
	if len(saved) == 0 {
		return nil, fmt.Errorf("there is no checkpoint of %s %s in the log, so check the number you asked for", kind, recordID)
	}
	return saved, nil
}
