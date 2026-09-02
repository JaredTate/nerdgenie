// The design of a fixed-format record with rules on what may change comes from
// Prime Agent's goal state at
// ~/Code/prime-agent/packages/coding-agent/src/core/goals.ts, where the
// objective is validated on the way in and treated as the user's data rather
// than as instructions. The Go here is written fresh.

package record

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/JaredTate/coeus/internal/contract"
)

// Start is what a record is created from, on the first tool call of a piece of
// work. A question the agent can answer without a tool gets an answer and no
// record at all.
type Start struct {
	// Kind says whether this is a task or a job.
	Kind contract.RecordKind
	// ID is the number of the task or the job, as a string.
	ID string
	// Origin is the channel the ask came in on, such as "Signal".
	Origin string
	// Ask is the user's message, word for word. It is never edited afterwards.
	Ask string
	// RoundsLeft is the task's tool-round budget from the configuration, and is
	// unused on a job.
	RoundsLeft int
	// MinutesLeft is the task's minute budget from the configuration, and is
	// unused on a job.
	MinutesLeft int
}

// Checkpoint is one saved copy of a record, which is what a checkpoint event in
// the event log holds. Reloading one needs nothing but the log.
type Checkpoint struct {
	// Number counts from one, in the order the checkpoints were saved.
	Number int `json:"number"`
	// Text is the record, printed.
	Text string `json:"text"`
}

// StoredResult is the full text of one result, which is what a tool-result event
// in the log holds and what "read r7" brings back after the text itself has left
// the model's window.
type StoredResult struct {
	// ID is the result's label, such as "r7" or "j4.2".
	ID string `json:"id"`
	// Summary is the one line the record keeps.
	Summary string `json:"summary"`
	// Text is the whole of what the tool returned.
	Text string `json:"text"`
}

// Keeper owns one record and the event log behind it. Every change to the record
// goes through a method on this type, because that is where the rules live, and
// every change saves the next numbered checkpoint into the log. One keeper is
// used by one turn at a time, which is how the agent runs a task.
type Keeper struct {
	store      contract.Store
	record     contract.Record
	checkpoint int
}

// New creates a record on the first tool call and saves the first checkpoint.
func New(ctx context.Context, store contract.Store, start Start) (*Keeper, error) {
	if store == nil {
		return nil, errors.New("a record needs an event log to write to, so pass the store")
	}
	if start.Kind != contract.RecordTask && start.Kind != contract.RecordJob {
		return nil, fmt.Errorf("the kind %q is neither a task nor a job, so say which one this record is", start.Kind)
	}
	if _, isNumber := readCount(start.ID); !isNumber {
		return nil, fmt.Errorf("a record is numbered, and %q is not a whole number of at least one, so pass a number such as %q",
			start.ID, "17")
	}
	if start.Ask == "" {
		return nil, errors.New("a record needs the user's ask, word for word, so pass the message")
	}
	if start.RoundsLeft < 0 || start.MinutesLeft < 0 {
		return nil, fmt.Errorf("the budget is %d rounds and %d minutes, and neither may be below zero",
			start.RoundsLeft, start.MinutesLeft)
	}

	header := contract.Header{
		Kind:   start.Kind,
		ID:     start.ID,
		Status: contract.StatusRunning,
		Origin: start.Origin,
	}
	// A job carries progress where a task carries a budget, and a record never
	// holds a number its own text does not print, so a budget handed to a job is
	// left behind here rather than kept where nothing would ever show it.
	if start.Kind == contract.RecordTask {
		header.RoundsLeft, header.MinutesLeft = start.RoundsLeft, start.MinutesLeft
	}
	keeper := &Keeper{store: store, record: contract.Record{Header: header, Goal: contract.Goal{Ask: start.Ask}}}
	if err := checkItReadsBack(keeper.record); err != nil {
		return nil, err
	}
	if err := keeper.save(ctx); err != nil {
		return nil, err
	}
	return keeper, nil
}

// checkItReadsBack refuses a record whose printed form would read back as
// something else. It is what makes the promise of this package true rather than
// merely intended: the record and its text always say the same thing, so a task
// put down for days and picked up again is the task that was put down. A piece of
// text that cannot be written without changing the record's meaning is refused
// here, where the writer can rephrase it, rather than quietly rewritten later.
func checkItReadsBack(held contract.Record) error {
	printed := Print(held)
	read, err := Parse(printed)
	if err != nil {
		return fmt.Errorf("this change writes a record that cannot be read back, so shorten it or rephrase it: %w", err)
	}
	if !reflect.DeepEqual(read, held) {
		return fmt.Errorf("this change writes a record that reads back as something else, so rephrase the text: it may not carry %q, %q, %q, or %q where a line of a record puts its own marks",
			arrow, ", ", reasonJoin, causeJoin)
	}
	return nil
}

// hold makes a keeper over a record that is already written, which is what the
// checkpoint readers use after they have parsed one out of the log. It is not
// exported, because a record handed in from outside could carry a rewritten ask
// or an edited correction, and the rules of this package are only worth having
// if there is no way round them.
func hold(store contract.Store, held contract.Record, checkpoint int) (*Keeper, error) {
	if store == nil {
		return nil, errors.New("a record needs an event log to write to, so pass the store")
	}
	if held.Header.ID == "" {
		return nil, errors.New("this record carries no number, so it cannot be tied to the log")
	}
	if checkpoint < 1 {
		return nil, fmt.Errorf("this record is held at checkpoint %d, and checkpoints count from one", checkpoint)
	}
	return &Keeper{store: store, record: held, checkpoint: checkpoint}, nil
}

// Record returns a copy of the record, so that a caller reading it cannot change
// it behind the rules.
func (keeper *Keeper) Record() contract.Record {
	return cloneRecord(keeper.record)
}

// Text returns the record printed, which is what the working context puts in
// front of the model.
func (keeper *Keeper) Text() string {
	return string(Print(keeper.record))
}

// ID is the number of the task or job this record belongs to.
func (keeper *Keeper) ID() string {
	return keeper.record.Header.ID
}

// Kind says whether this record is a task or a job.
func (keeper *Keeper) Kind() contract.RecordKind {
	return keeper.record.Header.Kind
}

// LogKey is the id every event of this record is written under. A task's events
// go under its own number and a job's under "j" and its number, because task 17
// and job 17 are different records and their events must never mix. Anything else
// that logs an event about this record uses this key too.
func (keeper *Keeper) LogKey() string {
	return contract.RecordLogKey(keeper.Kind(), keeper.ID())
}

// LatestCheckpoint is the number of the last checkpoint saved, which counts from
// one and grows by one with every change.
func (keeper *Keeper) LatestCheckpoint() int {
	return keeper.checkpoint
}

// save writes the record into the log as the next numbered checkpoint. Every
// change calls it, which is what makes a task resumable days later on another
// model, and what "/tasks 17 back 3" winds through.
func (keeper *Keeper) save(ctx context.Context) error {
	number := keeper.checkpoint + 1
	body, err := json.Marshal(Checkpoint{Number: number, Text: string(Print(keeper.record))})
	if err != nil {
		return fmt.Errorf("cannot write checkpoint %d of %s %s as JSON: %w", number, keeper.Kind(), keeper.ID(), err)
	}
	event := contract.Event{TaskID: keeper.LogKey(), Kind: contract.EventCheckpoint, Body: body}
	if _, err := keeper.store.Append(ctx, event); err != nil {
		return fmt.Errorf("cannot save checkpoint %d of %s %s to the log: %w", number, keeper.Kind(), keeper.ID(), err)
	}
	keeper.checkpoint = number
	return nil
}

// mustBe says no to an operation that belongs to the other kind of record.
func (keeper *Keeper) mustBe(kind contract.RecordKind, what string) error {
	if keeper.Kind() == kind {
		return nil
	}
	return fmt.Errorf("%s belongs to a %s and this record is a %s: %w", what, kind, keeper.Kind(), ErrWrongKind)
}

// cloneRecord copies a record and every list inside it.
func cloneRecord(held contract.Record) contract.Record {
	held.Goal.DoneWhen = slices.Clone(held.Goal.DoneWhen)
	held.Rules.Corrections = slices.Clone(held.Rules.Corrections)
	held.Rules.StopWhen = slices.Clone(held.Rules.StopWhen)
	held.Work.Situation = slices.Clone(held.Work.Situation)
	held.Work.Plan = slices.Clone(held.Work.Plan)
	held.Work.Tasks = slices.Clone(held.Work.Tasks)
	held.Work.Results = slices.Clone(held.Work.Results)
	held.Lessons.Decisions = slices.Clone(held.Lessons.Decisions)
	held.Lessons.Failures = slices.Clone(held.Lessons.Failures)
	return held
}
