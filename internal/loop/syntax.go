package loop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The syntax check after a change. On the fifth game build the play-test task
// wrote a diagnostic script with an unbalanced bracket and ran it four times,
// reading the same "Unexpected token ']'" each time: five rounds on a mistake
// the language's own checker finds in a tenth of a second. After every write
// or edit of a file in a language the harness has a checker for, the checker
// runs through the same shell tool and its one line rides on the change's own
// result, before the tests run, so a file that does not parse is never tested.

// TheFileParses and TheFileDoesNotParse open the checker's line on a result.
const (
	TheFileParses       = "parses"
	TheFileDoesNotParse = "does not parse: "
)

// theCheckers are the languages' own checkers, by file extension. Each takes
// the file's path in single quotes where the marker stands. TypeScript is
// left out, because its checker needs the project's settings and takes
// seconds, and Go's is gofmt in error mode, which reports syntax and nothing
// else.
var theCheckers = map[string]string{
	".js":  "node --check %s",
	".mjs": "node --check %s",
	".cjs": "node --check %s",
	".py":  "python3 -m py_compile %s",
	".go":  "gofmt -e %s > /dev/null",
}

// theWordsOfAnError are what a checker's telling line carries, in the order
// they are looked for, so that the one line the model reads is the error and
// not the caret under it.
var theWordsOfAnError = []string{"SyntaxError", "Error", "error"}

// checkTheSyntaxAfter runs the checker for the file a write or an edit that
// worked changed, and hands back the line to put on the change's result: one
// word when it parses, the checker's telling line when it does not, and
// nothing when there is no checker for the file or no shell tool.
func (running *run) checkTheSyntaxAfter(ctx context.Context, call contract.ToolCall, failed bool) string {
	if failed || (call.Name != contract.ToolWrite && call.Name != contract.ToolEdit) {
		return ""
	}
	path := fieldOfCall(call, "path")
	checker, known := theCheckers[strings.ToLower(filepath.Ext(path))]
	if !known {
		return ""
	}
	shell, found := running.tools().Lookup(contract.ToolShell)
	if !found {
		return ""
	}
	command := strings.Replace(checker, "%s", quotedForTheShell(wholePath(path)), 1)
	arguments, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		return ""
	}
	output, err := running.underTheTimeLimit(ctx, shell, contract.ToolCall{ID: call.ID + "-check", Name: contract.ToolShell, Input: arguments})
	if err != nil {
		return ""
	}
	if _, code := exitCodeIn(output.Text); code && !strings.Contains(firstLine(output.Text), "exit code 0") {
		return TheFileDoesNotParse + theTellingLineOf(output.Text)
	}
	return TheFileParses
}

// wholePath reads a path the way the file tools do: one that begins with a
// tilde is under the user's home.
func wholePath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// quotedForTheShell puts a path in single quotes, with any single quote in it
// closed, escaped and opened again, so a folder such as "Tater Tots Tetrisv1"
// is one argument.
func quotedForTheShell(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

// theTellingLineOf is the one line of a checker's output worth the model's
// reading: the first that names an error, or the last line that says anything.
func theTellingLineOf(text string) string {
	lines := strings.Split(text, "\n")
	for _, word := range theWordsOfAnError {
		for _, line := range lines {
			if strings.Contains(line, word) && !strings.HasPrefix(strings.TrimSpace(line), "at ") {
				return strings.TrimSpace(line)
			}
		}
	}
	for at := len(lines) - 1; at >= 0; at-- {
		if trimmed := strings.TrimSpace(lines[at]); trimmed != "" && !strings.HasPrefix(trimmed, "exit") {
			return trimmed
		}
	}
	return "the checker said nothing"
}
