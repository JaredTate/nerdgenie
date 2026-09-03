// The restart guard is Hermes' design, at
// ~/Code/hermes-agent/cron/lifecycle_guard.py, where work is refused at the
// moment it is written down rather than at the moment it runs, and the match is
// anchored on a command rather than on a word, so that prose about restarting
// something is not mistaken for a command that restarts it. Reading a program by
// the last part of its path, and reading the script a shell was handed as a
// command line of its own, is the design of this agent's own permission
// reducer, at internal/permission/reduce.go and internal/permission/nestedshell.go,
// because a guard that can be walked past by writing /sbin/reboot is no guard.
// The Go here is written fresh and holds only the fixed list this agent needs.

package job

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

// maxWrappingShells is how many shells may be wrapped inside one another before
// the guard stops reading and refuses the work. Three is more than any command a
// person writes needs, and a fourth is text that does not say what it will run.
const maxWrappingShells = 3

// refusal is one shape of work a job may never carry: a program, the words that
// make it stop something, and the thing it would stop.
type refusal struct {
	// program is the command that does the stopping.
	program string
	// verbs are the words that turn the program into one that stops something.
	// An empty list means the program stops something whatever follows it.
	verbs []string
	// target is the word a later token must hold, and is empty when the program
	// needs no target to be dangerous.
	target string
	// mustLeadTheCommand says the program only counts when it is the first word
	// of a command, once the words that merely wrap the command have been
	// stepped over. A program that needs neither a verb nor a target is a common
	// English word as well, and "the reboot of the franchise" is a blog piece
	// rather than an order to restart the machine.
	mustLeadTheCommand bool
}

// refusedWork is the fixed list of work that would stop or restart the agent. A
// job that could restart the agent would start itself again on the way up, and
// the agent would spend its life coming back from the dead.
var refusedWork = []refusal{
	{program: "systemctl", verbs: []string{"restart", "stop", "kill", "disable", "mask", "reload"}, target: "coeus"},
	{program: "coeus", verbs: []string{"restart", "stop", "uninstall", "update", "install"}},
	{program: "pkill", target: "coeus"},
	{program: "killall", target: "coeus"},
	{program: "kill", target: "coeus"},
	{program: "reboot", mustLeadTheCommand: true},
	{program: "shutdown", mustLeadTheCommand: true},
	{program: "halt", mustLeadTheCommand: true},
	{program: "poweroff", mustLeadTheCommand: true},
}

// wrapper is a program that runs another program, so the guard steps over it and
// reads the one behind it. "timeout 5 reboot" restarts the machine just as
// surely as "reboot" does.
type wrapper struct {
	// name is the wrapping program, by the last part of its path.
	name string
	// flagsWithAValue are its flags that take the next word as their value, so
	// that the value is stepped over too and is never read as the program.
	flagsWithAValue []string
	// ownWords is how many words after its flags belong to the wrapper itself,
	// such as the duration in "timeout 5 reboot".
	ownWords int
}

// wrappers are the programs that stand in front of a command without changing
// which command it is.
var wrappers = []wrapper{
	{name: "sudo", flagsWithAValue: []string{"-u", "-g", "-p", "-C", "-h", "-U", "-r", "-t",
		"--user", "--group", "--prompt", "--host", "--role", "--type"}},
	{name: "doas", flagsWithAValue: []string{"-u", "-C"}},
	{name: "env"},
	{name: "nohup"},
	{name: "setsid"},
	{name: "time"},
	{name: "exec"},
	{name: "nice", flagsWithAValue: []string{"-n", "--adjustment"}},
	{name: "ionice", flagsWithAValue: []string{"-c", "-n", "--class", "--classdata"}},
	{name: "stdbuf", flagsWithAValue: []string{"-i", "-o", "-e"}},
	{name: "timeout", flagsWithAValue: []string{"-k", "-s", "--kill-after", "--signal"}, ownWords: 1},
	{name: "xargs", flagsWithAValue: []string{"-n", "-P", "-I", "-d", "-s", "-a", "-E", "-L"}},
}

// shellPrograms are the programs that take a script as an argument, so that what
// follows their -c flag is a command line and has to be read as one.
var shellPrograms = []string{"sh", "bash", "zsh", "dash", "ksh"}

// segmentBreaks are the characters that end one command and begin the next, so
// that the guard reads "make the coffee; reboot" as two commands and finds the
// second one.
const segmentBreaks = ";|&\n\r()`"

// checkItCannotRestartTheAgent refuses work that would stop or restart the
// agent. It is checked when a job or a task is written down, because that is
// where there is somebody to tell.
func checkItCannotRestartTheAgent(text string) error {
	for _, segment := range commandSegments(text) {
		tokens, plain := commandTokens(segment)
		if !plain {
			return errors.New("this work wraps a command inside more than three shells, so it does not say what it would run; write the command out plainly instead")
		}
		if len(tokens) == 0 {
			continue
		}
		for _, refused := range refusedWork {
			if !matchesRefusedWork(tokens, refused) {
				continue
			}
			return fmt.Errorf("this work would run %q, which stops or restarts the agent, and a job that restarts the agent starts itself again, so take that command out of it",
				refused.program)
		}
	}
	return nil
}

// matchesRefusedWork says whether one command is the shape of work being
// refused: the program, then one of its verbs, then the thing it would stop.
//
// A program that carries a verb and a target is looked for anywhere in the
// command, because "run systemctl restart coeus every morning" is an order
// however it is worded. A program that needs neither has to lead the command,
// because it is an ordinary English word as well. Every token is read by the
// last part of its path, so "/sbin/reboot" is reboot.
func matchesRefusedWork(tokens []string, refused refusal) bool {
	at := slices.IndexFunc(tokens, func(token string) bool { return programName(token) == refused.program })
	if at < 0 || (refused.mustLeadTheCommand && at != 0) {
		return false
	}
	rest := tokens[at+1:]
	if len(refused.verbs) > 0 {
		at := slices.IndexFunc(rest, func(token string) bool { return slices.Contains(refused.verbs, token) })
		if at < 0 {
			return false
		}
		rest = rest[at+1:]
	}
	if refused.target == "" {
		return true
	}
	for _, token := range rest {
		if strings.Contains(token, refused.target) {
			return true
		}
	}
	return false
}

// commandSegments cuts a piece of text into the commands inside it, so that a
// command hidden behind a semicolon is read on its own.
func commandSegments(text string) []string {
	// A line continued with a backslash is one command, so the break is taken
	// out before the text is cut up.
	joined := strings.ReplaceAll(text, "\\\n", " ")
	return strings.FieldsFunc(joined, func(letter rune) bool {
		return strings.ContainsRune(segmentBreaks, letter)
	})
}

// commandTokens reads one command as the words it is made of, with the quotes
// and backslashes that only split a word taken out, and stepped past the words
// that merely wrap the command. It says false when the command is wrapped in
// more shells than the guard reads, because such a command says nothing about
// what it would run.
func commandTokens(segment string) ([]string, bool) {
	plain := strings.NewReplacer(`"`, "", `'`, "", `\`, "", ",", " ", "[", " ", "]", " ").Replace(segment)
	return stepPastTheWrappers(strings.Fields(strings.ToLower(plain)), 0)
}

// stepPastTheWrappers drops the words in front of a command that do not change
// which command it is: an environment setting, a wrapping program with its own
// flags and words, and a shell that was handed a script, whose script is read as
// a command line of its own.
func stepPastTheWrappers(tokens []string, shellsSoFar int) ([]string, bool) {
	if shellsSoFar > maxWrappingShells {
		return nil, false
	}
	for len(tokens) > 0 {
		first := programName(tokens[0])
		switch {
		case isEnvironmentAssignment(tokens[0]):
			tokens = tokens[1:]
		case slices.Contains(shellPrograms, first):
			script, handed := scriptHandedToAShell(tokens[1:])
			if !handed {
				return tokens, true
			}
			return stepPastTheWrappers(script, shellsSoFar+1)
		default:
			stepped, wrapped := withoutTheWrapper(tokens, first)
			if !wrapped {
				return tokens, true
			}
			tokens = stepped
		}
	}
	return tokens, true
}

// withoutTheWrapper drops one wrapping program, its own flags, the values those
// flags take, and the words that belong to the wrapper itself, and says whether
// the first token was a wrapper at all.
func withoutTheWrapper(tokens []string, name string) ([]string, bool) {
	at := slices.IndexFunc(wrappers, func(known wrapper) bool { return known.name == name })
	if at < 0 {
		return tokens, false
	}
	stepping := wrappers[at]
	rest := tokens[1:]
	for len(rest) > 0 && isFlag(rest[0]) {
		if slices.Contains(stepping.flagsWithAValue, rest[0]) && len(rest) > 1 {
			rest = rest[2:]
			continue
		}
		rest = rest[1:]
	}
	for range stepping.ownWords {
		if len(rest) == 0 {
			break
		}
		rest = rest[1:]
	}
	return rest, true
}

// scriptHandedToAShell returns the script a shell was handed, which follows the
// short flag that hands one over. The flag may carry other letters with it,
// because "bash -lc" is how a login shell is written and it hands over a script
// just the same.
func scriptHandedToAShell(afterTheShell []string) ([]string, bool) {
	for at, token := range afterTheShell {
		if handsOverAScript(token) && at+1 < len(afterTheShell) {
			return afterTheShell[at+1:], true
		}
	}
	return nil, false
}

// handsOverAScript says whether the word is the short flag that hands a shell a
// script, on its own or with other short flags beside it.
func handsOverAScript(word string) bool {
	if len(word) < 2 || !strings.HasPrefix(word, "-") || strings.HasPrefix(word, "--") {
		return false
	}
	return strings.ContainsRune(word[1:], 'c')
}

// programName is the name of the program a word names, without the folders it
// was written with, because "/sbin/reboot" is reboot just as much as "reboot" is.
func programName(word string) string {
	return filepath.Base(word)
}

// isFlag says whether a word is a flag rather than something to act on. A lone
// dash is the standard input, not a flag.
func isFlag(word string) bool {
	return len(word) > 1 && strings.HasPrefix(word, "-")
}

// isEnvironmentAssignment says whether a word sets an environment variable, as
// in "PATH=/usr/bin", which stands in front of a command without being one.
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
