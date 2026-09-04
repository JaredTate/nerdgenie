// The five envelope shapes and the order of trust between them are ZeroClaw's
// design, read from its parser and its tests at
// ~/Code/zeroclaw/crates/zeroclaw-tool-call-parser/src/lib.rs and written fresh
// here.

package repair

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// candidate is one thing in a reply that looked like a tool call, with where it
// sat so that the envelope can be taken out of the text that is left.
type candidate struct {
	// id is the provider's identifier, and is empty for a call found in text.
	id string
	// name is the tool name exactly as it was written.
	name string
	// arguments are the arguments exactly as they were written, and may be
	// empty when the model wrote none.
	arguments []byte
	// start and end are where the envelope sits in the searched text.
	start int
	end   int
	// unreadable says the envelope was plainly a tool call and its contents
	// could not be read at all.
	unreadable bool
}

// The regular expressions are compiled once, at start-up, because they are used
// on every model reply.
var (
	toolCallOpenTag  = regexp.MustCompile("(?i)" + regexp.QuoteMeta(contract.ToolCallOpenTag))
	toolCallCloseTag = regexp.MustCompile("(?i)" + regexp.QuoteMeta(contract.ToolCallCloseTag))
	codeFence        = regexp.MustCompile("(?s)```[ \t]*([A-Za-z0-9_+-]*)[ \t]*\r?\n(.*?)```")
	functionCallLine = regexp.MustCompile(`(?m)^[ \t]*([A-Za-z_][A-Za-z0-9_.-]{0,119})[ \t]*(\(|:)[ \t]*`)
)

// scan reads the text for tool calls, most trusted shape first, and stops at the
// first shape that finds anything. A shape that is plainly a tool call and
// cannot be read stops the scan too, because falling through to a looser shape
// would hide the mistake from the model rather than tell it.
func scan(text string) []candidate {
	if found := scanTagBlocks(text); len(found) > 0 {
		return found
	}
	if found := scanCodeFences(text); len(found) > 0 {
		return found
	}
	if found := scanBareObjects(text); len(found) > 0 {
		return found
	}
	return scanFunctionLines(text)
}

// scanTagBlocks reads the one text form of a tool call that the harness itself
// asks for, with or without its closing tag, one block or many.
func scanTagBlocks(text string) []candidate {
	found := []candidate{}
	at := 0
	for _, opening := range toolCallOpenTag.FindAllStringIndex(text, maxCallsInOneReply+1) {
		if opening[0] < at {
			continue
		}
		body, end := tagBody(text, opening[1])
		at = end
		inside, readable := callsInJSON(stripFence(body))
		if !readable {
			found = append(found, candidate{start: opening[0], end: end, unreadable: true})
			continue
		}
		found = append(found, placed(inside, opening[0], end)...)
	}
	return found
}

// tagBody returns what sits between the opening tag and either its closing tag
// or, when the model wrote no closing tag, the end of the JSON value it started.
func tagBody(text string, from int) (string, int) {
	if closing := toolCallCloseTag.FindStringIndex(text[from:]); closing != nil {
		return text[from : from+closing[0]], from + closing[1]
	}
	opened := strings.IndexAny(text[from:], "{[")
	if opened < 0 {
		return text[from:], len(text)
	}
	end := balancedSpan(text, from+opened)
	if end < 0 {
		return text[from:], len(text)
	}
	return text[from+opened : end], end
}

// scanCodeFences reads a fence with no language tag or with the json tag. A
// fence holding JSON that is not a tool call is left alone, because a model
// showing the user a settings file is not asking for a tool.
func scanCodeFences(text string) []candidate {
	found := []candidate{}
	for _, fence := range codeFence.FindAllStringSubmatchIndex(text, maxCallsInOneReply+1) {
		language := strings.ToLower(text[fence[2]:fence[3]])
		if language != "" && language != "json" {
			continue
		}
		body := text[fence[4]:fence[5]]
		inside, readable := callsInJSON(body)
		if !readable {
			if looksLikeCall(body) {
				found = append(found, candidate{start: fence[0], end: fence[1], unreadable: true})
			}
			continue
		}
		found = append(found, placed(inside, fence[0], fence[1])...)
	}
	return found
}

// scanBareObjects reads a JSON object written straight into the text with a name
// and an arguments object, which is what a small model writes when it has
// forgotten the tags altogether.
func scanBareObjects(text string) []candidate {
	found := []candidate{}
	for _, span := range topLevelObjectSpans(text, maxCallsInOneReply+1) {
		one, ok := bareCandidate([]byte(text[span[0]:span[1]]))
		if !ok {
			continue
		}
		one.start, one.end = span[0], span[1]
		found = append(found, one)
	}
	return found
}

// scanFunctionLines reads a line that names a tool and then writes its arguments
// as an object, either as name({...}) or as name: {...}.
func scanFunctionLines(text string) []candidate {
	found := []candidate{}
	for _, line := range functionCallLine.FindAllStringSubmatchIndex(text, maxCallsInOneReply+1) {
		opened := line[1]
		if opened >= len(text) || text[opened] != '{' {
			continue
		}
		end := balancedSpan(text, opened)
		if end < 0 {
			continue
		}
		arguments := []byte(text[opened:end])
		if _, ok := compactObject(arguments); !ok {
			continue
		}
		if text[line[4]] == '(' && end < len(text) && text[end] == ')' {
			end++
		}
		found = append(found, candidate{name: text[line[2]:line[3]], arguments: arguments, start: line[0], end: end})
	}
	return found
}

// placed stamps the same span on every call that came out of one envelope, so
// that an envelope holding a list of calls is taken out of the text once.
func placed(found []candidate, start int, end int) []candidate {
	for index := range found {
		found[index].start, found[index].end = start, end
	}
	return found
}

// stripFence takes a code fence off the inside of a tool-call block, which is
// what a model does when it has been taught to write JSON in fences and then
// asked to write it in tags.
func stripFence(body string) string {
	trimmed := strings.TrimSpace(body)
	if !strings.HasPrefix(trimmed, "```") {
		return body
	}
	_, rest, found := strings.Cut(trimmed, "\n")
	if !found {
		return body
	}
	return strings.TrimSuffix(strings.TrimSpace(rest), "```")
}

// callsInJSON reads an envelope's contents as either one tool call or a list of
// them, and says whether it could read them at all.
func callsInJSON(body string) ([]candidate, bool) {
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, "[") {
		var items []json.RawMessage
		if json.Unmarshal([]byte(trimmed), &items) != nil {
			return nil, false
		}
		found := []candidate{}
		for _, item := range items {
			one, ok := candidateFromObject(item)
			if !ok {
				return nil, false
			}
			found = append(found, one)
		}
		return found, len(found) > 0
	}
	one, ok := candidateFromObject([]byte(trimmed))
	if !ok {
		return nil, false
	}
	return []candidate{one}, true
}

// writtenCall is the JSON a model writes for one tool call: a name, and its
// arguments under either of the two words the two big providers use for them.
type writtenCall struct {
	// Name is the tool the model asked for.
	Name string `json:"name"`
	// Arguments is what the two text-shaped forms and the OpenAI interface call
	// the arguments.
	Arguments json.RawMessage `json:"arguments"`
	// Input is what the Anthropic interface calls them.
	Input json.RawMessage `json:"input"`
}

// candidateFromObject reads one JSON object as a tool call.
func candidateFromObject(raw []byte) (candidate, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return candidate{}, false
	}
	written := writtenCall{}
	if json.Unmarshal(trimmed, &written) != nil {
		return candidate{}, false
	}
	name := strings.TrimSpace(written.Name)
	if name == "" {
		return candidate{}, false
	}
	arguments := written.Arguments
	if len(bytes.TrimSpace(arguments)) == 0 {
		arguments = written.Input
	}
	return candidate{name: name, arguments: arguments}, true
}

// bareCandidate is the stricter reading the loosest shapes get: a bare object in
// the middle of prose counts as a tool call only when it carries both a name and
// an arguments object, so that ordinary JSON in a reply stays ordinary text.
func bareCandidate(raw []byte) (candidate, bool) {
	one, ok := candidateFromObject(raw)
	if !ok {
		return candidate{}, false
	}
	arguments := bytes.TrimSpace(one.arguments)
	if len(arguments) == 0 || arguments[0] != '{' {
		return candidate{}, false
	}
	return one, true
}

// looksLikeCall says whether text that would not parse was nonetheless plainly
// meant as a tool call, which is what turns a broken envelope into a message the
// model can act on rather than silence.
func looksLikeCall(text string) bool {
	if !strings.Contains(text, `"name"`) {
		return false
	}
	return strings.Contains(text, `"arguments"`) || strings.Contains(text, `"input"`)
}

// topLevelObjectSpans walks the text once and returns where each JSON object
// that is not inside another one begins and ends, up to the limit.
func topLevelObjectSpans(text string, limit int) [][2]int {
	spans := [][2]int{}
	at := 0
	for len(spans) < limit {
		opened := strings.IndexByte(text[at:], '{')
		if opened < 0 {
			return spans
		}
		start := at + opened
		end := balancedSpan(text, start)
		if end < 0 {
			return spans
		}
		spans = append(spans, [2]int{start, end})
		at = end
	}
	return spans
}

// balancedSpan returns where the JSON value that begins at start closes, or -1
// when it never closes. Text inside a JSON string is skipped, so a brace in a
// file path or a command never throws the count off.
func balancedSpan(text string, start int) int {
	depth := 0
	inString := false
	escaped := false
	for index := start; index < len(text); index++ {
		letter := text[index]
		if inString {
			switch {
			case escaped:
				escaped = false
			case letter == '\\':
				escaped = true
			case letter == '"':
				inString = false
			}
			continue
		}
		switch letter {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth <= 0 {
				return index + 1
			}
		}
	}
	return -1
}
