package loop

import (
	"context"
	"errors"
	"fmt"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// theRecordOfTheTask is the record of the task running now, found when it is
// asked for rather than when the tools were built.
type theRecordOfTheTask struct {
	running *run
}

// Record returns the record as it stands, which is empty until the first tool
// call has made one.
func (held theRecordOfTheTask) Record() contract.Record {
	if held.running.keeper == nil {
		return contract.Record{}
	}
	return held.running.keeper.Record()
}

// Apply writes the model's half of the record.
func (held theRecordOfTheTask) Apply(ctx context.Context, update record.Update) error {
	if held.running.keeper == nil {
		return errors.New("this task has no record yet, so ask for a tool before writing the record")
	}
	return held.running.keeper.Apply(ctx, update)
}

// Read brings back the whole text of one result by its label. On a task of a
// job made from a work order, the ask label brings back the job's whole ask,
// because that is the long one the task's front shows only a slice of.
func (held theRecordOfTheTask) Read(ctx context.Context, id string) (string, error) {
	if id == record.AskLabel && held.running.jobAsk != "" {
		return held.running.jobAsk, nil
	}
	if held.running.keeper == nil {
		return "", fmt.Errorf("this task has no result %s yet, because nothing has been run", id)
	}
	return held.running.keeper.Read(ctx, id)
}
