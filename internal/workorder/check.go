package workorder

import "strings"

// A done line the harness can check itself says how, in brackets at its end:
// "[tests pass: npm test]", "[exit 0: node build.js]", "[shows: "Tater Tots
// Tetris" at http://127.0.0.1:8091]", "[exists: dist/index.html]". The four
// kinds are the ones the harness already knows how to run: a command whose
// output reads as a test run, a command whose exit code is read, a page opened
// in the browser and read for a text, and a file looked for. Anything else in
// brackets is words, not a check.

// Check is one bracketed check read off the end of a done line.
type Check struct {
	// Kind is one of "tests pass", "exit 0", "shows" and "exists", in lower case.
	Kind string
	// Argument is everything after the kind's colon, trimmed: the command, the
	// path, or the quoted text and its address.
	Argument string
	// Text is the quoted text a "shows" check looks for, and is empty otherwise.
	Text string
	// URL is the address a "shows" check opens, and is empty otherwise.
	URL string
	// Line is the done line without its check, trimmed.
	Line string
}

// theKinds are the four checks, as they are written.
var theKinds = []string{"tests pass", "exit 0", "shows", "exists"}

// ReadCheck reads the check off the end of a done line and says whether there
// was one. A bracket that does not end the line, a kind it does not know, an
// empty argument, and a "shows" with no quoted text or no address are not
// checks.
func ReadCheck(line string) (Check, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasSuffix(trimmed, "]") {
		return Check{}, false
	}
	open := strings.LastIndex(trimmed, "[")
	if open < 0 {
		return Check{}, false
	}
	inside := trimmed[open+1 : len(trimmed)-1]
	kind, argument, hasColon := strings.Cut(inside, ":")
	if !hasColon {
		return Check{}, false
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	argument = strings.TrimSpace(argument)
	if !isAKind(kind) || argument == "" {
		return Check{}, false
	}
	check := Check{Kind: kind, Argument: argument, Line: strings.TrimSpace(trimmed[:open])}
	if kind == "shows" {
		text, url, found := readShows(argument)
		if !found {
			return Check{}, false
		}
		check.Text, check.URL = text, url
	}
	return check, true
}

// isAKind says whether the word is one of the four checks.
func isAKind(kind string) bool {
	for _, known := range theKinds {
		if kind == known {
			return true
		}
	}
	return false
}

// readShows splits a "shows" argument, a quoted text and then " at " and an
// address, and says whether it had that shape.
func readShows(argument string) (string, string, bool) {
	if !strings.HasPrefix(argument, `"`) {
		return "", "", false
	}
	closing := strings.Index(argument[1:], `"`)
	if closing < 0 {
		return "", "", false
	}
	text := argument[1 : closing+1]
	rest := strings.TrimSpace(argument[closing+2:])
	url, found := strings.CutPrefix(rest, "at ")
	url = strings.TrimSpace(url)
	if !found || text == "" || url == "" {
		return "", "", false
	}
	return text, url, true
}
