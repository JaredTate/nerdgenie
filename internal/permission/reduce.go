// The readable form of a command is the design OpenCode calls its arity table,
// at ~/Code/opencode/packages/opencode/src/permission/arity.ts: a command line is
// worth reading only once the arguments that change every time are gone. The
// table below is written fresh for Coeus and holds only the programs Coeus rules
// on, plus the two git subcommands whose flags decide whether anything is
// destroyed.

package permission

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"unicode"

	"github.com/JaredTate/coeus/internal/contract"
)

// MaxReducedRunes is the cap on the readable form of a call. It goes into a log
// line, a pattern match, and a preview, and none of those wants a page of text.
const MaxReducedRunes = 200

// EmptiesFileOverBytes is the size at which emptying a file counts as deleting
// many things at once. Emptying a scratch file of a few lines is ordinary work;
// emptying a file bigger than this is not.
const EmptiesFileOverBytes = 4096

// maxInputBytes is how much of a tool call's arguments are read at all, because
// the arguments come from a model and a model can write anything.
const maxInputBytes = 1 << 20

// sudoProgram is the one program that means "with administrator powers", and it
// stays in the readable form so that the user and the log both see it.
const sudoProgram = "sudo"

// commandShape says how one program's command line is read: how many words,
// counting the program and its subcommands and never its flags, define the
// command, and whether the flags the user wrote belong in the readable form.
type commandShape struct {
	// words is how many words, the program included, define the command.
	words int
	// keepFlags says whether the flags change what the command does enough to
	// belong in the readable form.
	keepFlags bool
}

// defaultShape is what a program the table does not name gets: the program
// itself and its flags, because for a program with no subcommand it is the flags
// that tell "rm -rf" apart from "rm".
var defaultShape = commandShape{words: 1, keepFlags: true}

// commandShapes names the programs that take a subcommand, so that their
// subcommand is kept and their flags are not, and the two git subcommands whose
// flags decide whether anything is destroyed.
var commandShapes = map[string]commandShape{
	"apt":            {words: 2},
	"apt-get":        {words: 2},
	"brew":           {words: 2},
	"bun":            {words: 2},
	"bun run":        {words: 3},
	"cargo":          {words: 2},
	"docker":         {words: 2},
	"docker compose": {words: 3},
	"gh":             {words: 3},
	"git":            {words: 2},
	"git clean":      {words: 2, keepFlags: true},
	"git reset":      {words: 2, keepFlags: true},
	"go":             {words: 2},
	"kubectl":        {words: 2},
	"make":           {words: 2},
	"npm":            {words: 2},
	"npm run":        {words: 3},
	"pip":            {words: 2},
	"systemctl":      {words: 2},
	"yarn":           {words: 2},
}

// mainFieldsByTool names the input field that says what a call will actually do,
// for every tool whose readable form is more than its name. The field names are
// the ones the tools in docs/briefs/wave-2/2.5-tools.md take, and the first one
// present is used, so a tool that names its field differently still reduces to
// something a person can read.
var mainFieldsByTool = map[string][]string{
	contract.ToolRead:           {"path"},
	contract.ToolWrite:          {"path"},
	contract.ToolEdit:           {"path"},
	contract.ToolSearch:         {"pattern", "path"},
	contract.ToolWeb:            {"url", "query"},
	contract.ToolSkill:          {"name"},
	contract.ToolJob:            {"name"},
	contract.ToolBrowserOpen:    {"url", "intent"},
	contract.ToolBrowserRead:    {"url", "intent"},
	contract.ToolBrowserClick:   {"intent", "element"},
	contract.ToolBrowserType:    {"intent", "element", "text"},
	contract.ToolBrowserAct:     {"intent"},
	contract.ToolBrowserLogin:   {"site", "url"},
	contract.ToolBrowserHandoff: {"intent", "reason"},
	contract.ToolComputer:       {"intent"},
}

// Reduce returns the readable form of one tool call: a shell command cut down to
// its program and the words that matter, or any other tool's name and the one
// field that says what it will do. It is what the rules match on, what the event
// log records, and what the preview shows, so it always fits on one line and is
// never longer than MaxReducedRunes.
func Reduce(request contract.PermissionRequest) string {
	form, _ := reduceCall(request)
	return form
}

// reduceCall returns the readable form of one tool call and, when that form is
// not the whole story, the note saying what it leaves out. The permission
// function puts such a call to the user rather than running it, because a bound
// that hides the end of a command is a bound that turns the rule off.
func reduceCall(request contract.PermissionRequest) (string, string) {
	if request.ToolName == contract.ToolShell {
		line, note := reduceShellCall(request)
		return oneLine(line, note)
	}
	return oneLine(reduceOtherCall(request), "")
}

// reduceShellCall reduces a shell call to the commands it runs, in order, with
// the arguments that change every time left out, and passes on the note when the
// call does not say everything it will do.
func reduceShellCall(request contract.PermissionRequest) (string, string) {
	fields := readFields(request.Input)
	command, written := stringField(fields, "command")
	if !written {
		if prefix := strings.TrimSpace(request.CommandPrefix); prefix != "" {
			return prefix, ""
		}
		return contract.ToolShell, ""
	}

	line, note := reduceCommandLine(command, 0)
	if boolField(fields, "escalate") {
		line = strings.TrimSpace(sudoProgram + " " + line)
	}
	return line, note
}

// reduceCommandLine reduces every command on one line, joins them the way a
// shell chains them, and passes on the note when the line does not say
// everything it will do. The depth counts the shells this line is already
// wrapped inside.
func reduceCommandLine(command string, depth int) (string, string) {
	segments, note := commandWords(command)
	reduced := []string{}
	for _, words := range segments {
		part, partNote := reduceOneCommand(words, depth)
		if part != "" {
			reduced = append(reduced, part)
		}
		if note == "" {
			note = partNote
		}
	}
	return strings.Join(reduced, " | "), note
}

// reduceOneCommand reduces the words of one command to its program, the
// subcommand words the table says define it, and the flags that matter. A shell
// handed a script reduces that script as a command line of its own.
func reduceOneCommand(words []string, depth int) (string, string) {
	words = withoutEnvironmentAssignments(words)
	prefix := ""
	if len(words) > 0 && programName(words[0]) == sudoProgram {
		prefix = sudoProgram + " "
		words = withoutSudoFlags(words[1:])
	}
	if len(words) == 0 {
		return strings.TrimSpace(prefix), ""
	}
	if flag, script, handed := scriptHandedToAShell(words); handed {
		return reduceNestedShell(prefix+words[0]+" "+flag, script, depth)
	}
	return prefix + wordsThatDefineTheCommand(withoutTheValuesOfFlags(words)), ""
}

// wordsThatDefineTheCommand keeps the program, the subcommand words the shape
// says define it, and the flags the shape says matter, and leaves out the
// arguments that change every time.
func wordsThatDefineTheCommand(words []string) string {
	shape := shapeOf(words)
	kept := []string{}
	taken := 0
	for _, word := range words {
		switch {
		case isFlag(word):
			if shape.keepFlags {
				kept = append(kept, word)
			}
		case taken < shape.words:
			kept = append(kept, word)
			taken++
		case !shape.keepFlags:
			return strings.Join(kept, " ")
		}
	}
	return strings.Join(kept, " ")
}

// shapeOf finds the shape of a command by its longest named prefix, counting
// only the words that are not flags, and falls back to the default shape. The
// program is looked up by its own name, so that "/usr/bin/git" is git.
func shapeOf(words []string) commandShape {
	leading := []string{}
	for _, word := range words {
		if isFlag(word) {
			continue
		}
		leading = append(leading, word)
		if len(leading) == 3 {
			break
		}
	}
	if len(leading) > 0 {
		leading[0] = programName(leading[0])
	}
	for length := len(leading); length > 0; length-- {
		if shape, named := commandShapes[strings.Join(leading[:length], " ")]; named {
			return shape
		}
	}
	return defaultShape
}

// withoutEnvironmentAssignments drops the "NAME=value" words a shell sets before
// the program, and the "env" that sometimes carries them, so that the program is
// the first word left.
func withoutEnvironmentAssignments(words []string) []string {
	for len(words) > 0 {
		if words[0] == "env" && len(words) > 1 {
			words = words[1:]
			continue
		}
		if !isEnvironmentAssignment(words[0]) {
			break
		}
		words = words[1:]
	}
	return words
}

// withoutSudoFlags drops sudo's own flags, and the value of a flag that takes
// one, so that the program sudo will run is the first word left.
func withoutSudoFlags(words []string) []string {
	takeAValue := flagsThatTakeAValue[sudoProgram]
	for len(words) > 0 && isFlag(words[0]) {
		if slices.Contains(takeAValue, words[0]) && len(words) > 1 {
			words = words[2:]
			continue
		}
		words = words[1:]
	}
	return words
}

// isEnvironmentAssignment says whether a word sets an environment variable, as
// in "PATH=/usr/bin".
func isEnvironmentAssignment(word string) bool {
	name, _, split := strings.Cut(word, "=")
	if !split || name == "" || unicode.IsDigit(rune(name[0])) {
		return false
	}
	for _, letter := range name {
		if letter != '_' && !unicode.IsLetter(letter) && !unicode.IsDigit(letter) {
			return false
		}
	}
	return true
}

// isFlag says whether a word is a flag rather than something to act on. A lone
// dash is the standard input, not a flag.
func isFlag(word string) bool {
	return len(word) > 1 && strings.HasPrefix(word, "-")
}

// reduceOtherCall reduces a call that is not a shell command to the tool's name
// and the one field that says what it will do.
func reduceOtherCall(request contract.PermissionRequest) string {
	fields := readFields(request.Input)
	if note, emptying := emptyingNote(request.ToolName, fields); emptying {
		return request.ToolName + " " + note
	}
	for _, name := range mainFieldsByTool[request.ToolName] {
		value, written := stringField(fields, name)
		if written && strings.TrimSpace(value) != "" {
			return request.ToolName + " " + strings.TrimSpace(value)
		}
	}
	return request.ToolName
}

// emptyingNote says so when a write or an edit would empty a file bigger than
// EmptiesFileOverBytes, because that is deleting many things at once wearing the
// clothes of an ordinary file change.
func emptyingNote(toolName string, fields map[string]json.RawMessage) (string, bool) {
	path, written := stringField(fields, "path")
	if !written || path == "" {
		return "", false
	}
	switch toolName {
	case contract.ToolWrite:
		content, hasContent := stringField(fields, "content")
		if !hasContent || strings.TrimSpace(content) != "" {
			return "", false
		}
		size := sizeOnDisk(path)
		if size <= EmptiesFileOverBytes {
			return "", false
		}
		return fmt.Sprintf("%s emptying a file of %d bytes", path, size), true
	case contract.ToolEdit:
		replacement, _ := stringField(fields, "new")
		removed, _ := stringField(fields, "old")
		if strings.TrimSpace(replacement) != "" || len(removed) <= EmptiesFileOverBytes {
			return "", false
		}
		return fmt.Sprintf("%s emptying a file of %d bytes", path, len(removed)), true
	default:
		return "", false
	}
}

// sizeOnDisk returns how many bytes the file holds now, or zero when there is no
// such file, because a file that is not there cannot be emptied.
func sizeOnDisk(path string) int64 {
	about, err := os.Stat(path)
	if err != nil || about.IsDir() {
		return 0
	}
	return about.Size()
}

// readFields reads a tool call's arguments, and returns nothing at all when they
// are not a JSON object or are longer than the cap.
func readFields(input json.RawMessage) map[string]json.RawMessage {
	if len(input) == 0 || len(input) > maxInputBytes {
		return nil
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(input, &fields); err != nil {
		return nil
	}
	return fields
}

// stringField reads one field written as a JSON string, and says whether it was
// there at all.
func stringField(fields map[string]json.RawMessage, name string) (string, bool) {
	raw, present := fields[name]
	if !present {
		return "", false
	}
	value := ""
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

// boolField reads one field written as a JSON true or false, and is false when
// the field is missing or is something else.
func boolField(fields map[string]json.RawMessage, name string) bool {
	raw, present := fields[name]
	if !present {
		return false
	}
	value := false
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	return value
}

// oneLine puts text on a single line, collapses the runs of spaces that leaves
// behind, and cuts it to the cap, so that a readable form is always readable. It
// ends the form with the note when the form is not the whole story, and the cap
// itself is one of the reasons it may not be.
func oneLine(text string, note string) (string, string) {
	flattened := strings.Map(func(letter rune) rune {
		if letter == '\t' || unicode.IsControl(letter) {
			return ' '
		}
		return letter
	}, text)
	tidied := strings.Join(strings.Fields(flattened), " ")
	letters := []rune(tidied)
	if note == "" && len(letters) <= MaxReducedRunes {
		return tidied, ""
	}
	if note == "" {
		note = cutShortNote
	}
	room := max(MaxReducedRunes-len([]rune(note))-1, 0)
	if len(letters) > room {
		tidied = strings.TrimSpace(string(letters[:room]))
	}
	return strings.TrimSpace(tidied + " " + note), note
}
