package web

import (
	"fmt"
	"strconv"
	"strings"
)

// The bounds on turning one page into text.
const (
	// MaxTags is how many tags one page is read through before the reader gives
	// up, so that a page made of nothing but tags cannot hold the agent up.
	MaxTags = 200000
	// MaxHeadingLevel is how deep a heading goes, from h1 to h6.
	MaxHeadingLevel = 6
)

// tagsWhoseWordsAreDropped hold text that is not the page's words: a script, a
// stylesheet, and the fallback shown when scripts are off.
var tagsWhoseWordsAreDropped = map[string]bool{"script": true, "style": true, "noscript": true, "head": true}

// tagsThatBreakTheLine end the line they are on, so that a paragraph and a
// heading do not run into the next one.
var tagsThatBreakTheLine = map[string]bool{
	"p": true, "div": true, "br": true, "tr": true, "section": true, "article": true,
	"header": true, "footer": true, "ul": true, "ol": true, "table": true, "blockquote": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "li": true,
}

// HTMLToText turns a web page into the text the model reads, keeping the three
// things a model needs to make sense of a page: the headings, the links with
// their addresses, and where one item of a list ends and the next begins.
// Everything else, including the words inside a script or a stylesheet, is
// dropped. It never fails: any text at all comes back as some text.
func HTMLToText(page string) string {
	written := &strings.Builder{}
	dropUntil := ""
	at := 0

	for tags := 0; at < len(page) && tags < MaxTags; tags++ {
		next := strings.IndexByte(page[at:], '<')
		if next < 0 {
			break
		}
		if dropUntil == "" {
			written.WriteString(unescape(page[at : at+next]))
		}
		name, closing, after := readTag(page, at+next)
		if after <= at+next {
			at = at + next + 1
			continue
		}
		dropUntil = writeTag(written, page, at+next, name, closing, dropUntil)
		at = after
	}
	if dropUntil == "" && at < len(page) {
		written.WriteString(unescape(page[at:]))
	}
	return tidy(written.String())
}

// writeTag writes what one tag means in the text, and says which tag the reader
// is waiting for the end of, so that the words inside a script are dropped.
func writeTag(written *strings.Builder, page string, at int, name string, closing bool, dropUntil string) string {
	if dropUntil != "" {
		if closing && name == dropUntil {
			return ""
		}
		return dropUntil
	}
	switch {
	case !closing && tagsWhoseWordsAreDropped[name]:
		return name
	case name == "a" && !closing:
		writeLinkOpening(written, page, at)
	case name == "a" && closing:
		written.WriteString("]")
	case name == "li" && !closing:
		written.WriteString("\n- ")
	case headingLevel(name) > 0 && !closing:
		fmt.Fprintf(written, "\n%s ", strings.Repeat("#", headingLevel(name)))
	case tagsThatBreakTheLine[name]:
		written.WriteString("\n")
	}
	return ""
}

// writeLinkOpening writes the mark that opens a link and the address it points
// at, so that the model can follow it.
func writeLinkOpening(written *strings.Builder, page string, at int) {
	address := attributeOf(page, at, "href")
	if address == "" {
		written.WriteString("[")
		return
	}
	fmt.Fprintf(written, "[%s ", unescape(address))
}

// readTag reads one tag and returns its name, whether it closes something, and
// where the text after it starts. A comment is passed over whole.
func readTag(page string, at int) (string, bool, int) {
	if strings.HasPrefix(page[at:], "<!--") {
		end := strings.Index(page[at:], "-->")
		if end < 0 {
			return "", false, len(page)
		}
		return "", false, at + end + 3
	}
	end := strings.IndexByte(page[at:], '>')
	if end < 0 {
		return "", false, len(page)
	}
	inside := page[at+1 : at+end]
	closing := strings.HasPrefix(inside, "/")
	inside = strings.TrimPrefix(inside, "/")
	name, _, _ := strings.Cut(strings.TrimSpace(inside), " ")
	return strings.ToLower(strings.TrimSuffix(name, "/")), closing, at + end + 1
}

// attributeOf reads one attribute out of the tag that starts here, which is how
// a link's address is found.
func attributeOf(page string, at int, wanted string) string {
	end := strings.IndexByte(page[at:], '>')
	if end < 0 {
		return ""
	}
	inside := page[at+1 : at+end]
	found := strings.Index(strings.ToLower(inside), wanted+"=")
	if found < 0 {
		return ""
	}
	rest := strings.TrimSpace(inside[found+len(wanted)+1:])
	if rest == "" {
		return ""
	}
	if quote := rest[0]; quote == '"' || quote == '\'' {
		closed := strings.IndexByte(rest[1:], quote)
		if closed < 0 {
			return rest[1:]
		}
		return rest[1 : closed+1]
	}
	value, _, _ := strings.Cut(rest, " ")
	return value
}

// headingLevel is the depth of a heading tag, and zero for a tag that is not
// one.
func headingLevel(name string) int {
	if len(name) != 2 || name[0] != 'h' {
		return 0
	}
	level, err := strconv.Atoi(name[1:])
	if err != nil || level < 1 || level > MaxHeadingLevel {
		return 0
	}
	return level
}

// namedCharacters are the handful of named marks a page uses often enough to be
// worth knowing by name.
var namedCharacters = map[string]string{
	"amp": "&", "lt": "<", "gt": ">", "quot": `"`, "apos": "'",
	"nbsp": " ", "hellip": "...", "mdash": "-", "ndash": "-", "#39": "'",
}

// unescape turns the marks a page writes as names or numbers back into the
// characters they stand for.
func unescape(text string) string {
	if !strings.ContainsRune(text, '&') {
		return text
	}
	written := &strings.Builder{}
	for at := 0; at < len(text); {
		if text[at] != '&' {
			written.WriteByte(text[at])
			at++
			continue
		}
		end := strings.IndexByte(text[at:], ';')
		if end < 0 || end > 12 {
			written.WriteByte(text[at])
			at++
			continue
		}
		written.WriteString(oneCharacter(text[at+1 : at+end]))
		at += end + 1
	}
	return written.String()
}

// oneCharacter turns one escaped name or number into the character it stands
// for, and leaves it as it was written when it stands for nothing.
func oneCharacter(name string) string {
	if found, known := namedCharacters[strings.ToLower(name)]; known {
		return found
	}
	if number, isNumber := readNumberedCharacter(name); isNumber {
		return string(number)
	}
	return "&" + name + ";"
}

// readNumberedCharacter reads a character written as a number, in either the
// decimal or the hexadecimal form a page may use.
func readNumberedCharacter(name string) (rune, bool) {
	digits, found := strings.CutPrefix(name, "#")
	if !found {
		return 0, false
	}
	base := 10
	if hex, isHex := strings.CutPrefix(strings.ToLower(digits), "x"); isHex {
		digits, base = hex, 16
	}
	number, err := strconv.ParseInt(digits, base, 32)
	if err != nil || number <= 0 || number > 0x10FFFF {
		return 0, false
	}
	return rune(number), true
}

// tidy collapses the runs of spaces and blank lines the tags left behind, and
// cuts the whole to the page cap.
func tidy(text string) string {
	lines := []string{}
	for _, line := range strings.Split(text, "\n") {
		if kept := strings.Join(strings.Fields(line), " "); kept != "" {
			lines = append(lines, kept)
		}
	}
	joined := strings.Join(lines, "\n")
	if len(joined) > MaxPageBytes {
		return joined[:MaxPageBytes]
	}
	return joined
}
