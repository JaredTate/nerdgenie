// The flags of a command, read as flags rather than as text. OpenCode's arity
// table at ~/Code/opencode/packages/opencode/src/permission/arity.ts reads a
// command line by its leading words; this file is the step that table needs
// before it can do that, because a flag that takes the next word swallows the
// subcommand, and a flag written in front of another one moves it.

package permission

import (
	"slices"
	"strings"
)

// flagsThatTakeAValue names, for each program the shape table knows, the flags
// whose value is the next word. They are taken out in pairs, because a value
// left behind is read as the subcommand: "git -C /tmp reset --hard" is a reset,
// and reading it as "git /tmp" hides the one part that throws work away.
var flagsThatTakeAValue = map[string][]string{
	"apt":       {"-o", "-c", "-t", "--option", "--config-file", "--target-release"},
	"apt-get":   {"-o", "-c", "-t", "--option", "--config-file", "--target-release"},
	"bun":       {"--cwd", "--config"},
	"cargo":     {"--manifest-path", "--config", "--color"},
	"docker":    {"-H", "-f", "--host", "--file", "--context", "--config", "--log-level"},
	"gh":        {"-R", "--repo"},
	"git":       {"-C", "-c", "--git-dir", "--work-tree", "--exec-path", "--namespace"},
	"go":        {"-C"},
	"kubectl":   {"-n", "-s", "--namespace", "--context", "--kubeconfig", "--cluster", "--server"},
	"make":      {"-C", "-f", "--directory", "--file", "--makefile"},
	"npm":       {"-w", "--prefix", "--workspace"},
	"pip":       {"--cache-dir", "--log", "--proxy"},
	"systemctl": {"-H", "-M", "-t", "--host", "--machine", "--type", "--property"},
	"sudo":      {"-u", "-g", "-p", "-C", "-h", "-U", "-r", "-t", "--user", "--group", "--prompt", "--host", "--role", "--type"},
	"yarn":      {"--cwd"},
}

// withoutTheValuesOfFlags takes out the flags of this program that take the next
// word, and that word with them, so that the value of a flag is never mistaken
// for the subcommand. A flag written with its value after an equals sign is one
// word, and that whole word goes.
func withoutTheValuesOfFlags(words []string) []string {
	if len(words) == 0 {
		return words
	}
	takeAValue, named := flagsThatTakeAValue[programName(words[0])]
	if !named {
		return words
	}

	kept := []string{words[0]}
	for index := 1; index < len(words); index++ {
		name, _, writtenWithAnEquals := strings.Cut(words[index], "=")
		if !slices.Contains(takeAValue, name) {
			kept = append(kept, words[index])
			continue
		}
		if !writtenWithAnEquals {
			index++
		}
	}
	return kept
}

// flagsSpelledOut rewrites a readable form as the second form the rules match
// on: the same commands, each with its flags moved to the end and written one to
// a pair of brackets, spelled out, in order and without repeats. Matching flags
// as text asks which letters were written next to which; matching this form asks
// only which flags the command was given, so "rm -v -rf" and "rm --force
// --recursive" answer the same question as "rm -rf" does. Nobody is shown this
// form; the readable form is still what the user reads and the log records.
func flagsSpelledOut(readable string) string {
	commands := strings.Split(readable, " | ")
	written := make([]string, 0, len(commands))
	for _, command := range commands {
		written = append(written, oneCommandWithItsFlagsSpelledOut(command))
	}
	return strings.Join(written, " | ")
}

// oneCommandWithItsFlagsSpelledOut writes one command of a readable form with
// its flags spelled out after the words that name the command.
func oneCommandWithItsFlagsSpelledOut(command string) string {
	named := []string{}
	flags := []string{}
	for _, word := range strings.Fields(command) {
		if isFlag(word) {
			flags = append(flags, spellingsOfAFlag(word)...)
			continue
		}
		named = append(named, word)
	}
	slices.Sort(flags)
	return strings.TrimSpace(strings.Join(named, " ") + " " + strings.Join(slices.Compact(flags), ""))
}

// spellingsOfAFlag returns the ways one flag word may be read: the flag as it
// was written, without any value written after an equals sign, and, when it is
// one dash with several letters, each of those letters on its own, because
// "-rf" is "-r" and "-f" and nothing tells that apart from find's "-delete",
// which is one flag with one dash. Reading a flag both ways can only make the
// harness ask about more calls, never about fewer.
func spellingsOfAFlag(word string) []string {
	name, _, _ := strings.Cut(word, "=")
	spellings := []string{"[" + name + "]"}
	if strings.HasPrefix(name, "--") {
		return spellings
	}
	for _, letter := range name[1:] {
		spellings = append(spellings, "[-"+string(letter)+"]")
	}
	return spellings
}
