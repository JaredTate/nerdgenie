package record

import (
	"context"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// Unproven returns the lines of the done list with nothing behind them: the ones
// not marked done, the ones marked done that name neither a result nor a reply
// from the user, and the ones that name a result this record never wrote, because
// proof that points at nothing is no proof. A line in this list sends the model
// back to work, which is what stops it from declaring victory early.
//
// This is the reading half of the done-check. The mechanical checks that go on
// top of it, such as running a command a line names, are wave 3's.
func Unproven(held contract.Record) []contract.DoneLine {
	waiting := []contract.DoneLine{}
	for _, line := range held.Goal.DoneWhen {
		if line.Done && (line.UserReply != "" || recordHoldsResult(&held, line.ResultID)) {
			continue
		}
		waiting = append(waiting, line)
	}
	return waiting
}

// DoneCheck says whether a record may close. It refuses a done list with nothing
// in it, because a record that never said what done looks like has proved
// nothing, and it refuses any line that is still waiting, naming the lines.
func DoneCheck(held contract.Record) error {
	if len(held.Goal.DoneWhen) == 0 {
		return fmt.Errorf("%s %s has no done list, so write what done looks like before closing it: %w",
			held.Header.Kind, held.Header.ID, ErrDoneLineNeedsProof)
	}
	waiting := Unproven(held)
	if len(waiting) == 0 {
		return nil
	}
	texts := make([]string, 0, len(waiting))
	for _, line := range waiting {
		texts = append(texts, fmt.Sprintf("%q", line.Text))
	}
	return fmt.Errorf("%d of the %d done lines have nothing behind them, so name what proves each one: %s: %w",
		len(waiting), len(held.Goal.DoneWhen), strings.Join(texts, ", "), ErrDoneLineNeedsProof)
}

// SetStatus writes where the record stands. Closing it runs the done-check, so a
// record can never say it is done while a line of its done list is waiting.
//
// This one always saves its checkpoint, whatever its owner does about rounds:
// where a record stands is what a resume, a listing and a replay read, and an
// ending has no round after it to save what was waiting.
func (keeper *Keeper) SetStatus(ctx context.Context, status contract.RecordStatus) error {
	if !knownStatus(status) {
		return fmt.Errorf("%q is not where a record can stand, so use running, waiting, stopped, failed, or done", status)
	}
	return keeper.changeAndSave(ctx, func(into *contract.Record) error {
		if status == contract.StatusDone {
			if err := DoneCheck(*into); err != nil {
				return err
			}
		}
		into.Header.Status = status
		return nil
	})
}
