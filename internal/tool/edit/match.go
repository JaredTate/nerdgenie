// The idea of trying an exact replacement first and then a short list of
// forgiving matchers, each of which must land on one place or be passed over, is
// OpenCode's, at ~/Code/opencode/packages/opencode/src/tool/edit.ts. The four
// matchers below are written fresh and each has a job the others do not.

package edit

import (
	"errors"
	"fmt"
	"strings"
)

// The names of the five ways a span is found, in the order they are tried. The
// tool says which one it used, because an edit made by a forgiving matcher is
// worth the model knowing about.
const (
	// MatchExact is the text found exactly as it was written.
	MatchExact = "exact"
	// MatchTrimmedLines is the lines found once trailing spaces are off both.
	MatchTrimmedLines = "trimmed-lines"
	// MatchWhitespaceNormalized is one line found once every run of spaces in it
	// counts as one space.
	MatchWhitespaceNormalized = "whitespace-normalized"
	// MatchIndentationFlexible is a block found once the indentation the whole
	// of it shares is taken off both sides.
	MatchIndentationFlexible = "indentation-flexible"
	// MatchUniqueSubstring is the text, with its ends trimmed, found as a
	// substring in exactly one place.
	MatchUniqueSubstring = "unique-substring"
)

// MaxMatchGrowth is how much bigger than the text asked for a matched span may
// be. A forgiving matcher that lands on ten times what the model quoted has
// found something else, and replacing it would be a change nobody asked for.
const MaxMatchGrowth = 3

// ErrSpanNotFound says the text the model quoted is nowhere in the file.
var ErrSpanNotFound = errors.New("that text is not in the file, so read the file again and quote the part to replace exactly as it stands")

// ErrSpanNotUnique says the text the model quoted is in the file more than once.
var ErrSpanNotUnique = errors.New("that text is in the file more than once, so quote more of the lines around it to say which one you mean")

// matcher offers the spans of the text that might be what the model meant, in
// the order they were found. Every span it offers is checked for being in the
// file exactly once before anything is replaced.
type matcher struct {
	// name says which of the five ways this is, for the line the tool returns.
	name string
	// spans returns the candidate spans, which may be none.
	spans func(content string, wanted string) []string
}

// matchers are the five ways of finding a span, in the order they are tried:
// the exact text first, then the four that forgive whitespace, then the one that
// forgives the ends.
var matchers = []matcher{
	{MatchExact, exactSpans},
	{MatchTrimmedLines, trimmedLineSpans},
	{MatchWhitespaceNormalized, whitespaceNormalizedSpans},
	{MatchIndentationFlexible, indentationFlexibleSpans},
	{MatchUniqueSubstring, uniqueSubstringSpans},
}

// Replace puts the new text in place of the one span of the old text in the
// content, and says which of the five matchers found it. It refuses an edit that
// changes nothing, an edit with nothing to look for, a span it cannot find, and
// a span it finds more than once.
func Replace(content string, wanted string, replacement string) (string, string, error) {
	if wanted == "" {
		return "", "", errors.New("this edit says nothing to look for, so quote the text to replace, or use write to replace the whole file")
	}
	if wanted == replacement {
		return "", "", errors.New("the file already holds this text, so the change is in place and this edit would replace it with itself; read the file to see it, and go on")
	}

	found := false
	for _, way := range matchers {
		for _, span := range way.spans(content, wanted) {
			at := strings.Index(content, span)
			if at < 0 || span == "" {
				continue
			}
			found = true
			if len(span) > MaxMatchGrowth*len(wanted)+len(wanted) {
				return "", "", fmt.Errorf("the nearest text in the file is %d bytes against the %d asked for, which is too much of a difference to be the edit you meant: %w",
					len(span), len(wanted), ErrSpanNotFound)
			}
			if at != strings.LastIndex(content, span) {
				continue
			}
			return content[:at] + replacement + content[at+len(span):], way.name, nil
		}
	}
	if !found {
		return "", "", fmt.Errorf("the edit looked for %q: %w", cutForMessage(wanted), ErrSpanNotFound)
	}
	return "", "", fmt.Errorf("the edit looked for %q: %w", cutForMessage(wanted), ErrSpanNotUnique)
}

// cutForMessage shortens a piece of text for an error message, because an error
// carrying a whole file is an error nobody reads.
func cutForMessage(text string) string {
	letters := []rune(text)
	if len(letters) <= 60 {
		return text
	}
	return string(letters[:60]) + "..."
}
