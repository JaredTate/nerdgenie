package testkit

import (
	"fmt"
	"maps"
	"slices"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The names the fixture writes a record update under. They are the fields of the
// task tool, and a fixture that wrote anything else would be read by nobody, so
// the loader refuses one it does not know.
const (
	updateWhy      = "why"
	updateDoneWhen = "doneWhen"
	updateStopWhen = "stopWhen"
	updatePlan     = "plan"
	updateDecision = "decision"
	updateFailure  = "failure"
)

// FortyStepUpdate is one round's record write, read out of the fixture's JSON
// into the shapes the contract uses. Every package that drives the fixture reads
// a record write through this, rather than each writing its own reader for the
// same map and drifting apart.
//
// A decision and a failure carry no identifier: the record hands those out when
// it takes the write, and the fixture does not say what they will be.
type FortyStepUpdate struct {
	// Why is the one line on why the user wants it, or empty.
	Why string
	// DoneWhen is the done list, in the model's words, or nil.
	DoneWhen []string
	// StopWhen is the stop list, or nil.
	StopWhen []string
	// Plan is the list of steps, or nil.
	Plan []string
	// Decision is the choice the model made this round, with its reason, or nil.
	Decision *contract.Decision
	// Failure is what went wrong this round, with its cause, or nil.
	Failure *contract.Failure
}

// Empty says the round wrote nothing into the record.
func (update FortyStepUpdate) Empty() bool {
	return update.Why == "" && update.DoneWhen == nil && update.StopWhen == nil &&
		update.Plan == nil && update.Decision == nil && update.Failure == nil
}

// Update is what the model writes into the record this round, read into the
// contract's own shapes. A round that writes nothing returns the zero value.
func (round FortyStepRound) Update() (FortyStepUpdate, error) {
	update := FortyStepUpdate{}
	for _, name := range slices.Sorted(maps.Keys(round.TaskUpdate)) {
		if err := update.take(round, name); err != nil {
			return FortyStepUpdate{}, err
		}
	}
	return update, nil
}

// take reads one named piece of a round's record write.
func (update *FortyStepUpdate) take(round FortyStepRound, name string) error {
	written := round.TaskUpdate[name]
	switch name {
	case updateWhy:
		update.Why = textOf(written)
	case updateDoneWhen:
		update.DoneWhen = textsIn(written)
	case updateStopWhen:
		update.StopWhen = textsIn(written)
	case updatePlan:
		update.Plan = textsIn(written)
	case updateDecision:
		pair := namedTextIn(written)
		update.Decision = &contract.Decision{Text: pair["text"], Reason: pair["reason"]}
	case updateFailure:
		pair := namedTextIn(written)
		update.Failure = &contract.Failure{Text: pair["text"], Cause: pair["cause"]}
	default:
		return fmt.Errorf("round %d writes %q into the record, and nothing knows how to read that, so add it to the loader or take it out of the fixture",
			round.Number, name)
	}
	return nil
}

// namedTextIn reads a decision or a failure, which the fixture writes as two
// named pieces of text.
func namedTextIn(value any) map[string]string {
	fields, isObject := value.(map[string]any)
	if !isObject {
		return map[string]string{}
	}
	pair := map[string]string{}
	for name, held := range fields {
		pair[name] = textOf(held)
	}
	return pair
}
