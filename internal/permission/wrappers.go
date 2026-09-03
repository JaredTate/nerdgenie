// The programs whose one job is to run another program. This is the same idea as
// the shell handed a script in nestedshell.go, which is OpenCode's prefix table
// at ~/Code/opencode/packages/opencode/src/permission/arity.ts carried one step
// further: a command written behind "nohup", "timeout 60" or "xargs" is still
// that command, and a reducer that stops at the wrapper rules on a program that
// deletes nothing while the one that deletes everything runs.

package permission

import (
	"slices"
	"strings"
)

// maxWrappers is how many programs that run another program the reducer reads
// through before it gives up. Four is more than any command a person writes
// needs, and a fifth is read as a call that does not say what it will do, so it
// goes to the user rather than through.
const maxWrappers = 4

// wrapper says how to read past one program that runs another: the flags of its
// own whose value is the next word, how many words that are not flags stand
// between it and the command it runs, and whether its name belongs in the
// readable form.
type wrapper struct {
	// flagsWithAValue are the wrapper's own flags that take the next word.
	flagsWithAValue []string
	// wordsBeforeTheCommand is how many words that are not flags belong to the
	// wrapper itself, as the duration in "timeout 60 rm -rf" does.
	wordsBeforeTheCommand int
	// namedInTheForm says whether the wrapper's own name is kept in front of the
	// readable form, so that the user and the log see how the command was run.
	namedInTheForm bool
}

// wrapperPrograms are the programs that run another program. Every one of them
// was a way of writing a recursive delete that the ask-me-first list never saw.
// "env" is the one whose name is left out of the readable form, because setting
// variables and running a program is still running that program, and the
// variables are arguments that change every time.
var wrapperPrograms = map[string]wrapper{
	"busybox": {namedInTheForm: true},
	"doas":    {flagsWithAValue: []string{"-u", "-C", "-a"}, namedInTheForm: true},
	"env":     {flagsWithAValue: []string{"-u", "--unset", "-C", "--chdir", "-S", "--split-string"}},
	"nice":    {flagsWithAValue: []string{"-n", "--adjustment"}, namedInTheForm: true},
	"nohup":   {namedInTheForm: true},
	"setsid":  {namedInTheForm: true},
	"stdbuf":  {flagsWithAValue: []string{"-i", "-o", "-e", "--input", "--output", "--error"}, namedInTheForm: true},
	"timeout": {flagsWithAValue: []string{"-s", "--signal", "-k", "--kill-after"}, wordsBeforeTheCommand: 1, namedInTheForm: true},
	"xargs": {flagsWithAValue: []string{
		"-n", "-I", "-i", "-d", "-E", "-L", "-P", "-s", "-a",
		"--max-args", "--replace", "--delimiter", "--eof", "--max-lines", "--max-procs", "--max-chars", "--arg-file",
	}, namedInTheForm: true},
}

// pastTheWrappers reads through the programs that run another program, sudo
// among them, and returns the names to keep in front of the readable form, the
// words of the command they wrap, and the note saying the wrappers were piled
// deeper than the reducer reads. A call carrying that note goes to the user,
// because a form that stops at a wrapper cannot be ruled on.
func pastTheWrappers(words []string) (string, []string, string) {
	prefix := ""
	for round := 0; round < maxWrappers; round++ {
		if len(words) == 0 {
			return prefix, words, ""
		}
		if programName(words[0]) == sudoProgram {
			prefix += sudoProgram + " "
			words = withoutEnvironmentAssignments(withoutSudoFlags(words[1:]))
			continue
		}
		runner, wraps := wrapperPrograms[programName(words[0])]
		if !wraps {
			return prefix, words, ""
		}
		behind, found := theCommandBehindTheWrapper(runner, words[1:])
		if !found {
			return prefix, words, ""
		}
		if runner.namedInTheForm {
			prefix += programName(words[0]) + " "
		}
		words = withoutEnvironmentAssignments(behind)
	}
	return prefix, words, cutShortNote
}

// theCommandBehindTheWrapper returns the words of the command one wrapper runs,
// with the wrapper's own flags and the words that belong to the wrapper itself
// left behind, and says whether there is a command there at all.
func theCommandBehindTheWrapper(runner wrapper, words []string) ([]string, bool) {
	taken := 0
	for index := 0; index < len(words); index++ {
		if isFlag(words[index]) {
			name, _, writtenWithAnEquals := strings.Cut(words[index], "=")
			if !writtenWithAnEquals && slices.Contains(runner.flagsWithAValue, name) {
				index++
			}
			continue
		}
		if taken < runner.wordsBeforeTheCommand {
			taken++
			continue
		}
		return words[index:], true
	}
	return nil, false
}
