// Writing down what a finished task did without spending a single model token
// is the design behind Hermes' memory tool at
// ~/Code/hermes-agent/tools/memory_tool.py, where the model has to ask for
// every entry, turned around: here the harness writes what ordinary code can
// verify, and the model is only asked for the one lesson the review draws. What
// the model would have had to notice by itself is read straight out of the
// event log instead.

package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/JaredTate/coeus/internal/contract"
)

// MaxCapturedFacts is how many facts one finished task may leave behind, so
// that a task with a thousand tool calls cannot fill the memory files on its
// own.
const MaxCapturedFacts = 200

// maxCapturePages is how many reads of the event log one capture may make. A
// list read of the log returns at most log.MaxEventsPerRead events, so this is
// far more of a task than any task ever writes, and it is here so that a log
// that keeps saying there is more can never hold a capture open for ever.
const maxCapturePages = 100

// correctionWords are the words that turn a message from the user into a
// correction, which is kept word for word. They are the five from design
// section 10, with the apostrophe-less spelling of the last one accepted too.
var correctionWords = []string{"no", "actually", "always", "never", "don't", "dont"}

// messageBody is what this package reads out of a message event. The loop
// writes the event, so the field names are read in either the plain form or the
// capitalised one, which is what the JSON reader does by itself.
type messageBody struct {
	// Text is what was said.
	Text string `json:"text"`
	// Role says who said it, and only a message the model wrote is passed over.
	Role string `json:"role"`
}

// toolCallBody is what this package reads out of a tool-call event: the tool's
// name and the arguments the model wrote, under either of the two names the
// arguments go by.
type toolCallBody struct {
	// Name is the tool the model asked for.
	Name string `json:"name"`
	// Input is the arguments as contract.ToolCall carries them.
	Input json.RawMessage `json:"input"`
	// Arguments is the arguments under the name a text-shaped call uses.
	Arguments json.RawMessage `json:"arguments"`
}

// field reads one argument of a tool call as text, and reads anything it cannot
// find as empty.
func (call toolCallBody) field(name string) string {
	written := call.Input
	if len(written) == 0 {
		written = call.Arguments
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(written, &fields); err != nil {
		return ""
	}
	value := ""
	if err := json.Unmarshal(fields[name], &value); err != nil {
		return ""
	}
	return oneLine(value)
}

// Capture writes down what a finished task did, with no model call at all: the
// files it changed, the commands it ran, the sites it visited, the jobs it
// created, and every message from the user that begins with one of the
// correction words, kept word for word. Running it twice over the same task
// changes nothing, because every fact it writes has an id built from the task
// and the event it came from.
func (memory *Memory) Capture(ctx context.Context, taskID string) error {
	if taskID == "" {
		return errors.New("cannot capture a task with no id, so pass the id of the task that finished")
	}
	captured, err := memory.capturedFactsOfTask(ctx, taskID)
	if err != nil {
		return err
	}
	fresh, err := memory.factsNotSavedYet(ctx, captured)
	if err != nil {
		return err
	}
	if len(fresh) == 0 {
		return nil
	}
	return memory.Save(ctx, fresh)
}

// capturedFactsOfTask reads the whole of a finished task out of the event log
// and turns it into the facts worth keeping. A task with more events than one
// read of the log returns comes back cut short, and the rest of it is read in
// pages with ByRange, because a capture that took the first page for the whole
// task would quietly forget everything a long task did after it. It stops at the
// end of the log, at the cap on captured facts, or at the cap on pages.
func (memory *Memory) capturedFactsOfTask(ctx context.Context, taskID string) ([]contract.Fact, error) {
	facts := []contract.Fact{}
	events, err := memory.eventLog.ByTask(ctx, taskID)
	for page := 1; ; page++ {
		if err != nil && !readWasCutShort(events, err) {
			return nil, fmt.Errorf("cannot read the events of task %q to capture what it did: %w", taskID, err)
		}
		facts = appendCapturedFacts(facts, taskID, events)
		if err == nil || len(facts) >= MaxCapturedFacts || page >= maxCapturePages {
			return facts, nil
		}
		after := events[len(events)-1].Sequence
		events, err = memory.eventLog.ByRange(ctx, contract.EventRange{From: after + 1, To: math.MaxInt64})
	}
}

// readWasCutShort says whether a read of the event log stopped at the log's own
// cap rather than failing. internal/log hands back the events it read together
// with an error telling the caller to read the rest in pages with ByRange, and
// the name in that message is what tells the two apart, because contract.Store
// has no other way of saying it. A read that really failed hands back nothing.
func readWasCutShort(events []contract.Event, err error) bool {
	return err != nil && len(events) > 0 && strings.Contains(err.Error(), "ByRange")
}

// appendCapturedFacts adds the facts worth keeping from one page of the event
// log, in the order they happened, and stops at the cap on captured facts. A
// page read by ByRange holds every task's events, so the ones belonging to some
// other task are passed over here.
func appendCapturedFacts(facts []contract.Fact, taskID string, events []contract.Event) []contract.Fact {
	source := "task " + taskID
	for _, event := range events {
		if len(facts) >= MaxCapturedFacts {
			return facts
		}
		if event.TaskID != taskID {
			continue
		}
		text, aboutTheUser, worthKeeping := capturedFrom(event)
		if !worthKeeping {
			continue
		}
		facts = append(facts, contract.Fact{
			ID:       capturedFactID(taskID, event.Sequence, aboutTheUser),
			Text:     cutToBytes(text, MaxFactTextBytes),
			Source:   source,
			Recorded: event.Occurred,
		})
	}
	return facts
}

// capturedFrom reads one event and says what, if anything, is worth writing
// down about it, and whether what it says is a fact about the user.
func capturedFrom(event contract.Event) (text string, aboutTheUser bool, worthKeeping bool) {
	switch event.Kind {
	case contract.EventFileChange:
		body := contract.FileChangeBody{}
		if err := json.Unmarshal(event.Body, &body); err != nil || body.Path == "" {
			return "", false, false
		}
		return "changed the file " + oneLine(body.Path), false, true
	case contract.EventMessage:
		return capturedFromMessage(event)
	case contract.EventToolCall:
		return capturedFromToolCall(event)
	default:
		return "", false, false
	}
}

// capturedFromMessage keeps a message from the user word for word when it
// begins with one of the correction words, and passes over everything else.
func capturedFromMessage(event contract.Event) (text string, aboutTheUser bool, worthKeeping bool) {
	body := messageBody{}
	if err := json.Unmarshal(event.Body, &body); err != nil {
		return "", false, false
	}
	if strings.EqualFold(body.Role, string(contract.RoleAssistant)) {
		return "", false, false
	}
	said := oneLine(body.Text)
	if !slices.Contains(correctionWords, firstWord(said)) {
		return "", false, false
	}
	return said, true, true
}

// capturedFromToolCall writes down the commands the task ran, the sites it
// visited, and the jobs it created, reading the argument that says what the
// call would do.
func capturedFromToolCall(event contract.Event) (text string, aboutTheUser bool, worthKeeping bool) {
	call := toolCallBody{}
	if err := json.Unmarshal(event.Body, &call); err != nil {
		return "", false, false
	}
	written := ""
	switch call.Name {
	case contract.ToolShell:
		written = describeIfWritten("ran the command ", call.field("command"))
	case contract.ToolWeb, contract.ToolBrowserOpen:
		written = describeIfWritten("visited the site ", call.field("url"))
	case contract.ToolJob:
		written = describeIfWritten("created the job ", call.field("name"))
	}
	if written == "" {
		return "", false, false
	}
	return written, false, true
}

// describeIfWritten joins an opening phrase to an argument, and says nothing at
// all when the model wrote no argument.
func describeIfWritten(opening string, written string) string {
	if written == "" {
		return ""
	}
	return opening + written
}

// firstWord returns the first word of a message in lower case, which is what
// decides whether the message is a correction. An apostrophe counts as part of
// a word, so that "don't" is one word and not two.
func firstWord(text string) string {
	plain := strings.ToLower(strings.ReplaceAll(text, "’", "'"))
	plain = strings.TrimLeftFunc(plain, func(letter rune) bool { return !unicode.IsLetter(letter) })
	end := strings.IndexFunc(plain, func(letter rune) bool {
		return !unicode.IsLetter(letter) && letter != '\''
	})
	if end < 0 {
		return plain
	}
	return plain[:end]
}

// capturedFactID builds the id of a captured fact out of the task and the event
// it came from, so that capturing the same task twice writes nothing new.
func capturedFactID(taskID string, sequence int64, aboutTheUser bool) string {
	prefix := "c"
	if aboutTheUser {
		prefix = UserFactPrefix + "c"
	}
	tail := "-" + strconv.FormatInt(sequence, 10)
	name := onlyIDLetters(taskID)
	room := maxFactIDRunes - len(prefix) - len(tail)
	if room < 0 {
		room = 0
	}
	if len(name) > room {
		name = name[:room]
	}
	return prefix + name + tail
}

// onlyIDLetters keeps the letters of a task id that a fact line can hold.
func onlyIDLetters(taskID string) string {
	return strings.Map(func(letter rune) rune {
		if validFactID(string(letter)) {
			return letter
		}
		return -1
	}, taskID)
}

// factsNotSavedYet keeps the facts memory does not already hold, which is what
// makes a second capture of the same task change nothing.
func (memory *Memory) factsNotSavedYet(ctx context.Context, facts []contract.Fact) ([]contract.Fact, error) {
	fresh := []contract.Fact{}
	for _, fact := range facts {
		there, err := factIsThere(ctx, memory.database, fact.ID)
		if err != nil {
			return nil, err
		}
		if !there {
			fresh = append(fresh, fact)
		}
	}
	return fresh, nil
}

// cutToBytes shortens text to a number of bytes on a rune boundary, saying that
// the rest is in the event log, because the log keeps every message in full.
func cutToBytes(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	note := " (cut here; the whole of it is in the event log)"
	room := limit - len(note)
	if room < 0 {
		room = 0
	}
	cut := text[:room]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + note
}
