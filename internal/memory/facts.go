// One fact on one line, with where it came from and when it was written down,
// is the design of Hermes' memory entries at
// ~/Code/hermes-agent/tools/memory_tool.py, written fresh in Go. Hermes joins
// its entries with a separator character and lets an entry run over many lines;
// Nerd Genie keeps one fact to one line, so that a person reading MEMORY.md in a
// terminal sees one fact per row and so that a file can be read back a line at
// a time without a parser that can lose its place.

package memory

import (
	"strings"
	"time"
	"unicode"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// maxFactTextBytes is the longest one fact may be. Anything longer is not a
// fact but a document, and a document belongs in a note in the memory folder.
const maxFactTextBytes = 4000

// maxFactIDRunes is the longest a fact's id may be, so that the front of a fact
// line can never crowd out the fact itself.
const maxFactIDRunes = 64

// The pieces of one fact line. The line reads
// "- m7 [2026-09-02T14:00:00Z | task 17 | supersedes m3] the fact itself", and
// these are the marks that hold it together.
const (
	factLineStart      = "- "
	factLineFieldsOpen = " ["
	factLineFieldsShut = "] "
	factLineSeparator  = " | "
	factLineSupersedes = "supersedes "
)

// formatFactLine writes one fact as the single line that goes in a memory file.
// The date, the source, and the fact this one supersedes sit in brackets, and
// the fact itself is the rest of the line, so that the eye lands on the fact.
func formatFactLine(fact contract.Fact) string {
	fields := []string{
		fact.Recorded.UTC().Truncate(time.Second).Format(time.RFC3339),
		withoutSeparators(fact.Source),
	}
	if fact.Supersedes != "" {
		fields = append(fields, factLineSupersedes+withoutSeparators(fact.Supersedes))
	}
	return factLineStart + fact.ID + factLineFieldsOpen + strings.Join(fields, factLineSeparator) +
		factLineFieldsShut + oneLine(fact.Text)
}

// parseFactLine reads one line of a memory file back into a fact, and says no
// when the line is anything else, such as a heading somebody typed in by hand.
// It never fails: a line it cannot read is simply not a fact.
func parseFactLine(line string) (contract.Fact, bool) {
	rest, isFact := strings.CutPrefix(strings.TrimRight(line, "\r"), factLineStart)
	if !isFact {
		return contract.Fact{}, false
	}
	id, rest, found := strings.Cut(rest, " ")
	if !found || !validFactID(id) {
		return contract.Fact{}, false
	}
	inside, text, found := cutFields(rest)
	if !found {
		return contract.Fact{}, false
	}
	fact, readable := factFromFields(inside)
	if !readable {
		return contract.Fact{}, false
	}
	fact.ID = id
	fact.Text = oneLine(text)
	if fact.Text == "" {
		return contract.Fact{}, false
	}
	return fact, true
}

// cutFields splits the part of a line after the id into what is inside the
// brackets and the fact text after them.
func cutFields(rest string) (inside string, text string, found bool) {
	if !strings.HasPrefix(rest, "[") {
		return "", "", false
	}
	inside, text, found = strings.Cut(rest[1:], "]")
	if !found || !strings.HasPrefix(text, " ") {
		return "", "", false
	}
	return inside, text[1:], true
}

// factFromFields reads the two or three bracketed fields: the date, the source,
// and the optional id of the fact this one supersedes.
func factFromFields(inside string) (contract.Fact, bool) {
	fields := strings.Split(inside, factLineSeparator)
	if len(fields) < 2 || len(fields) > 3 {
		return contract.Fact{}, false
	}
	recorded, err := time.Parse(time.RFC3339, fields[0])
	if err != nil || !writableDate(recorded) {
		return contract.Fact{}, false
	}
	source := strings.TrimSpace(fields[1])
	if strings.ContainsAny(source, "|]") {
		return contract.Fact{}, false
	}
	fact := contract.Fact{
		Source:   withoutSeparators(source),
		Recorded: recorded.UTC().Truncate(time.Second),
	}
	if len(fields) == 3 {
		superseded, isSupersedes := strings.CutPrefix(fields[2], factLineSupersedes)
		if !isSupersedes || !validFactID(superseded) {
			return contract.Fact{}, false
		}
		fact.Supersedes = superseded
	}
	return fact, true
}

// writableDate says whether a moment can be written on a fact line and read
// back. The date form has no room for a year before one or after nine thousand
// nine hundred and ninety-nine, and a moment outside that range would be
// written down as something that cannot be read again.
func writableDate(moment time.Time) bool {
	year := moment.UTC().Year()
	return year >= 1 && year <= 9999
}

// validFactID says whether an id can be written on a fact line and read back:
// letters, digits, hyphens, underscores, and full stops, and nothing else, so
// that the id can never hold a space or one of the line's own marks.
func validFactID(id string) bool {
	runes := []rune(id)
	if len(runes) == 0 || len(runes) > maxFactIDRunes {
		return false
	}
	for _, letter := range runes {
		switch {
		case letter >= 'a' && letter <= 'z', letter >= 'A' && letter <= 'Z':
		case letter >= '0' && letter <= '9':
		case letter == '-', letter == '_', letter == '.':
		default:
			return false
		}
	}
	return true
}

// oneLine turns any text into a single line: every run of spaces, tabs, and
// line breaks becomes one space, and the ends are trimmed.
func oneLine(text string) string {
	return strings.Join(strings.FieldsFunc(text, unicode.IsSpace), " ")
}

// withoutSeparators takes the line's own marks out of a bracketed field, so
// that a source with a bar or a bracket in it cannot make the line unreadable.
func withoutSeparators(field string) string {
	replaced := strings.Map(func(letter rune) rune {
		if letter == '|' || letter == ']' || letter == '[' {
			return ' '
		}
		return letter
	}, field)
	return oneLine(replaced)
}
