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
