package task

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/JaredTate/coeus/internal/contract"
)

// writtenCall is one call to this tool as the model wrote it. Every field is held
// exactly as it arrived and read one at a time, so that a field written in a
// shape this tool cannot read is refused by name instead of killing the whole
// call, and so that both names a model gives the same field are taken.
type writtenCall struct {
	// Operation says which of the seven this call is, and may be left out.
	Operation string `json:"operation"`
	// Why is the one line on why the user wants this.
	Why json.RawMessage `json:"why"`
	// DoneWhen is the whole done list.
	DoneWhen json.RawMessage `json:"done_when"`
	// DoneWhenOneWord is the same list under the name the forty-step fixture
	// and the loop's own reader use.
	DoneWhenOneWord json.RawMessage `json:"doneWhen"`
	// StopWhen is the whole stop list.
	StopWhen json.RawMessage `json:"stop_when"`
	// StopWhenOneWord is the same list under the fixture's name.
	StopWhenOneWord json.RawMessage `json:"stopWhen"`
	// Plan is the whole plan, one line per step.
	Plan json.RawMessage `json:"plan"`
	// Text is the choice, or the thing that went wrong.
	Text json.RawMessage `json:"text"`
	// Reason is why a choice was made.
	Reason json.RawMessage `json:"reason"`
	// Cause is why something went wrong.
	Cause json.RawMessage `json:"cause"`
	// Line is which done line to pin a result to, counting from one.
	Line json.RawMessage `json:"line"`
	// Result is the result to pin, such as r7.
	Result json.RawMessage `json:"result"`
	// ResultOneWord is the same label under the name the fixture uses.
	ResultOneWord json.RawMessage `json:"resultId"`
	// Decision is one choice with its reason, written as an object.
	Decision json.RawMessage `json:"decision"`
	// Failure is one thing that went wrong with its cause, written as an object.
	Failure json.RawMessage `json:"failure"`
}

// writtenPair is a decision or a failure as the model writes it: what it is,
// and the reason or the cause it must carry.
type writtenPair struct {
	// Text is the choice, or what went wrong.
	Text string `json:"text"`
	// Reason is why a decision was made.
	Reason string `json:"reason"`
	// Cause is why a failure happened.
	Cause string `json:"cause"`
}

// writtenDoneLine is one line of the done list as the model writes it.
type writtenDoneLine struct {
	// Text is the line itself.
	Text string `json:"text"`
	// Done says the model believes the line is satisfied.
	Done bool `json:"done"`
	// Result is the result that proves it, such as r7.
	Result string `json:"result"`
	// ResultOneWord is the same label under the name the fixture and the loop's
	// own reader use.
	ResultOneWord string `json:"resultId"`
	// UserReply is the user's own words standing in for a result.
	UserReply string `json:"user_reply"`
	// UserReplyOneWord is the same words under the fixture's name.
	UserReplyOneWord string `json:"userReply"`
}

// theShapeOfADoneLine is what a done line may be written as. It is said once
// because three refusals share it.
const theShapeOfADoneLine = `a list of lines, each the line itself as a string or an object with "text" ` +
	`(and "done" with the "result" that proves it)`

// UnmarshalJSON takes a done line written either as an object or as the bare
// line in a string, because a model writes the list as strings more often than
// not, and a refusal there stalls the whole task. Anything else is refused with
// the shape named.
func (line *writtenDoneLine) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		return json.Unmarshal(trimmed, &line.Text)
	}
	type plainDoneLine writtenDoneLine
	var held struct {
		plainDoneLine
		// Line, Item and Description are the other names a model gives the text.
		Line        string `json:"line"`
		Item        string `json:"item"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(trimmed, &held); err != nil {
		return errors.New(theShapeOfADoneLine)
	}
	*line = writtenDoneLine(held.plainDoneLine)
	for _, other := range []string{held.Line, held.Item, held.Description} {
		if line.Text == "" {
			line.Text = other
		}
	}
	return nil
}

// theLine is the done line the record takes, whichever of the two names the
// model gave the result and the user's reply.
func (line writtenDoneLine) theLine() contract.DoneLine {
	return contract.DoneLine{
		Text:      line.Text,
		Done:      line.Done,
		ResultID:  eitherName(line.Result, line.ResultOneWord),
		UserReply: eitherName(line.UserReply, line.UserReplyOneWord),
	}
}

// eitherName takes whichever of the two names for one field the model used.
func eitherName(first string, second string) string {
	if first != "" {
		return first
	}
	return second
}

// eitherRaw takes whichever of the two names for one field the model wrote
// something in.
func eitherRaw(first json.RawMessage, second json.RawMessage) json.RawMessage {
	if !nothingWritten(first) {
		return first
	}
	return second
}

// aboutTheField is the refusal for one field written in a shape this tool
// cannot read. The field is named first, so that the model rewrites that field
// rather than guessing at the whole call.
func aboutTheField(field string, shape string) error {
	return fmt.Errorf("this call's %q is written in a shape this tool cannot read, so write it as %s", field, shape)
}

// nothingWritten says whether the model left a field out or wrote null in it.
func nothingWritten(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || string(trimmed) == "null"
}

// readText reads one piece of text the model wrote: a string, the first line of
// a list of them, or the text inside an object. A model writes all three, and
// the line it meant is plainly there in each, so refusing any of them costs a
// round for nothing.
func readText(raw json.RawMessage, field string) (string, error) {
	if nothingWritten(raw) {
		return "", nil
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return one, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		if len(many) == 0 {
			return "", nil
		}
		return many[0], nil
	}
	held := writtenPair{}
	if err := json.Unmarshal(raw, &held); err == nil && held.Text != "" {
		return held.Text, nil
	}
	return "", aboutTheField(field, `one line of text, a list whose first line is it, or an object with "text" in it`)
}

// readLines reads a list the model wrote as a JSON list of strings or as one
// string, which happens often enough that refusing it stalls the task.
func readLines(raw json.RawMessage, field string) ([]string, error) {
	if nothingWritten(raw) {
		return nil, nil
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return []string{one}, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return nil, aboutTheField(field, "a JSON list of strings, one line each, or one string")
	}
	return many, nil
}

// readDoneLines reads the done list, which a model writes as a list of lines or
// as one line on its own.
func readDoneLines(raw json.RawMessage, field string) ([]contract.DoneLine, error) {
	if nothingWritten(raw) {
		return nil, nil
	}
	trimmed := bytes.TrimSpace(raw)
	held := []writtenDoneLine{}
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &held); err != nil {
			return nil, aboutTheField(field, theShapeOfADoneLine)
		}
	} else {
		one := writtenDoneLine{}
		if err := one.UnmarshalJSON(trimmed); err != nil {
			return nil, aboutTheField(field, theShapeOfADoneLine)
		}
		held = append(held, one)
	}
	lines := make([]contract.DoneLine, 0, len(held))
	for _, line := range held {
		lines = append(lines, line.theLine())
	}
	return lines, nil
}

// readNumber reads a whole number the model may have written as a number or as
// a string holding one.
func readNumber(raw json.RawMessage, field string) (int, error) {
	if nothingWritten(raw) {
		return 0, nil
	}
	trimmed := bytes.Trim(bytes.TrimSpace(raw), `"`)
	if len(trimmed) == 0 {
		return 0, nil
	}
	number, err := strconv.Atoi(string(trimmed))
	if err != nil {
		return 0, aboutTheField(field, "a whole number, counting from one")
	}
	return number, nil
}

// readPair reads a decision or a failure written as an object, which is how the
// forty-step fixture and the loop's own reader write both, or as the text on
// its own.
func readPair(raw json.RawMessage, field string) (*writtenPair, error) {
	if nothingWritten(raw) {
		return nil, nil
	}
	held := writtenPair{}
	if err := json.Unmarshal(raw, &held); err == nil {
		return &held, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return &writtenPair{Text: text}, nil
	}
	return nil, aboutTheField(field, `an object with "text" and the reason or the cause behind it`)
}
