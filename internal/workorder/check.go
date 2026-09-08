package workorder

import (
	"strconv"
	"strings"
)

// A done line the harness can check itself says how, in brackets at its end:
// "[tests pass: npm test]", "[exit 0: node build.js]", "[shows: "Tater Tots
// Tetris" at http://127.0.0.1:8091]", "[exists: dist/index.html]", "[looks:
// http://127.0.0.1:8091 at 1440, 768, 390]". The five kinds are the ones the
// harness already knows how to run: a command whose output reads as a test
// run, a command whose exit code is read, a page opened in the browser and
// read for a text, a file looked for, and a page opened at each width and
// read for an overflow and a page error. Anything else in brackets is words,
// not a check.

// The bounds of a looks check: how many widths one line may name, the
// narrowest and widest page the browser is asked for, which are the resize
// tool's own, and the width a line that names none is looked at.
const (
	MaxLooksWidths  = 4
	NarrowestLooks  = 320
	WidestLooks     = 3840
	TheDefaultLooks = 1440
)

// Check is one bracketed check read off the end of a done line.
type Check struct {
	// Kind is one of "tests pass", "exit 0", "shows", "exists" and "looks", in
	// lower case.
	Kind string
	// Argument is everything after the kind's colon, trimmed: the command, the
	// path, or the quoted text and its address.
	Argument string
	// Text is the quoted text a "shows" check looks for, and is empty otherwise.
	Text string
	// URL is the address a "shows" or a "looks" check opens, and is empty
	// otherwise.
	URL string
	// Widths are the page widths a "looks" check opens the address at, in
	// pixels, in the order written, and nil otherwise.
	Widths []int
	// Line is the done line without its check, trimmed.
	Line string
}

// theKinds are the five checks, as they are written.
var theKinds = []string{"tests pass", "exit 0", "shows", "exists", "looks"}

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
	if kind == "looks" {
		url, widths, found := readLooks(argument)
		if !found {
			return Check{}, false
		}
		check.URL, check.Widths = url, widths
	}
	return check, true
}

// readLooks splits a "looks" argument, an address and then " at " and a list
// of widths, and says whether it had that shape: the address must be one, the
// widths whole numbers within the bounds, at most MaxLooksWidths of them, and
// an argument with no " at " is the address alone at the default width.
func readLooks(argument string) (string, []int, bool) {
	url, list, hasWidths := strings.Cut(argument, " at ")
	url = strings.TrimSpace(url)
	if !isAnAddress(url) {
		return "", nil, false
	}
	if !hasWidths {
		return url, []int{TheDefaultLooks}, true
	}
	var widths []int
	for _, word := range strings.Split(list, ",") {
		width, err := strconv.Atoi(strings.TrimSpace(word))
		if err != nil || width < NarrowestLooks || width > WidestLooks {
			return "", nil, false
		}
		widths = append(widths, width)
	}
	if len(widths) == 0 || len(widths) > MaxLooksWidths {
		return "", nil, false
	}
	return url, widths, true
}

// isAnAddress says whether the text is a web address or a file the browser
// can open.
func isAnAddress(text string) bool {
	if strings.ContainsAny(text, " \t") {
		return false
	}
	return strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://") || strings.HasPrefix(text, "file://")
}

// isAKind says whether the word is one of the five checks.
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
