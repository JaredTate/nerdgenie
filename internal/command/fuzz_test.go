package command_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/command"
)

// FuzzTheLineSplitterNeverPanics throws arbitrary text at the splitter that
// reads a command's name off a typed line. Everything a user or a channel can
// send arrives here first, so a crash in it would take the whole program down
// on one strange message.
//
// Whatever comes back has to obey three rules: the name holds no space and no
// slash, so it can never be looked up as something it is not; both halves are
// really in the line that was typed; and a line with a name in it never comes
// back with an empty one.
func FuzzTheLineSplitterNeverPanics(f *testing.F) {
	f.Add("/status")
	f.Add("status")
	f.Add("  /model   claude  ")
	f.Add("/deny 3 the post is wrong")
	f.Add("")
	f.Add("/")
	f.Add("//")
	f.Add("/ ")
	f.Add("\t\n\r ")
	f.Add("/tasks\t17 back 3")
	f.Add("/\x00status")
	f.Add(strings.Repeat("/", 4096))
	f.Add("/ünïcödé argument")
	f.Add("/status\n/model")

	f.Fuzz(func(t *testing.T, line string) {
		name, arguments := command.SplitLine(line)

		if strings.ContainsAny(name, " \t\r\n") {
			t.Errorf("the name %q holds a space, so it is not one word: line %q", name, line)
		}
		if strings.HasPrefix(name, "/") {
			t.Errorf("the name %q still carries its slash: line %q", name, line)
		}
		if name != "" && !strings.Contains(line, name) {
			t.Errorf("the name %q is not in the line %q at all", name, line)
		}
		if arguments != "" && !strings.Contains(line, arguments) {
			t.Errorf("the arguments %q are not in the line %q at all", arguments, line)
		}
		if name == "" && arguments != "" {
			t.Errorf("the line %q came back with arguments %q and no command to give them to", line, arguments)
		}
		if name == "" && strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "/")) != "" {
			t.Errorf("the line %q holds a command and came back with none", line)
		}
	})
}
