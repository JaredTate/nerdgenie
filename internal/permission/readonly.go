// A command whose readable form is not the whole story is put to the user, even
// when it only reads, and in one benchmark run that stopped the agent six times
// on a long "node -e" command that only inspected files. This file is the one
// exception: a command the reducer could not read to the end, or that works out
// part of itself while it runs, runs without a preview when every program it
// runs is a reader from the small list below. The idea of a fixed list of
// programs that only read is Coeus's own; nothing was borrowed for it.

package permission

import (
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// readerPrograms are the programs that only read: they print something, or look
// at a file, or do nothing at all, and none of them writes, deletes, or spends.
// "find", "node" and "env" are readers only in some shapes, so they are ruled on
// by the three functions below rather than by being in this list.
var readerPrograms = map[string]bool{
	"pwd":   true,
	"ls":    true,
	"cat":   true,
	"head":  true,
	"tail":  true,
	"wc":    true,
	"echo":  true,
	"grep":  true,
	"stat":  true,
	"file":  true,
	"which": true,
	"true":  true,
}

// findWritingPrimaries are the "find" primaries that do more than look: they
// delete what they find, run another program over it, or write a file. A "find"
// carrying any of them is not a reader.
var findWritingPrimaries = map[string]bool{
	"-delete":  true,
	"-exec":    true,
	"-execdir": true,
	"-ok":      true,
	"-okdir":   true,
	"-fprint":  true,
	"-fprint0": true,
	"-fprintf": true,
	"-fls":     true,
}

// nodeReaderModes are the "node" flags that make it evaluate or check a script
// handed on the command line rather than run a file: "-e" and its long spelling
// evaluate the next word, "--test" runs test files, and "--check" only checks
// that a file parses. A "node" that runs a file could do anything, so it is not
// a reader.
var nodeReaderModes = map[string]bool{
	"-e":      true,
	"--eval":  true,
	"--test":  true,
	"--check": true,
}

// commandOnlyReads says whether the shell command in this call is one the
// permission function may run without a preview even though its readable form is
// not the whole story. It is false for anything but a shell command, and false
// when the command carries no text to read.
func commandOnlyReads(request contract.PermissionRequest) bool {
	if request.ToolName != contract.ToolShell {
		return false
	}
	command, written := stringField(readFields(request.Input), "command")
	if !written {
		return false
	}
	return commandTextOnlyReads(command)
}

// commandTextOnlyReads says whether one line of shell only reads: every command
// on it runs a reader, the commands are joined only by the operators a shell
// chains them with, and nothing on the line writes a file or works part of
// itself out while it runs. When any of that is in doubt the answer is no,
// because a command put to the user by mistake is a bother and a command run by
// mistake is not.
func commandTextOnlyReads(command string) bool {
	if writesAFileOrBuildsItself(command) {
		return false
	}
	segments, note := commandWords(command)
	// A quote that never closes leaves where one word ends and the next begins a
	// guess, so the program names cannot be trusted.
	if note == unclosedQuoteNote || note == buildsItselfNote {
		return false
	}
	// The reader stops at its caps and says so. A stop at the number of separate
	// commands or the length of the whole line may hide a command that is not a
	// reader, so it is put to the user; a stop inside one command's own words
	// hides only arguments, and the program is still known.
	if note == cutShortNote && (len(command) >= maxCommandBytes || len(segments) >= maxSegments) {
		return false
	}
	if len(segments) == 0 {
		return false
	}
	for _, words := range segments {
		if !oneCommandOnlyReads(words) {
			return false
		}
	}
	return true
}

// oneCommandOnlyReads says whether one command on the line runs a reader. The
// environment assignments a shell sets before the program are dropped first, so
// that "FOO=bar cat x" is read as the "cat" it runs.
func oneCommandOnlyReads(words []string) bool {
	words = withoutEnvironmentAssignments(words)
	if len(words) == 0 {
		return false
	}
	program := programName(words[0])
	switch program {
	case "find":
		return findOnlyReads(words[1:])
	case "node":
		return nodeOnlyReads(words[1:])
	case "env":
		return envOnlyReads(words[1:])
	default:
		return readerPrograms[program]
	}
}

// findOnlyReads says whether a "find" only looks. A "find" that deletes, runs
// another program, or writes a file is not a reader, whatever else it does.
func findOnlyReads(arguments []string) bool {
	for _, word := range arguments {
		name, _, _ := strings.Cut(word, "=")
		if findWritingPrimaries[name] {
			return false
		}
	}
	return true
}

// nodeOnlyReads says whether a "node" evaluates or checks a script rather than
// running a file. It is a reader when one of the reader-mode flags comes before
// any word that is not a flag, because that first plain word would be the file
// node runs, and a file node runs could do anything.
func nodeOnlyReads(arguments []string) bool {
	for _, word := range arguments {
		if nodeReaderModes[word] {
			return true
		}
		if !isFlag(word) {
			return false
		}
	}
	return false
}

// envOnlyReads says whether an "env" only prints the environment. It is a reader
// only when it sets nothing and runs nothing: an assignment is a write of a
// variable and a plain word is a program env would run, which env cannot vouch
// for. An "env" that runs another program is put to the user rather than read
// through here.
func envOnlyReads(arguments []string) bool {
	for _, word := range arguments {
		if !isFlag(word) {
			return false
		}
	}
	return true
}

// writesAFileOrBuildsItself reads one line of shell with the shell's own quoting
// rules and says whether it sends output to a file or works part of itself out
// while it runs. An output redirection ("cat x > out") writes a file; a command
// substitution ("$(...)", a backtick, or a process substitution "<(...)") runs a
// command nothing here can read. Either one means the readable programs are not
// the whole story, so the command is never treated as read only. A ">" or a
// "$(" written inside single quotes is a plain character to the shell and is not
// counted, which is why the quoting is followed rather than the raw text
// searched.
func writesAFileOrBuildsItself(command string) bool {
	var quote rune // 0 for none, '\'' inside single quotes, '"' inside double quotes.
	escaped := false
	var previous rune
	for _, letter := range command {
		switch {
		case escaped:
			escaped = false
		case quote == '\'':
			if letter == '\'' {
				quote = 0
			}
		case letter == '\\':
			escaped = true
		case quote == '"':
			// Inside double quotes a redirection is a plain character, but a
			// command substitution still runs.
			switch {
			case letter == '`':
				return true
			case letter == '(' && previous == '$':
				return true
			case letter == '"':
				quote = 0
			}
		case letter == '\'' || letter == '"':
			quote = letter
		case letter == '`':
			return true
		case letter == '(' && (previous == '$' || previous == '<' || previous == '>'):
			return true
		case letter == '>':
			return true
		}
		previous = letter
	}
	return false
}
