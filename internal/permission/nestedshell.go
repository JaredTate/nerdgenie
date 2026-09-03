// A shell handed a script runs a command line of its own, so the script is read
// as one. This is the same idea as OpenCode's prefix table at
// ~/Code/opencode/packages/opencode/src/permission/arity.ts, carried one step
// further: a program whose one argument is a command line has to be read as a
// command line, or "sh -c" becomes a way of writing anything at all and having
// the ask-me-first list see none of it.

package permission

import (
	"path/filepath"
	"strings"
)

// maxNestedShells is how many shells may be wrapped inside one another before
// the reducer stops reading. Three is more than any command a person writes
// needs, and a fourth is read as a call that does not say what it will do.
const maxNestedShells = 3

// shellPrograms are the programs that take a script as an argument, so that what
// follows their -c flag is a command line and not an argument that varies.
var shellPrograms = []string{"sh", "bash", "zsh", "dash"}

// programName is the name of the program a word names, without the folders it
// was written with, because "/usr/bin/sudo" is sudo just as much as "sudo" is.
func programName(word string) string {
	return filepath.Base(word)
}

// scriptHandedToAShell says whether these words are a shell being handed a
// script to run, and gives back the flag that hands it over and the script
// itself. The flag may carry other letters with it, because "bash -lc" is how a
// login shell is written and it hands over a script just the same.
func scriptHandedToAShell(words []string) (string, string, bool) {
	if !isAShell(programName(words[0])) {
		return "", "", false
	}
	for index := 1; index < len(words)-1; index++ {
		if handsOverAScript(words[index]) {
			return words[index], words[index+1], true
		}
	}
	return "", "", false
}

// isAShell says whether the program is one of the shells that take a script.
func isAShell(name string) bool {
	for _, shell := range shellPrograms {
		if name == shell {
			return true
		}
	}
	return false
}

// handsOverAScript says whether the word is the short flag that hands a shell a
// script, on its own or with other short flags beside it.
func handsOverAScript(word string) bool {
	if len(word) < 2 || !strings.HasPrefix(word, "-") || strings.HasPrefix(word, "--") {
		return false
	}
	return strings.ContainsRune(word[1:], 'c')
}

// reduceNestedShell reduces the script a shell was handed as a command line of
// its own, because a delete written inside the quotes of "sh -c" deletes just as
// surely as one written on its own. A nest deeper than the cap is read as a call
// that does not say what it will do.
func reduceNestedShell(head string, script string, depth int) (string, string) {
	if depth >= maxNestedShells {
		return head, cutShortNote
	}
	inside, note := reduceCommandLine(script, depth+1)
	if inside == "" {
		return head, note
	}
	return head + " " + inside, note
}
