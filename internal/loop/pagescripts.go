package loop

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// A page's scripts are what its Start button runs, and the task that
// play-tests a page is seldom the task that wrote them. So the harness reads
// them from where the page reads them: the local server the model started, or
// the disk when the page was opened as a file, and never another machine.

// MaxScriptsRead is the most scripts read off one page.
const MaxScriptsRead = 8

// MaxScriptBytes is the most of one page or script that is read.
const MaxScriptBytes = 2 << 20

// PageReadDeadline bounds the whole reading of a page and its scripts.
const PageReadDeadline = 5 * time.Second

// theScriptSource finds the address of every script a page loads.
var theScriptSource = regexp.MustCompile(`(?is)<script\b[^>]*\bsrc\s*=\s*["']?([^"'\s>]+)`)

// theLoopOpening finds a while or a for on a line.
var theLoopOpening = regexp.MustCompile(`\b(while|for)\s*\(`)

// namedText is one page or script with the name it is listed under.
type namedText struct {
	name string
	text string
}

// isAPageOnThisMachine says whether an address is served from this machine
// or is a file on it, the only pages whose scripts are read.
func isAPageOnThisMachine(address string) bool {
	parsed, err := url.Parse(address)
	if err != nil {
		return false
	}
	switch parsed.Scheme {
	case "file":
		return true
	case "http", "https":
		host := parsed.Hostname()
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	}
	return false
}

// theScriptsOf reads a page and the scripts it loads, the page first so that
// its inline loops are listed too. A page that cannot be read gives nothing.
func theScriptsOf(ctx context.Context, address string) []namedText {
	ctx, cancel := context.WithTimeout(ctx, PageReadDeadline)
	defer cancel()
	base, err := url.Parse(address)
	if err != nil {
		return nil
	}
	page, err := readAddress(ctx, base)
	if err != nil {
		return nil
	}
	read := []namedText{{name: filepath.Base(base.Path), text: page}}
	for _, source := range theScriptSourcesIn(page) {
		if len(read) > MaxScriptsRead {
			break
		}
		relative, err := url.Parse(source)
		if err != nil {
			continue
		}
		whole := base.ResolveReference(relative)
		if !isAPageOnThisMachine(whole.String()) {
			continue
		}
		text, err := readAddress(ctx, whole)
		if err != nil {
			continue
		}
		read = append(read, namedText{name: filepath.Base(whole.Path), text: text})
	}
	return read
}

// theScriptSourcesIn lists the script addresses a page names, in order.
func theScriptSourcesIn(page string) []string {
	sources := []string{}
	for _, match := range theScriptSource.FindAllStringSubmatch(page, MaxScriptsRead) {
		sources = append(sources, match[1])
	}
	return sources
}

// readAddress reads one page or script, from the local server or the disk,
// up to MaxScriptBytes.
func readAddress(ctx context.Context, at *url.URL) (string, error) {
	if at.Scheme == "file" {
		file, err := os.Open(at.Path)
		if err != nil {
			return "", err
		}
		defer file.Close()
		return readUpToTheCap(file)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, at.String(), nil)
	if err != nil {
		return "", err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s answered %d, so the script cannot be read", at, response.StatusCode)
	}
	return readUpToTheCap(response.Body)
}

// readUpToTheCap reads at most MaxScriptBytes.
func readUpToTheCap(from io.Reader) (string, error) {
	read, err := io.ReadAll(io.LimitReader(from, MaxScriptBytes))
	return string(read), err
}

// TheNeverChangesMark is put after a while loop whose body never mentions a
// word its condition reads, which is the loop that never yields nine times in
// ten: the ghost-piece loop of the fifth game build tested the piece's cells
// and counted a number its condition never looked at.
const TheNeverChangesMark = "<- nothing in this loop's body touches what its condition reads"

// MaxBodyLinesRead is how far into a loop's body the search for its
// condition's words goes.
const MaxBodyLinesRead = 60

// loopLinesIn lists every line of a script that opens a while or a for, as
// name:line: text, the whiles apart from the fors, because a while is the
// usual loop whose condition never changes and a counted for seldom is. A
// while whose body never mentions what its condition reads is marked, and
// comes first among the whiles.
func loopLinesIn(script namedText) (whiles []string, fors []string) {
	lines := strings.Split(script.text, "\n")
	marked := []string{}
	for number, line := range lines {
		match := theLoopOpening.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		listed := fmt.Sprintf("%s:%d: %s", script.name, number+1, strings.TrimSpace(line))
		switch {
		case match[1] != "while":
			fors = append(fors, listed)
		case bodyNeverTouchesTheCondition(lines, number):
			marked = append(marked, listed+" "+TheNeverChangesMark)
		default:
			whiles = append(whiles, listed)
		}
	}
	return append(marked, whiles...), fors
}

// bodyNeverTouchesTheCondition says whether the body of the while opened on
// the line at this index, up to its closing brace or MaxBodyLinesRead lines,
// mentions none of the words its condition reads. A loop with no brace on its
// line, or no word in its condition, is left unmarked.
func bodyNeverTouchesTheCondition(lines []string, at int) bool {
	opening := lines[at]
	brace := strings.Index(opening, "{")
	conditionStart := strings.Index(opening, "(")
	if brace < 0 || conditionStart < 0 || brace < conditionStart {
		return false
	}
	condition := map[string]bool{}
	for _, word := range wordsOfCode(opening[conditionStart:brace]) {
		condition[word] = true
	}
	if len(condition) == 0 {
		return false
	}
	depth := strings.Count(opening[brace:], "{") - strings.Count(opening[brace:], "}")
	body := []string{opening[brace+1:]}
	for index := at + 1; index < len(lines) && depth > 0 && len(body) <= MaxBodyLinesRead; index++ {
		body = append(body, lines[index])
		depth += strings.Count(lines[index], "{") - strings.Count(lines[index], "}")
	}
	for _, word := range wordsOfCode(strings.Join(body, "\n")) {
		if condition[word] {
			return false
		}
	}
	return true
}

// wordsOfCode is the names in a piece of code: runs of letters, digits,
// underscores and dollar signs that start with a letter, underscore or
// dollar sign.
func wordsOfCode(code string) []string {
	words := []string{}
	for _, run := range strings.FieldsFunc(code, func(character rune) bool {
		return !(character == '_' || character == '$' || (character >= '0' && character <= '9') || (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z'))
	}) {
		if run[0] >= '0' && run[0] <= '9' {
			continue
		}
		words = append(words, run)
	}
	return words
}
