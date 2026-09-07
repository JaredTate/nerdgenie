package permission_test

// The five ways a reviewer got a deleting command past the ask-me-first list at
// the wave 1 gate. Every probe in this file is written the way the reviewer ran
// it, and every one of them really deletes, really asks for administrator
// powers, or really works out what it will run while it runs.

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/permission"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// evasions are the reviewer's own probes: a command line long enough to push the
// delete past the cap on the readable form, a delete handed to a nested shell in
// its two common spellings, sudo written as the full path to the program, and a
// program a command substitution works out while the line runs.
var evasions = []struct {
	name    string
	command string
}{
	{"a long prefix that pushes the delete past the cap", strings.Repeat("echo hello; ", 40) + "rm -rf /home/user/nerdgenie"},
	{"a delete handed to a nested shell", `sh -c "rm -rf ~"`},
	{"a delete handed to a nested login shell", `bash -lc 'rm -rf ~'`},
	{"sudo written as the full path to the program", "/usr/bin/sudo -u root rm -rf /home/user/nerdgenie"},
	{"a program a command substitution works out while it runs", "$(printf rm) -rf /home/user/nerdgenie"},
}

func TestNoEvasionTheGateReviewFoundRunsWithoutAsking(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	for _, evasion := range evasions {
		decision := decide(t, decider, shellRequest(t, evasion.command))
		if decision.Ruling == contract.RulingAllow {
			t.Errorf("%s: %q was ruled %q, and a call that really deletes may not run without a yes", evasion.name, evasion.command, decision.Ruling)
		}
	}
}

// nestedShells are the shells handed a script, and the readable form each one
// has to come back with, because a script handed to a shell is a command line in
// its own right and not an argument that varies.
var nestedShells = []struct {
	command string
	reduced string
}{
	{`sh -c "rm -rf ~"`, "sh -c rm -rf"},
	{`bash -lc 'rm -rf ~'`, "bash -lc rm -rf"},
	{`/bin/bash -c "git reset --hard origin/main"`, "/bin/bash -c git reset --hard"},
	{`zsh -c 'echo hello | rm -r /tmp/x'`, "zsh -c echo | rm -r"},
	{`dash -c 'ls -la'`, "dash -c ls -la"},
	{`bash --norc -c 'rm -rf /tmp/x'`, "bash -c rm -rf"},
	{`sh - -c 'rm -rf /tmp/x'`, "sh -c rm -rf"},
	{`sh -c ""`, "sh -c"},
	{"sh -c", "sh -c"},
	{"sh script.sh", "sh"},
}

func TestAScriptHandedToAShellIsReducedAsACommandOfItsOwn(t *testing.T) {
	for _, nested := range nestedShells {
		if reduced := permission.Reduce(shellRequest(t, nested.command)); reduced != nested.reduced {
			t.Errorf("the readable form of %q is %q, want %q", nested.command, reduced, nested.reduced)
		}
	}
}

func TestADeleteHandedToANestedShellIsCaughtByTheList(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	for _, command := range []string{`sh -c "rm -rf ~"`, `bash -lc 'rm -rf ~'`, `zsh -c 'git clean -f -d'`} {
		decision := decide(t, decider, shellRequest(t, command))
		if decision.Ruling != contract.RulingAsk {
			t.Errorf("%q was ruled %q, want %q", command, decision.Ruling, contract.RulingAsk)
		}
		if decision.Reason != contract.AskFirstBulkDelete {
			t.Errorf("%q was caught by %q, want the entry %q", command, decision.Reason, contract.AskFirstBulkDelete)
		}
	}
}

func TestAShellNestedDeeperThanTheCapSaysSoAndAsks(t *testing.T) {
	command := "rm -rf /tmp/x"
	for wrapping := 0; wrapping < 5; wrapping++ {
		command = "sh -c '" + strings.ReplaceAll(command, "'", `'\''`) + "'"
	}

	reduced := permission.Reduce(shellRequest(t, command))
	if !strings.HasSuffix(reduced, "(cut short before the end)") {
		t.Errorf("the readable form of five nested shells is %q, and the reducer has to say where it stopped", reduced)
	}

	decision := decide(t, newDecider(t, contract.DefaultConfig()), shellRequest(t, command))
	if decision.Ruling != contract.RulingAsk {
		t.Errorf("a delete under five nested shells was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
}

// programsWrittenAsAFullPath are the commands whose program is written with the
// folders it lives in, which is the same program under a longer name.
var programsWrittenAsAFullPath = []struct {
	command string
	reduced string
}{
	{"/usr/bin/sudo -u root rm -rf /home/user/nerdgenie", "sudo rm -rf"},
	{"/usr/bin/sudo apt install ripgrep", "sudo apt install"},
	{`/usr/bin/git commit -m "a change"`, "/usr/bin/git commit"},
	{"/bin/rm -rf /tmp/x", "/bin/rm -rf"},
}

func TestAProgramIsTheSameProgramWhenItIsWrittenAsAFullPath(t *testing.T) {
	for _, written := range programsWrittenAsAFullPath {
		if reduced := permission.Reduce(shellRequest(t, written.command)); reduced != written.reduced {
			t.Errorf("the readable form of %q is %q, want %q", written.command, reduced, written.reduced)
		}
	}
}

func TestSudoWrittenAsAFullPathIsStillCaughtBySudo(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	decision := decide(t, decider, shellRequest(t, "/usr/bin/sudo -u root rm -rf /home/user/nerdgenie"))
	if decision.Ruling != contract.RulingAsk {
		t.Fatalf("sudo written as a full path was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
	if decision.Reason != contract.AskFirstSudo && decision.Reason != contract.AskFirstBulkDelete {
		t.Errorf("the reason is %q, want one of the two entries that cover it", decision.Reason)
	}
}

// tooLongToRead are the four caps the word reader keeps, each reached by a
// command line that hides a delete behind it. None of them may drop the delete
// quietly.
var tooLongToRead = []struct {
	name    string
	command string
}{
	{"more separate commands than the reader takes", strings.Repeat("echo hello; ", 100) + "rm -rf /tmp/x"},
	{"more words in one command than the reader takes", "echo " + strings.Repeat("hello ", 400) + "&& rm -rf /tmp/x"},
	{"a word longer than the reader takes", "echo " + strings.Repeat("x", 600) + " && rm -rf /tmp/x"},
	{"a line longer than the reader takes", strings.Repeat("echo hello; ", 900) + "rm -rf /tmp/x"},
	{"a readable form longer than the cap", strings.Repeat("echo hello; ", 40) + "rm -rf /home/user/nerdgenie"},
}

func TestACallTooLongToReadToTheEndSaysSoAndAsks(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	for _, long := range tooLongToRead {
		reduced := permission.Reduce(shellRequest(t, long.command))
		if !strings.HasSuffix(reduced, "(cut short before the end)") {
			t.Errorf("%s: the readable form is %q, and a form that stops before the end has to say so", long.name, reduced)
		}
		if decision := decide(t, decider, shellRequest(t, long.command)); decision.Ruling != contract.RulingAsk {
			t.Errorf("%s: the call was ruled %q, want %q", long.name, decision.Ruling, contract.RulingAsk)
		}
	}
}

// commandsThatBuildThemselves work out part of what they will run while they
// run, so nothing read beforehand can say what they will do.
var commandsThatBuildThemselves = []string{
	"$(printf rm) -rf /home/user/nerdgenie",
	"`printf rm` -rf /home/user/nerdgenie",
	`echo "$(rm -rf /tmp/x)"`,
	"diff <(ls /tmp) <(ls /var)",
}

func TestACommandThatBuildsItselfWhileItRunsSaysSoAndAsks(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	for _, command := range commandsThatBuildThemselves {
		reduced := permission.Reduce(shellRequest(t, command))
		if !strings.HasSuffix(reduced, "(builds part of itself at run time)") {
			t.Errorf("the readable form of %q is %q, and it has to say that the command builds itself", command, reduced)
		}
		if decision := decide(t, decider, shellRequest(t, command)); decision.Ruling != contract.RulingAsk {
			t.Errorf("%q was ruled %q, want %q", command, decision.Ruling, contract.RulingAsk)
		}
	}
}

// commandsWithAQuoteTheyNeverClose leave a quote open, so where one word ends
// and the next begins is a guess: everything after the quote, the characters
// that end one command and start another among them, is read as more of the same
// word. The fuzzer found the second of these by putting a subshell round the
// first, and the swallowed bracket turned "/sudo" into a program named "sudo )",
// which nothing on the ask-me-first list knows.
var commandsWithAQuoteTheyNeverClose = []string{
	`/sudo"`,
	`( /sudo" )`,
	`rm -rf '/tmp/unclosed`,
	`echo "hello`,
	`( rm -rf "/tmp/x )`,
}

func TestACommandWithAQuoteItNeverClosesSaysSoAndAsks(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	for _, command := range commandsWithAQuoteTheyNeverClose {
		reduced := permission.Reduce(shellRequest(t, command))
		if !strings.HasSuffix(reduced, "(a quote that is never closed)") {
			t.Errorf("the readable form of %q is %q, and it has to say that a quote was never closed", command, reduced)
		}
		if decision := decide(t, decider, shellRequest(t, command)); decision.Ruling != contract.RulingAsk {
			t.Errorf("%q was ruled %q, want %q, because nothing here can say where one word ends and the next begins",
				command, decision.Ruling, contract.RulingAsk)
		}
	}
}

// bracketsThatBuildNothing look like a substitution and are not one, so the
// permission function has to leave every one of them alone.
var bracketsThatBuildNothing = []string{
	`echo '$(rm -rf /tmp/x)'`,
	`echo "a (b) c"`,
	"( ls -la )",
	`sh -c 'ls -la'`,
	"/usr/bin/ls -la",
}

func TestBracketsThatBuildNothingStillRunOnTheirOwn(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	for _, command := range bracketsThatBuildNothing {
		if decision := decide(t, decider, shellRequest(t, command)); decision.Ruling != contract.RulingAllow {
			t.Errorf("%q was ruled %q, want %q, because it neither deletes nor builds itself", command, decision.Ruling, contract.RulingAllow)
		}
	}
}

// disguises are the wrappings a command has been hidden inside: the four the
// wave 1 gate review used, which are a nested shell, a pipe, a subshell and a
// long prefix, and then one for each program the wave 6 security review found
// that runs another program. Wrapping a command may never make it easier to run
// than the command on its own.
var disguises = []func(string) string{
	func(command string) string { return "sh -c '" + insideSingleQuotes(command) + "'" },
	func(command string) string { return "echo hello | " + command },
	func(command string) string { return "( " + command + " )" },
	func(command string) string { return strings.Repeat("echo hello; ", 40) + command },
	func(command string) string { return "nohup " + command },
	func(command string) string { return "timeout 60 " + command },
	func(command string) string { return "nice -n 10 " + command },
	func(command string) string { return "setsid " + command },
	func(command string) string { return "stdbuf -oL " + command },
	func(command string) string { return "busybox " + command },
	func(command string) string { return "env -i " + command },
	func(command string) string { return "doas " + command },
	func(command string) string { return "sudo -u nobody " + command },
	func(command string) string { return "su -c '" + insideSingleQuotes(command) + "'" },
	func(command string) string { return "echo /home/user/nerdgenie | xargs " + command },
}

// insideSingleQuotes writes a command so that a shell handed it between single
// quotes runs the command that went in, quotes and all.
func insideSingleQuotes(command string) string {
	return strings.ReplaceAll(command, "'", `'\''`)
}

// commandsRunThroughAWrapper hand the real command to a program that runs it,
// and they seed the fuzzer because a wrapper is the shape of disguise the
// reducer has to read through. The wave 6 security review wrote this table, and
// it lives here now because the review's own test file left the tree once every
// finding it named had a fix and a test of its own.
var commandsRunThroughAWrapper = []struct {
	name    string
	command string
}{
	{"a delete run under nohup", "nohup rm -rf /home/user/nerdgenie"},
	{"a delete given a time limit", "timeout 60 rm -rf /home/user/nerdgenie"},
	{"a delete run through xargs", "echo /home/user/nerdgenie | xargs rm -rf"},
	{"a delete run through busybox", "busybox rm -rf /home/user/nerdgenie"},
	{"a delete run through env with a flag", "env -i sh -c 'rm -rf /home/user/nerdgenie'"},
	{"a delete handed to su", "su -c 'rm -rf /home/user/nerdgenie'"},
}

func FuzzADisguisedCommandStillNeedsTheSameYes(f *testing.F) {
	for _, evasion := range evasions {
		for number := range disguises {
			f.Add(evasion.command, number)
		}
	}
	for _, call := range bulkDeleteCalls {
		if command, written := call.fields["command"].(string); written {
			for number := range disguises {
				f.Add(command, number)
			}
		}
	}
	for _, wrapped := range commandsRunThroughAWrapper {
		f.Add(wrapped.command, 0)
	}
	f.Add("sudo apt install ripgrep", 0)
	f.Add("find /tmp -name '*.log' -delete", 0)
	f.Add("rm --force --recursive /home/user/nerdgenie", 4)
	f.Add("git -C /tmp reset --hard", 5)

	decider, err := permission.New(contract.DefaultConfig(), testkit.NewFakeClock(theTestTime))
	if err != nil {
		f.Fatalf("building the permission function failed: %v", err)
	}

	f.Fuzz(func(t *testing.T, command string, number int) {
		if number < 0 || number >= len(disguises) {
			t.Skip("the fuzzer picked a disguise number that does not name one of the disguises")
		}
		if firstWord := strings.Fields(command); len(firstWord) > 0 && strings.HasPrefix(firstWord[0], "-") {
			t.Skip("a command line whose first word is a flag names no program to run, and a program that runs another program" +
				" would read that flag as one of its own, as xargs reads the -r in \"xargs -r rm\" and runs rm without it")
		}
		if decide(t, decider, shellRequest(t, command)).Ruling == contract.RulingAllow {
			return
		}
		disguised := disguises[number](command)
		if decide(t, decider, shellRequest(t, disguised)).Ruling == contract.RulingAllow {
			t.Fatalf("%q needs a yes, and hiding it as %q let it run without one", command, disguised)
		}
	})
}
