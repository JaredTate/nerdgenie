package record

import (
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// A record holds one item per line, so any line break inside a piece of text is
// folded to a backslash and an "n" when it is printed and unfolded when it is
// read back. A backslash is doubled so that the folding can always be undone.
// Nothing else is touched, which is why the two examples in the design print
// back byte for byte.
const (
	// escapedBackslash is what one backslash becomes.
	escapedBackslash = `\\`
	// escapedNewline is what one line break becomes.
	escapedNewline = `\n`
	// escapedReturn is what one carriage return becomes.
	escapedReturn = `\r`
)

// foldText writes a piece of text so that it fits on one line of a record.
func foldText(text string) string {
	folded := strings.ReplaceAll(text, `\`, escapedBackslash)
	folded = strings.ReplaceAll(folded, "\n", escapedNewline)
	return strings.ReplaceAll(folded, "\r", escapedReturn)
}

// unfoldText undoes foldText. A backslash in front of anything else is kept,
// together with the character after it, so that folding and unfolding always
// agree with each other however odd the text is.
func unfoldText(text string) string {
	unfolded := strings.Builder{}
	for at := 0; at < len(text); at++ {
		if text[at] != '\\' || at+1 >= len(text) {
			unfolded.WriteByte(text[at])
			continue
		}
		switch text[at+1] {
		case '\\':
			unfolded.WriteByte('\\')
		case 'n':
			unfolded.WriteByte('\n')
		case 'r':
			unfolded.WriteByte('\r')
		default:
			unfolded.WriteByte('\\')
			unfolded.WriteByte(text[at+1])
		}
		at++
	}
	return unfolded.String()
}

// quoted wraps the user's own words in the double quotes the design puts around
// an ask and a correction.
func quoted(text string) string {
	return `"` + foldText(text) + `"`
}

// unquote takes the double quotes off a piece of the user's own words and says
// whether they were there, because those words without them are a broken line.
func unquote(text string) (string, bool) {
	if len(text) < 2 || !strings.HasPrefix(text, `"`) || !strings.HasSuffix(text, `"`) {
		return "", false
	}
	return unfoldText(text[1 : len(text)-1]), true
}

// knownResultID says whether an identifier is one this package would have written
// for a result of this kind of record: "r7" on a task, "j4.2" on job number four.
func knownResultID(kind contract.RecordKind, recordID string, id string) bool {
	if kind == contract.RecordJob {
		jobID, _, valid := contract.ParseReportID(id)
		return valid && jobID == recordID
	}
	_, valid := contract.ParseResultID(id)
	return valid
}

// readThousands reads a token count written as "6.1k" with the words after it,
// which is the only precision the record's cost line keeps.
func readThousands(text string, after string) (int, bool) {
	number, found := strings.CutSuffix(text, after)
	if !found {
		return 0, false
	}
	number, found = strings.CutSuffix(number, "k")
	if !found {
		return 0, false
	}
	whole, tenth, split := strings.Cut(number, ".")
	if !split || len(tenth) != 1 {
		return 0, false
	}
	wholeCount, readWhole := readCount(whole)
	if whole == "0" {
		wholeCount, readWhole = 0, true
	}
	tenthCount, readTenth := readCount(tenth)
	if tenth == "0" {
		tenthCount, readTenth = 0, true
	}
	if !readWhole || !readTenth {
		return 0, false
	}
	return wholeCount*1000 + tenthCount*100, true
}
