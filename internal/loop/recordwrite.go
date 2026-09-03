package loop

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
)

// TheTaskToolSpec is what the model is told about the task tool when the
// registry holds none of its own. It is the same tool internal/tool builds, and
// the loop only describes and applies it while a registry is built without the
// record of the task running now.
var TheTaskToolSpec = contract.ToolSpec{
	Name: contract.ToolTask,
	Description: "Writes the task record: the why, the done list, the stop list, the plan, " +
		"a decision with its reason, or a failure with its cause.",
	Fields: []contract.ToolField{
		{Name: "why", Type: "string", Description: "The one line on why the user wants this, written once."},
		{Name: "done_when", Type: "array", Description: "The whole done list, at most five lines, each line with the result that proves it. " +
			"A line only your answer to the user can prove names \"reply\" as its result. More than five lines is a job."},
		{Name: "stop_when", Type: "array", Description: "The whole stop list, one line each."},
		{Name: "plan", Type: "array", Description: "The whole plan, one line per step, in order."},
		{Name: "decision", Type: "object", Description: "A choice, as a text and the reason it was made."},
		{Name: "failure", Type: "object", Description: "Something that went wrong, as a text and its cause."},
		{Name: "operation", Type: "string", Description: "One of stop_now, pin_evidence, unpin_evidence, or left out to write the record."},
		{Name: "text", Type: "string", Description: "With stop_now, the line of the stop list that has come true."},
		{Name: "result", Type: "string", Description: "With pin_evidence or unpin_evidence, the result to pin or unpin, such as r7."},
	},
	Classes: []contract.PermissionClass{contract.ClassWrite},
}

// recordWrite is what the model may write into the record, as it writes it in
// the arguments of a task call. Both ways of writing a name are read: the one
// the forty-step fixture uses and the one brief 2.5 fixed for the task tool, so
// that the loop and the tool understand the same calls. Anything else in the
// object is left alone, so that a model that adds a field of its own is not
// refused over it.
type recordWrite struct {
	// Operation is which of the task tool's operations this call is, and is
	// empty on a call that simply writes the sections it carries.
	Operation string `json:"operation"`
	// Why is the one line on why the user wants this.
	Why string `json:"why"`
	// DoneWhen is the whole done list.
	DoneWhen []doneLineWrite `json:"doneWhen"`
	// DoneWhenWritten is the same list under the task tool's own name.
	DoneWhenWritten []doneLineWrite `json:"done_when"`
	// StopWhen is the whole stop list.
	StopWhen []string `json:"stopWhen"`
	// StopWhenWritten is the same list under the task tool's own name.
	StopWhenWritten []string `json:"stop_when"`
	// Plan is a task's plan, one line per step.
	Plan []string `json:"plan"`
	// Tasks is a job's task list.
	Tasks []jobTaskWrite `json:"tasks"`
	// Decision is one choice with the reason it must carry.
	Decision *pairWrite `json:"decision"`
	// Failure is one thing that went wrong with the cause it must carry.
	Failure *pairWrite `json:"failure"`
	// Text is the choice, or the thing that went wrong, when the operation says
	// which of the two it is instead of the field name saying it.
	Text string `json:"text"`
	// Reason is why a choice was made.
	Reason string `json:"reason"`
	// Cause is why something went wrong.
	Cause string `json:"cause"`
	// Line is which done line to point at a result, counting from one.
	Line wholeNumber `json:"line"`
	// Result is the result to point that line at, such as "r7".
	Result string `json:"result"`
}

// wholeNumber is a number a model may write as a number or as a string holding
// one, which is how the shipping task tool reads the same field.
type wholeNumber int

// UnmarshalJSON takes a number or a quoted number, and nothing else.
func (number *wholeNumber) UnmarshalJSON(raw []byte) error {
	trimmed := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if trimmed == "" || trimmed == "null" {
		*number = 0
		return nil
	}
	read, err := strconv.Atoi(trimmed)
	if err != nil {
		return errors.New("a line number is written as a whole number, counting from one")
	}
	*number = wholeNumber(read)
	return nil
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
		Text string `json:"text"`
		Done bool   `json:"done"`
		// ResultID and Result are the two names for the result that proves the
		// line: the one the forty-step fixture writes and the one the shipping
		// task tool writes.
		ResultID string `json:"resultId"`
		Result   string `json:"result"`
		// UserReply and UserReplyWritten are the same two names for the user's
		// own words standing in for a result.
		UserReply        string `json:"userReply"`
		UserReplyWritten string `json:"user_reply"`
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return fmt.Errorf("a done line is a line of text, or an object with a text and the result that proves it: %w", err)
	}
	line.DoneLine = contract.DoneLine{
		Text: object.Text, Done: object.Done,
		ResultID:  firstWritten(object.ResultID, object.Result),
		UserReply: firstWritten(object.UserReply, object.UserReplyWritten),
	}
	return nil
}

// firstWritten takes whichever of two names for the same thing the model used.
func firstWritten(first string, second string) string {
	if first != "" {
		return first
	}
	return second
}

// readRecordUpdate turns the arguments of a task call into the update the
// record takes, and says what to write instead when it can read nothing. The
// record it is given is the record as it stands, which the operation that points
// a done line at its result writes back with that one line changed.
func readRecordUpdate(arguments json.RawMessage, held contract.Record) (record.Update, error) {
	written := recordWrite{}
	if err := json.Unmarshal(arguments, &written); err != nil {
		return record.Update{}, fmt.Errorf("the arguments of the task tool do not read as an object: %w. %s",
			err, whatTheTaskToolTakes())
	}
	switch written.Operation {
	case operationPinResult:
		return pinTheResultToItsLine(written, held)
	case operationDecision:
		written.Decision = &pairWrite{Text: written.Text, Reason: written.Reason}
	case operationFailure:
		written.Failure = &pairWrite{Text: written.Text, Cause: written.Cause}
	case operationWhy:
		written.Why = firstWritten(written.Why, written.Text)
	}
	update := record.Update{
		Why:      written.Why,
		StopWhen: eitherWay(written.StopWhen, written.StopWhenWritten),
		Plan:     written.Plan,
	}
	for _, line := range eitherWay(written.DoneWhen, written.DoneWhenWritten) {
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

// The operations of the shipping task tool that change what the fields of a
// call mean. The rest write the sections they carry, which is what a call with
// no operation at all does, so they need no name here.
const (
	// operationWhy sets the one line on why the user wants this, and may write
	// it in the text field rather than the why field.
	operationWhy = "why"
	// operationDecision adds one choice, with its text and reason written flat.
	operationDecision = "decision"
	// operationFailure adds one failure, with its text and cause written flat.
	operationFailure = "failure"
	// operationPinResult points one done line at the result that proves it.
	operationPinResult = "pin_result"
)

// pinTheResultToItsLine marks one done line proven and points it at the result
// that proves it, by writing the whole done list back with that one line
// changed, which is what the shipping task tool does with the same call.
func pinTheResultToItsLine(written recordWrite, held contract.Record) (record.Update, error) {
	lines := held.Goal.DoneWhen
	if written.Line < 1 || int(written.Line) > len(lines) {
		return record.Update{}, fmt.Errorf(
			"there is no done line numbered %d in this record, whose done list has %d lines in it, so number one it holds",
			written.Line, len(lines))
	}
	if strings.TrimSpace(written.Result) == "" {
		return record.Update{}, errors.New(
			"this call names no result, so give the label of the result in this record that proves the line, such as r7")
	}
	changed := slices.Clone(lines)
	changed[written.Line-1].Done = true
	changed[written.Line-1].ResultID = written.Result
	changed[written.Line-1].UserReply = ""
	return record.Update{DoneWhen: changed}, nil
}

// eitherWay takes whichever of the two ways of writing a list the model used.
func eitherWay[Item any](first []Item, second []Item) []Item {
	if len(first) > 0 {
		return first
	}
	return second
}

// nothingWritten says whether an update would change nothing at all.
func nothingWritten(update record.Update) bool {
	return update.Why == "" && update.DoneWhen == nil && update.StopWhen == nil &&
		update.Plan == nil && update.Tasks == nil && update.Decision == nil && update.Failure == nil
}

// whatTheTaskToolTakes names the fields of the task tool, so that a model whose
// write was refused is told what to write instead.
func whatTheTaskToolTakes() string {
	return `Write one object with any of: "why", "done_when", "stop_when", "plan", "tasks", "decision" (with a reason), "failure" (with a cause).`
}

// fieldsWritten names the parts of the record one write touched, in order, for
// the one line the record keeps about it.
//
// The answer it reads is what the task tool gave back, which is a sentence
// naming what was written with the whole update as JSON under it, so the reading
// starts at the first brace. An answer with no update under it at all is one
// change and nothing more can be said about it.
func fieldsWritten(answer string) string {
	if starts := strings.Index(answer, "{"); starts > 0 {
		answer = answer[starts:]
	}
	written := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(answer), &written); err != nil {
		return "one change"
	}
	names := slices.Sorted(maps.Keys(written))
	if len(names) == 0 {
		return "nothing"
	}
	return strings.Join(names, ", ")
}
