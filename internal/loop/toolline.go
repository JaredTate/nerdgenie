package loop

import (
	"encoding/json"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// ToolLineMark is what the design puts in front of the one dim line that says
// which tool is running, so that a screen and a test read one shape.
const ToolLineMark = "▸"

// ToolLineSeparator stands between the call and what came back of it.
const ToolLineSeparator = " · "

// MaxToolLineRunes is the longest a tool line may be. It is one line on a strip
// beside everything else, so it is short.
const MaxToolLineRunes = 90

// MaxArgumentRunes is the longest the argument on a tool line may be, so that
// what came back, and the word refused, always have room on the line: the
// fifth game build's screen drew a green check on a refused click because the
// click's intent was a whole sentence and the word refused was cut off the end.
const MaxArgumentRunes = 40

// The names of the arguments that say what a call will do, in the order they are
// looked for. They are the ones internal/permission already reduces a call by, so
// a person reads the same words on the strip that they read in a preview.
var argumentsThatSayWhat = []string{
	"command", "path", "pattern", "url", "query", "intent", "name", "id",
}

// toolLineFor writes the one line the strip draws for a call: the mark, the
// tool's name, and the one argument that says what it will do, with what came
// back added once it has. The design writes it as "▸ task done_when" while the
// call runs and "▸ task done_when · updated the record · r5" once it is done.
func toolLineFor(call contract.ToolCall, came string, failed bool) string {
	line := ToolLineMark + " " + call.Name
	if said := whatTheCallSays(call); said != "" {
		line += " " + cutToRunes(said, MaxArgumentRunes)
	}
	ending := ""
	if failed {
		// The summary of a refused call begins with the tool's name and "was
		// refused", which the line's front and end already say.
		came = strings.Replace(came, call.Name+" was refused: ", "", 1)
		ending = ToolLineSeparator + "refused"
	}
	if came != "" {
		line += ToolLineSeparator + oneLine(came)
	}
	return cutToRunes(line, MaxToolLineRunes-len([]rune(ending))) + ending
}

// whatTheCallSays is the one argument that says what a call will do: the command
// a shell will run, the path a file tool will touch, the address the web tool
// will fetch. A call whose arguments name none of them is described by the
// fields it wrote, which is what the task tool's calls look like.
func whatTheCallSays(call contract.ToolCall) string {
	written := map[string]json.RawMessage{}
	if err := json.Unmarshal(call.Input, &written); err != nil {
		return ""
	}
	for _, name := range argumentsThatSayWhat {
		if held, there := written[name]; there {
			return oneLine(plainText(held))
		}
	}
	return strings.Join(namesWritten(written), " ")
}

// namesWritten is the names of the fields a call wrote, in the order the JSON
// reader gives them, sorted so that one call always reads the same way.
func namesWritten(written map[string]json.RawMessage) []string {
	names := make([]string, 0, len(written))
	for name := range written {
		names = append(names, name)
	}
	sortWords(names)
	return names
}

// sortWords puts a handful of words in order without pulling in a package for
// it, because the list is never longer than a tool's fields.
func sortWords(words []string) {
	for at := 1; at < len(words); at++ {
		for back := at; back > 0 && words[back] < words[back-1]; back-- {
			words[back], words[back-1] = words[back-1], words[back]
		}
	}
}

// plainText reads a JSON value as the text a person would read: a string as
// itself, and anything else as the JSON it is.
func plainText(held json.RawMessage) string {
	said := ""
	if err := json.Unmarshal(held, &said); err == nil {
		return said
	}
	return string(held)
}

// oneLine folds text onto one line, because a strip has one line to draw in.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// noteToolLine sends one line about the call in flight to whoever is watching.
func (running *run) noteToolLine(line string) {
	if running.theLoop.options.ToolLine == nil {
		return
	}
	running.theLoop.options.ToolLine(line)
}
