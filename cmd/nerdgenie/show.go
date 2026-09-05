package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// maxShownBytes is the most text one show answers with. A screen opens a result
// to read it, and a result past this is a file rather than a reading; the
// socket line it rides on is built to hold about as much.
const maxShownBytes = 64 * 1024

// theLeftOutNoteRoom is the room kept at the end of a cut text for the line
// that says how much was left out, so that the whole answer stays inside the
// cap.
const theLeftOutNoteRoom = 96

// maxFieldLettersInAnError is how much of a field's value an error about a
// show reads back, so that a wrong ask is named without being repeated whole.
const maxFieldLettersInAnError = 40

// The names of the fields a show carries, which the contract's SocketShow
// comment lays down.
const (
	showFieldID   = "id"
	showFieldTask = "task"
	showFieldJob  = "job"
)

// showing answers a screen's show straight out of the log and the job store,
// with no model call: the full text of a result with the call that made it, or
// a task's or a job's record as the record package prints it.
type showing struct {
	// store is the event log the records and the results are read from.
	store contract.Store
	// jobs is the job store a job's record is loaded from, or nil when none is
	// open.
	jobs contract.Job
}

// theShowAnswerer is what answers a screen's show, over the agent's event log
// and its job store. The job store is looked at before it goes into the
// interface, because a nil pointer inside an interface is not a nil interface.
func (running *agent) theShowAnswerer() *showing {
	answering := &showing{store: running.events}
	if running.jobs != nil {
		answering.jobs = running.jobs
	}
	return answering
}

// answer reads the request by its fields, which take one of three shapes, and
// answers with a shown, or with an error that names what was asked and says
// what to ask instead.
func (answering *showing) answer(ctx context.Context, fields map[string]string) (contract.SocketEnvelope, error) {
	if answering.store == nil {
		return contract.SocketEnvelope{}, errors.New("there is no event log open, so nothing can be shown until the agent has started")
	}
	id := strings.TrimSpace(fields[showFieldID])
	task := strings.TrimSpace(fields[showFieldTask])
	job := strings.TrimSpace(fields[showFieldJob])
	// A screen that attached after the task ended has pills that carry no
	// task, because the status stopped naming one; its results belong to the
	// latest task, so that is the task the show is read against.
	if id != "" && task == "" && job == "" {
		if latest, there := latestTaskNumber(ctx, answering.store); there {
			task = latest
		}
	}
	switch {
	case id != "" && task != "" && job == "":
		return answering.showResult(ctx, id, task)
	case id == "" && task != "" && job == "":
		return answering.showTask(ctx, task)
	case id == "" && task == "" && job != "":
		return answering.showJob(ctx, job)
	default:
		return contract.SocketEnvelope{}, fmt.Errorf(
			"a show names a result with the fields id and task, a task with the field task alone, or a job with the field job alone, and this one carried %s, so ask in one of those three shapes",
			fieldsAsWords(fields))
	}
}

// showResult answers a result: the call that made it, laid out for reading,
// and then the whole of the stored text.
func (answering *showing) showResult(ctx context.Context, id string, task string) (contract.SocketEnvelope, error) {
	keeper, err := answering.loadTask(ctx, task)
	if err != nil {
		return contract.SocketEnvelope{}, err
	}
	text, err := keeper.Read(ctx, id)
	if err != nil {
		return contract.SocketEnvelope{}, fmt.Errorf(
			"the result %s of task %s cannot be read, so ask for one of the results the record lists: %w", id, task, err)
	}
	call, found := answering.callThatMade(ctx, keeper.LogKey(), id)
	return shown(callAndResult(call, found, id, text), map[string]string{showFieldID: id, showFieldTask: task}), nil
}

// showTask answers a task with its record as the record package prints it.
func (answering *showing) showTask(ctx context.Context, task string) (contract.SocketEnvelope, error) {
	keeper, err := answering.loadTask(ctx, task)
	if err != nil {
		return contract.SocketEnvelope{}, err
	}
	return shown(withinTheShownCap(string(record.Print(keeper.Record()))), map[string]string{showFieldTask: task}), nil
}

// showJob answers a job with its record as the job store loads it and the
// record package prints it.
func (answering *showing) showJob(ctx context.Context, job string) (contract.SocketEnvelope, error) {
	if answering.jobs == nil {
		return contract.SocketEnvelope{}, fmt.Errorf(
			"there is no job store open, so job %s cannot be shown; ask for a task instead", job)
	}
	held, err := answering.jobs.Load(ctx, job)
	if err != nil {
		return contract.SocketEnvelope{}, fmt.Errorf(
			"job %s cannot be loaded, so ask for one the jobs command lists: %w", job, err)
	}
	return shown(withinTheShownCap(string(record.Print(held))), map[string]string{showFieldJob: job}), nil
}

// loadTask reloads one task's keeper from its latest checkpoint, with an error
// that names the task and says what to ask instead when there is no such task.
func (answering *showing) loadTask(ctx context.Context, task string) (*record.Keeper, error) {
	keeper, err := record.Load(ctx, answering.store, contract.RecordTask, task)
	if err != nil {
		return nil, fmt.Errorf(
			"there is no task numbered %s to show, so ask for one the tasks command lists: %w", task, err)
	}
	return keeper, nil
}

// callThatMade finds the tool call written just before the result stored under
// the id, which is the call that made it, because the loop writes each call
// before it runs and stores its result before the next call. A call is used up
// by the first result after it, so a result no call made, such as the reply the
// harness writes as a result of its own, comes back false rather than with the
// call before it. A result stored twice, which a failed checkpoint can do,
// keeps the call its first copy had.
func (answering *showing) callThatMade(ctx context.Context, logKey string, id string) (contract.ToolCall, bool) {
	events, err := answering.store.ByTask(ctx, logKey)
	if err != nil {
		return contract.ToolCall{}, false
	}
	var lastCall, made contract.ToolCall
	callWaiting, found := false, false
	for _, event := range events {
		switch event.Kind {
		case contract.EventToolCall:
			call := contract.ToolCall{}
			if json.Unmarshal(event.Body, &call) == nil {
				lastCall, callWaiting = call, true
			}
		case contract.EventToolResult:
			stored := record.StoredResult{}
			if json.Unmarshal(event.Body, &stored) == nil && stored.ID == id && (callWaiting || !found) {
				made, found = lastCall, callWaiting
			}
			callWaiting = false
		}
	}
	return made, found
}

// callAndResult lays a result out for reading: the call's name on the first
// line, its input as indented JSON on the lines after, a blank line, the word
// result, and then the stored text, all kept within the cap.
func callAndResult(call contract.ToolCall, found bool, id string, text string) string {
	lines := []string{}
	if found {
		lines = append(lines, "call: "+call.Name)
		if input := prettyJSON(call.Input); input != "" {
			lines = append(lines, input)
		}
	} else {
		lines = append(lines, "call: no call was recorded for "+id)
	}
	lines = append(lines, "", "result:", text)
	return withinTheShownCap(strings.Join(lines, "\n"))
}

// prettyJSON lays a call's input out with one field per line, and hands back
// the input as it was when it is not JSON the layout can read.
func prettyJSON(input json.RawMessage) string {
	trimmed := bytes.TrimSpace(input)
	if len(trimmed) == 0 {
		return ""
	}
	laidOut := bytes.Buffer{}
	if err := json.Indent(&laidOut, trimmed, "", "  "); err != nil {
		return string(trimmed)
	}
	return laidOut.String()
}

// withinTheShownCap cuts a text to the cap and ends it with a line saying how
// much was left out, never cutting a letter in half; a text inside the cap
// comes back whole.
func withinTheShownCap(text string) string {
	if len(text) <= maxShownBytes {
		return text
	}
	kept := maxShownBytes - theLeftOutNoteRoom
	for kept > 0 && !utf8.RuneStart(text[kept]) {
		kept--
	}
	return text[:kept] + fmt.Sprintf(
		"\n... %d of %d bytes were left out, and the whole text is in the event log", len(text)-kept, len(text))
}

// shown is the answer to a show: the text and the fields it was asked with.
func shown(text string, fields map[string]string) contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketShown, Text: text, Fields: fields}
}

// fieldsAsWords names a request's fields for an error, in name order, with a
// long value cut short, and says "no fields" when there were none.
func fieldsAsWords(fields map[string]string) string {
	if len(fields) == 0 {
		return "no fields"
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	words := make([]string, 0, len(names))
	for _, name := range names {
		value := []rune(fields[name])
		if len(value) > maxFieldLettersInAnError {
			value = append(value[:maxFieldLettersInAnError], []rune("...")...)
		}
		words = append(words, name+"="+string(value))
	}
	return "the fields " + strings.Join(words, ", ")
}
