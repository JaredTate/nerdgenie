package record

import (
	"fmt"
	"slices"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// Proof is a result label on a done line or a plan step, and a label proves
// nothing unless this record wrote it.

// checkNamesAResult says no to a label that names a result this record never
// wrote, because proof that points at nothing is no proof.
func checkNamesAResult(into *contract.Record, id string, what string) error {
	if id == "" || recordHoldsResult(into, id) {
		return nil
	}
	return fmt.Errorf("%s names %q, which this record never wrote: %w", what, id, ErrNoSuchResult)
}

// recordHoldsResult says whether the record holds a result under this label.
func recordHoldsResult(into *contract.Record, id string) bool {
	if id == "" {
		return false
	}
	if slices.ContainsFunc(into.Work.Results, func(result contract.ResultLine) bool { return result.ID == id }) {
		return true
	}
	// A label the record reached is a label it wrote, whether or not its line
	// is still on the list, because the list lets its oldest lines go and the
	// log keeps every one: the fifth game build's play-test task could not
	// pin its last done line once the first line's result had left the list.
	number, valid := ResultNumber(into.Header, id)
	return valid && number >= 1 && number <= highestResultNumberOn(into)
}

// highestResultNumberOn is the largest label number the record's results have
// reached, or zero with none.
func highestResultNumberOn(into *contract.Record) int {
	highest := 0
	for _, result := range into.Work.Results {
		if number, valid := ResultNumber(into.Header, result.ID); valid && number > highest {
			highest = number
		}
	}
	return highest
}
