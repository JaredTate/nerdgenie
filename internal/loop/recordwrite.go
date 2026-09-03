package loop

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
)

// recordWrite is what the model may write into the record, as it writes it in
// the arguments of a task call. The names are the ones the forty-step fixture
// and brief 2.5 use, and anything else in the object is left alone, so that a
// model that adds a field of its own is not refused over it.
type recordWrite struct {
	// Why is the one line on why the user wants this.
	Why string `json:"why"`
	// DoneWhen is the whole done list.
	DoneWhen []doneLineWrite `json:"doneWhen"`
	// StopWhen is the whole stop list.
	StopWhen []string `json:"stopWhen"`
	// Plan is a task's plan, one line per step.
	Plan []string `json:"plan"`
	// Tasks is a job's task list.
	Tasks []jobTaskWrite `json:"tasks"`
	// Decision is one choice with the reason it must carry.
	Decision *pairWrite `json:"decision"`
	// Failure is one thing that went wrong with the cause it must carry.
	Failure *pairWrite `json:"failure"`
}

// doneLineWrite is one line of the done list. A model may write it as plain
// text, which is a line with nothing behind it yet, or as an object naming the
// result or the user's reply that proves it.
type doneLineWrite struct {
	contract.DoneLine
}

// jobTaskWrite is one task on a job's list as the model writes it.
type jobTaskWrite struct {
	// TaskID is the task's label, such as "t31".
	TaskID string `json:"taskId"`
	// Text says what the task does.
	Text string `json:"text"`
	// DueAt is the date it waits for, in plain words.
	DueAt string `json:"dueAt"`
}

// pairWrite is a decision or a failure, which are each two pieces of text.
type pairWrite struct {
	// Text is the choice, or what went wrong.
	Text string `json:"text"`
	// Reason is why a decision was made.
	Reason string `json:"reason"`
	// Cause is why a failure happened.
	Cause string `json:"cause"`
}

// UnmarshalJSON reads one done line written either as text or as an object.
func (line *doneLineWrite) UnmarshalJSON(raw []byte) error {
	var written string
	if err := json.Unmarshal(raw, &written); err == nil {
		line.DoneLine = contract.DoneLine{Text: written}
		return nil
	}
	var object struct {
		Text      string `json:"text"`
		Done      bool   `json:"done"`
		ResultID  string `json:"resultId"`
		UserReply string `json:"userReply"`
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return fmt.Errorf("a done line is a line of text, or an object with a text and the result that proves it: %w", err)
	}
	line.DoneLine = contract.DoneLine{
		Text: object.Text, Done: object.Done, ResultID: object.ResultID, UserReply: object.UserReply,
	}
	return nil
}

// readRecordUpdate turns the arguments of a task call into the update the
// record takes, and says what to write instead when it can read nothing.
func readRecordUpdate(arguments json.RawMessage) (record.Update, error) {
	written := recordWrite{}
	if err := json.Unmarshal(arguments, &written); err != nil {
		return record.Update{}, fmt.Errorf("the arguments of the task tool do not read as an object: %w. %s",
			err, whatTheTaskToolTakes())
	}
	update := record.Update{
		Why:      written.Why,
		StopWhen: written.StopWhen,
		Plan:     written.Plan,
	}
	for _, line := range written.DoneWhen {
		update.DoneWhen = append(update.DoneWhen, line.DoneLine)
	}
	for _, task := range written.Tasks {
		update.Tasks = append(update.Tasks, record.NewJobTask{TaskID: task.TaskID, Text: task.Text, DueAt: task.DueAt})
	}
	if written.Decision != nil {
		update.Decision = &record.NewDecision{Text: written.Decision.Text, Reason: written.Decision.Reason}
	}
	if written.Failure != nil {
		update.Failure = &record.NewFailure{Text: written.Failure.Text, Cause: written.Failure.Cause}
	}
	if nothingWritten(update) {
		return record.Update{}, errors.New("that record write says nothing this record can hold. " + whatTheTaskToolTakes())
	}
	return update, nil
}

// nothingWritten says whether an update would change nothing at all.
func nothingWritten(update record.Update) bool {
	return update.Why == "" && update.DoneWhen == nil && update.StopWhen == nil &&
		update.Plan == nil && update.Tasks == nil && update.Decision == nil && update.Failure == nil
}

// whatTheTaskToolTakes names the fields of the task tool, so that a model whose
// write was refused is told what to write instead.
func whatTheTaskToolTakes() string {
	return `Write one object with any of: "why", "doneWhen", "stopWhen", "plan", "tasks", "decision" (with a reason), "failure" (with a cause).`
}

// fieldsWritten names the parts of the record one write touched, in order, for
// the one line the record keeps about it.
func fieldsWritten(arguments string) string {
	written := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(arguments), &written); err != nil {
		return "one change"
	}
	names := slices.Sorted(maps.Keys(written))
	if len(names) == 0 {
		return "nothing"
	}
	return strings.Join(names, ", ")
}
