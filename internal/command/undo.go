package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// MaxUndoFiles is the most files one turn may have changed for "/undo" to put
// them all back. A turn that touched more than this is not something to undo
// blind, so the command refuses and says what to do instead.
const MaxUndoFiles = 200

// Undo is the "/undo" command: it puts back what the last turn wrote.
//
// The event log is what decides what happened, so the last turn is everything
// after the newest message in it, and the file-change events in that stretch
// carry the contents each file held before it was touched. They are put back
// newest first, so that a file written twice in one turn ends up holding what it
// held before the turn began, and a file the turn created is removed rather
// than restored.
func (commands *Commands) Undo() contract.Command {
	return contract.Command{
		Name: "undo",
		Help: "Puts back the files the last turn changed.",
		Run: func(ctx context.Context, _ string, _ contract.CommandContext) (string, error) {
			if commands.deps.Store == nil {
				return "", notWiredUp("undo", "Store")
			}
			turn, err := lastTurn(ctx, commands.deps.Store)
			if err != nil {
				return "", err
			}
			switch {
			case !turn.started:
				return "there is no turn to undo yet, so nothing was changed.\n", nil
			case turn.tooMany:
				return fmt.Sprintf("the last turn changed more than %d files, which is more than /undo puts back at once, so put them back by hand or restore last night's backup.\n", MaxUndoFiles), nil
			case len(turn.changes) == 0:
				return "the last turn changed no files, so there is nothing to put back.\n", nil
			}
			return putBack(turn.changes)
		},
	}
}

// turnChanges is what one walk of the log found: whether a turn ever started,
// the file changes since the newest message, and whether there were more of
// them than "/undo" will touch.
type turnChanges struct {
	started bool
	tooMany bool
	changes []contract.FileChangeBody
}

// lastTurn walks the log once and keeps only the last turn's file changes.
// Replay is the one read with no cap on it, and every message starts the count
// again, so the walk holds at most MaxUndoFiles changes however long the log is.
func lastTurn(ctx context.Context, store contract.Store) (turnChanges, error) {
	turn := turnChanges{}
	err := store.Replay(ctx, func(event contract.Event) error {
		switch event.Kind {
		case contract.EventMessage:
			turn = turnChanges{started: true}
		case contract.EventFileChange:
			var change contract.FileChangeBody
			if err := json.Unmarshal(event.Body, &change); err != nil {
				return fmt.Errorf("the file-change event %d cannot be read, so nothing was put back: %w", event.Sequence, err)
			}
			if len(turn.changes) >= MaxUndoFiles {
				turn.tooMany = true
				return nil
			}
			turn.changes = append(turn.changes, change)
		}
		return nil
	})
	if err != nil {
		return turnChanges{}, fmt.Errorf("the event log could not be read, so nothing was put back: %w", err)
	}
	return turn, nil
}

// putBack applies the last turn's file changes newest first, which is what
// leaves every file holding what it held before the turn began.
func putBack(changes []contract.FileChangeBody) (string, error) {
	restored, removed := []string{}, []string{}
	for at := len(changes) - 1; at >= 0; at-- {
		change := changes[at]
		if change.Existed {
			if err := os.WriteFile(change.Path, change.PriorContents, fileMode(change)); err != nil {
				return "", fmt.Errorf("the file %s could not be put back, so check that the folder is writable: %w", change.Path, err)
			}
			restored = append(restored, change.Path)
			continue
		}
		if err := os.Remove(change.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("the file %s could not be removed, so check that the folder is writable: %w", change.Path, err)
		}
		removed = append(removed, change.Path)
	}
	return undoReport(restored, removed), nil
}

// fileMode is the mode a restored file gets: the one it had before the change,
// or the ordinary file mode when the event recorded none.
func fileMode(change contract.FileChangeBody) fs.FileMode {
	if change.Mode == 0 {
		return contract.DataFileMode
	}
	return fs.FileMode(change.Mode).Perm()
}

// undoReport says what was put back and what was taken away, one line each, so
// that the user can see exactly what "/undo" did.
func undoReport(restored []string, removed []string) string {
	written := &strings.Builder{}
	fmt.Fprintf(written, "undid the last turn: put back %s and removed %s.\n",
		countedFiles(len(restored)), countedFiles(len(removed)))
	for _, path := range restored {
		written.WriteString("  put back " + path + "\n")
	}
	for _, path := range removed {
		written.WriteString("  removed  " + path + "\n")
	}
	return written.String()
}

// countedFiles writes a number of files the way a person would say it.
func countedFiles(many int) string {
	switch many {
	case 0:
		return "no files"
	case 1:
		return "one file"
	default:
		return fmt.Sprintf("%d files", many)
	}
}
