package record

import "strings"

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

// quoted wraps the user's own words in the double quotes the design puts around
// an ask and a correction.
func quoted(text string) string {
	return `"` + foldText(text) + `"`
}
