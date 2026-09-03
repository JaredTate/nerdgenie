// The restart guard is Hermes' design, at
// ~/Code/hermes-agent/cron/lifecycle_guard.py, where work is refused at the
// moment it is written down rather than at the moment it runs, and the match is
// anchored on a command rather than on a word, so that prose about restarting
// something is not mistaken for a command that restarts it. The Go here is
// written fresh and holds only the fixed list this agent needs.

package job

import (
	"fmt"
	"slices"
	"strings"
)

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
	// of a command. A program that needs neither a verb nor a target is a common
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

// leadingWords are the words that stand in front of a command without changing
// which command it is, so the guard steps over them before reading the program.
var leadingWords = []string{"sudo", "doas", "env", "nohup", "setsid", "time", "exec"}

// segmentBreaks are the characters that end one command and begin the next, so
// that the guard reads "make the coffee; reboot" as two commands and finds the
// second one.
const segmentBreaks = ";|&\n\r()`"

// checkItCannotRestartTheAgent refuses work that would stop or restart the
// agent. It is checked when a job or a task is written down, because that is
// where there is somebody to tell.
func checkItCannotRestartTheAgent(text string) error {
	for _, segment := range commandSegments(text) {
		tokens := commandTokens(segment)
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
// however it is worded. A program that needs neither has to be the first word,
// because it is an ordinary English word as well.
func matchesRefusedWork(tokens []string, refused refusal) bool {
	at := slices.Index(tokens, refused.program)
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
// and backslashes that only split a word taken out, and with the words that
// stand in front of a command stepped over.
func commandTokens(segment string) []string {
	plain := strings.NewReplacer(`"`, "", `'`, "", `\`, "", ",", " ", "[", " ", "]", " ").Replace(segment)
	tokens := strings.Fields(strings.ToLower(plain))
	for len(tokens) > 0 {
		first := tokens[0]
		if !slices.Contains(leadingWords, first) && !strings.Contains(first, "=") {
			break
		}
		tokens = tokens[1:]
	}
	return tokens
}
