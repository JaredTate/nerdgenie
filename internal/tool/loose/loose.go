package loose

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// MaxNamesShown is how many of the keys a call did carry are named in a refusal.
// A model that wrote forty of them has lost its way, and a message nobody reads
// is a message that does not help.
const MaxNamesShown = 8

// MaxShownRunes is how much of one key or one value a refusal shows, because
// both are text from outside and can be any length at all.
const MaxShownRunes = 40

// MaxNesting is how deep this reader looks inside a list or an object for the
// text of a field, because a model can nest one inside another forever.
const MaxNesting = 3

// Fields is one call's arguments as the model wrote them. A tool reads each
// field it knows by name, asks Wrong whether any of them was written as
// something the tool cannot use, and asks Missing for the refusal when a field
// it needs is not there at all.
type Fields struct {
	written map[string]json.RawMessage
	wrong   []string
}

// Read parses one call's arguments. What is written in place of an object is
// refused with the same line every tool used to write for itself, where what
// names the fields this tool wants, such as "a path and content".
func Read(written json.RawMessage, what string) (*Fields, error) {
	fields := &Fields{written: map[string]json.RawMessage{}}
	if len(strings.TrimSpace(string(written))) == 0 {
		return fields, nil
	}
	if err := json.Unmarshal(written, &fields.written); err != nil {
		return nil, fmt.Errorf("cannot read this call's arguments as JSON, so write an object with %s in it: %w", what, err)
	}
	return fields, nil
}

// Text is what the model wrote under the first of these names that the call
// carries, read from a string, a number, a list of strings, or an object with
// text in it. The second answer says whether any of the names was written at
// all; a field written as something this reader cannot make text of is not
// found, and Wrong then says so by name.
func (fields *Fields) Text(names ...string) (string, bool) {
	raw, name, found := fields.value(names)
	if !found {
		return "", false
	}
	text, readable := textOf(raw, MaxNesting)
	if !readable {
		fields.complain(name, "text", raw)
		return "", false
	}
	return text, true
}

// Number is the whole number the model wrote under the first of these names the
// call carries, whether it wrote it as a number or in quotes.
func (fields *Fields) Number(names ...string) (int, bool) {
	raw, name, found := fields.value(names)
	if !found {
		return 0, false
	}
	text, readable := textOf(raw, MaxNesting)
	if readable {
		if number, isNumber := wholeNumber(text); isNumber {
			return number, true
		}
	}
	fields.complain(name, "a whole number", raw)
	return 0, false
}

// Mark is the number of a control the model wrote under the first of these names
// the call carries. A model that has just read a browser page writes the page's
// own name for an element, such as e5, so a leading letter or a hash is read off
// before the number.
func (fields *Fields) Mark(names ...string) (int, bool) {
	raw, name, found := fields.value(names)
	if !found {
		return 0, false
	}
	text, readable := textOf(raw, MaxNesting)
	if readable {
		if number, isNumber := wholeNumber(strings.TrimLeft(strings.TrimSpace(text), "eE#")); isNumber {
			return number, true
		}
	}
	fields.complain(name, "the number of a control, such as 5", raw)
	return 0, false
}

// Flag is the true or false the model wrote under the first of these names the
// call carries, whether it wrote it as a JSON true, as the word "true", as
// "yes", or as one.
func (fields *Fields) Flag(names ...string) (bool, bool) {
	raw, name, found := fields.value(names)
	if !found {
		return false, false
	}
	text, readable := textOf(raw, MaxNesting)
	if readable {
		switch Action(text) {
		case "true", "yes", "y", "on", "1":
			return true, true
		case "false", "no", "n", "off", "0":
			return false, true
		}
	}
	fields.complain(name, "true or false", raw)
	return false, false
}

// Wrong is the refusal for every field this call wrote as something the tool
// cannot use, named one by one, or nothing when every field read cleanly. A tool
// asks it once, after reading its fields and before refusing a missing one.
func (fields *Fields) Wrong() error {
	if len(fields.wrong) == 0 {
		return nil
	}
	return errors.New(strings.Join(fields.wrong, "; "))
}

// Missing is the refusal for a field the tool needs and the call does not carry.
// It names the field, it names the key the model wrote instead when one of them
// looks like a wrong guess at it, and it ends with the field's exact name, so
// that a model reading the refusal can write the call again and get it right.
// What says what belongs in the field, such as "the text to replace".
func (fields *Fields) Missing(name string, what string) error {
	if near, found := fields.nearest(name); found {
		return fmt.Errorf("this call has no %q; you wrote %q, so write %s under %q", name, near, what, name)
	}
	if wrote := fields.namesWritten(); wrote != "" {
		return fmt.Errorf("this call has no %q, and what it does write is %s, so write %s under %q", name, wrote, what, name)
	}
	return fmt.Errorf("this call has no %q, so write %s under %q", name, what, name)
}

// Action folds the name of an action or a method the way a model may have
// written it: without the spaces around it, in lower case, and with dashes and
// spaces read as underscores, so that CLICK, Click and set-clipboard each name
// the action they plainly mean.
func Action(written string) string {
	folded := strings.ToLower(strings.TrimSpace(written))
	folded = strings.ReplaceAll(folded, "-", "_")
	return strings.ReplaceAll(folded, " ", "_")
}

// value is what the call wrote under the first of these names it carries, and
// the name it was written under. A name is matched exactly first and then
// folded, so that Path and file-path find the field as well.
func (fields *Fields) value(names []string) (json.RawMessage, string, bool) {
	for _, name := range names {
		if raw, held := fields.written[name]; held && !isNothing(raw) {
			return raw, name, true
		}
	}
	for _, name := range names {
		for _, key := range fields.keys() {
			raw := fields.written[key]
			if Action(key) == Action(name) && !isNothing(raw) {
				return raw, key, true
			}
		}
	}
	return nil, "", false
}

// keys are the names the call carries, in order, so that every refusal built
// from them reads the same way twice.
func (fields *Fields) keys() []string {
	keys := make([]string, 0, len(fields.written))
	for key := range fields.written {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// complain keeps one field-level refusal for Wrong to hand back, so that one
// badly written field does not take the fields around it with it.
func (fields *Fields) complain(name string, wanted string, raw json.RawMessage) {
	fields.wrong = append(fields.wrong, fmt.Sprintf("the field %q holds %s, and this tool needs %s there, so write %s under %q",
		name, cutRunes(string(raw), MaxShownRunes), wanted, wanted, name))
}

// nearest is the key the call wrote that looks like a wrong guess at the name,
// which is one that holds the name or is held by it once both are folded.
func (fields *Fields) nearest(name string) (string, bool) {
	wanted := Action(name)
	for _, key := range fields.keys() {
		folded := Action(key)
		if folded == wanted {
			continue
		}
		if strings.Contains(folded, wanted) || strings.Contains(wanted, folded) {
			return key, true
		}
	}
	return "", false
}

// namesWritten is the list of keys the call did carry, for a refusal, cut to the
// first few so that a message stays a message.
func (fields *Fields) namesWritten() string {
	keys := fields.keys()
	if len(keys) == 0 {
		return ""
	}
	shown := make([]string, 0, MaxNamesShown)
	for at, key := range keys {
		if at >= MaxNamesShown {
			shown = append(shown, fmt.Sprintf("and %d more", len(keys)-MaxNamesShown))
			break
		}
		shown = append(shown, strconv.Quote(cutRunes(key, MaxShownRunes)))
	}
	return strings.Join(shown, ", ")
}

// isNothing says whether the model wrote the field as null, which is the same as
// not writing it at all.
func isNothing(raw json.RawMessage) bool {
	return len(raw) == 0 || strings.TrimSpace(string(raw)) == "null"
}

// wholeNumber reads a whole number out of the text of a field, allowing the
// trailing zeros a model writes when it means a whole one.
func wholeNumber(text string) (int, bool) {
	trimmed := strings.TrimSpace(text)
	if number, err := strconv.Atoi(trimmed); err == nil {
		return number, true
	}
	if asFloat, err := strconv.ParseFloat(trimmed, 64); err == nil && asFloat == float64(int(asFloat)) {
		return int(asFloat), true
	}
	return 0, false
}

// cutRunes cuts a piece of text for a refusal, because both the keys and the
// values in a call come from outside and can be any length at all.
func cutRunes(text string, length int) string {
	letters := []rune(strings.Join(strings.Fields(text), " "))
	if len(letters) <= length {
		return string(letters)
	}
	return string(letters[:length]) + "..."
}
