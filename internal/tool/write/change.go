package write

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// MaxPriorContentsBytes is how much of what a file held is kept in the log. A
// file bigger than this is still changed, and the log says the prior contents
// were too large to keep, so that undo can tell "there was nothing" from "there
// was more than we kept".
const MaxPriorContentsBytes = 4 << 20

// Change is the one door every file change goes through: it writes down what a
// file held before anything touches it, so that the undo command can put the
// file back. The edit tool uses this same type, because there is one shape of
// file-change event and one place that writes it.
type Change struct {
	// Log is the event log the file-change event is written to.
	Log contract.Store
	// TaskID is the task or job the change belongs to.
	TaskID string
	// Clock is where the time on the event comes from.
	Clock contract.Clock
}

// Before records what a file holds now and returns the mode to write it back
// with. It is called before the file is touched, and a change it refuses is a
// change that does not happen.
func (change Change) Before(ctx context.Context, path string) (fs.FileMode, error) {
	if change.Log == nil {
		return 0, errors.New("this tool has no event log to record the change in, so wire the log in before writing files")
	}
	body := contract.FileChangeBody{Path: path}
	mode := contract.DataFileMode

	about, err := os.Stat(path)
	switch {
	case err == nil && about.IsDir():
		return 0, fmt.Errorf("%s is a folder, so name a file to write rather than the folder holding it", path)
	case err == nil:
		mode = about.Mode().Perm()
		body.Existed = true
		body.Mode = uint32(mode)
		if body.PriorContents, err = priorContents(path, about.Size()); err != nil {
			return 0, err
		}
	case !errors.Is(err, os.ErrNotExist):
		return 0, fmt.Errorf("cannot look at %s before changing it, so check that the agent may read it: %w", path, err)
	}

	if err := change.record(ctx, body); err != nil {
		return 0, err
	}
	return mode, nil
}

// priorContents reads what a file holds, up to the cap, so that one enormous
// file cannot put the log out of reach of the machine's memory.
func priorContents(path string, size int64) ([]byte, error) {
	if size > MaxPriorContentsBytes {
		return nil, nil
	}
	held, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read what %s holds before changing it, so check that the agent may read it: %w", path, err)
	}
	return held, nil
}

// record writes one file-change event into the log.
func (change Change) record(ctx context.Context, body contract.FileChangeBody) error {
	written, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("cannot write the change to %s as JSON: %w", body.Path, err)
	}
	event := contract.Event{TaskID: change.TaskID, Kind: contract.EventFileChange, Body: written}
	if change.Clock != nil {
		event.Occurred = change.Clock.Now()
	}
	if _, err := change.Log.Append(ctx, event); err != nil {
		return fmt.Errorf("cannot record the change to %s in the log, so the file was left as it was: %w", body.Path, err)
	}
	return nil
}
