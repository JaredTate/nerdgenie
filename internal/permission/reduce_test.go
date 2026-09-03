package permission_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/permission"
)

// shellReductions are the commands the reducer has to cut down and what it has
// to cut them down to. They cover quoted arguments, pipes, command
// substitutions, sudo prefixes, and environment assignments, because those are
// the four ways a command hides what it really does.
var shellReductions = []struct {
	command string
	reduced string
}{
	{"git commit -m \"add the reducer\"", "git commit"},
	{"git commit -m \"one; two\"", "git commit"},
	{"git push --force", "git push"},
	{"git reset --hard origin/main", "git reset --hard"},
	{"git reset", "git reset"},
	{"git clean -f -d", "git clean -f -d"},
	{"git clean -n", "git clean -n"},
	{"rm -rf /tmp/x", "rm -rf"},
	{"rm -r /tmp/x", "rm -r"},
	{"rm -fr /tmp/x", "rm -fr"},
	{"rm -f -r /tmp/x", "rm -f -r"},
	{"rm notes.txt", "rm"},
	{"rm -rf \"/tmp/my notes\"", "rm -rf"},
	{"find /tmp -name '*.log' -delete", "find -name -delete"},
	{"find /tmp -name '*.log'", "find -name"},
	{"sudo apt install ripgrep", "sudo apt install"},
	{"sudo -u postgres psql -c \"select 1\"", "sudo psql -c"},
	{"FOO=bar rm -rf /tmp/x", "rm -rf"},
	{"env FOO=bar rm -rf /tmp/x", "rm -rf"},
	{"cat notes.txt | grep coeus", "cat | grep"},
	{"make check && go build ./...", "make check | go build"},
	{"echo $(rm -rf /tmp/x)", "echo | rm -rf (builds part of itself at run time)"},
	{"echo `rm -rf /tmp/x`", "echo | rm -rf (builds part of itself at run time)"},
	{"npm run dev", "npm run dev"},
	{"npm install lodash", "npm install"},
	{"docker compose up -d", "docker compose up"},
	{"go test ./...", "go test"},
	{"ls -la /tmp", "ls -la"},
	{"python script.py", "python"},
	{"", ""},
}

func TestReduceCutsAShellCommandDownToTheWordsThatMatter(t *testing.T) {
	for _, reduction := range shellReductions {
		request := contract.PermissionRequest{
			ToolName: contract.ToolShell,
			Input:    jsonInput(t, map[string]any{"command": reduction.command}),
		}
		if reduced := permission.Reduce(request); reduced != reduction.reduced {
			t.Errorf("the reduced form of %q is %q, want %q", reduction.command, reduced, reduction.reduced)
		}
	}
}

func TestReduceSaysWhenAShellCommandAsksForAdministratorPowers(t *testing.T) {
	request := contract.PermissionRequest{
		ToolName: contract.ToolShell,
		Input:    jsonInput(t, map[string]any{"command": "apt install ripgrep", "escalate": true}),
	}

	if reduced := permission.Reduce(request); reduced != "sudo apt install" {
		t.Errorf("the reduced form of an escalated command is %q, want %q", reduced, "sudo apt install")
	}
}

// otherToolReductions are the calls that are not shell commands. Each one
// reduces to the tool's name and the one field that says what it will do.
var otherToolReductions = []struct {
	toolName string
	fields   map[string]any
	reduced  string
}{
	{contract.ToolRead, map[string]any{"path": "/home/jared/notes.md"}, "read /home/jared/notes.md"},
	{contract.ToolSearch, map[string]any{"pattern": "coeus"}, "search coeus"},
	{contract.ToolWeb, map[string]any{"url": "https://example.com/checkout"}, "web https://example.com/checkout"},
	{contract.ToolWeb, map[string]any{"query": "kayak prices"}, "web kayak prices"},
	{contract.ToolBrowserOpen, map[string]any{"url": "https://example.com"}, "browser_open https://example.com"},
	{contract.ToolBrowserOpen, map[string]any{"intent": "open the compose page"}, "browser_open open the compose page"},
	{contract.ToolBrowserLogin, map[string]any{"site": "x.com"}, "browser_login x.com"},
	{contract.ToolBrowserHandoff, map[string]any{"intent": "take the wheel and finish the login"}, "browser_handoff take the wheel and finish the login"},
	{contract.ToolBrowserType, map[string]any{"element": "e3", "text": "Nine years of DigiByte."}, "browser_type e3"},
	{contract.ToolBrowserType, map[string]any{"text": "Nine years of DigiByte."}, "browser_type Nine years of DigiByte."},
	{contract.ToolBrowserClick, map[string]any{"intent": "pay now", "element": "e12"}, "browser_click pay now"},
	{contract.ToolBrowserAct, map[string]any{"intent": "buy the blue kayak"}, "browser_act buy the blue kayak"},
	{contract.ToolComputer, map[string]any{"intent": "delete all the screenshots"}, "computer delete all the screenshots"},
	{contract.ToolTask, map[string]any{"operation": "plan"}, "task"},
	{contract.ToolMemory, map[string]any{"nothing": "recognised"}, "memory"},
}

func TestReduceNamesTheOneFieldThatSaysWhatAToolWillDo(t *testing.T) {
	for _, reduction := range otherToolReductions {
		request := contract.PermissionRequest{
			ToolName: reduction.toolName,
			Input:    jsonInput(t, reduction.fields),
		}
		if reduced := permission.Reduce(request); reduced != reduction.reduced {
			t.Errorf("the reduced form of a %s call is %q, want %q", reduction.toolName, reduced, reduction.reduced)
		}
	}
}

func TestReduceSaysWhenAWriteWouldEmptyAFileOverTheSize(t *testing.T) {
	big := filepath.Join(t.TempDir(), "big.log")
	writeFileOfSize(t, big, permission.EmptiesFileOverBytes+1)

	request := contract.PermissionRequest{
		ToolName: contract.ToolWrite,
		Input:    jsonInput(t, map[string]any{"path": big, "content": ""}),
	}

	reduced := permission.Reduce(request)
	if !strings.Contains(reduced, "emptying a file of") {
		t.Errorf("the reduced form of a write that empties a big file is %q, and it must say the file is being emptied", reduced)
	}
	if !strings.Contains(reduced, big) {
		t.Errorf("the reduced form %q does not name the file %q", reduced, big)
	}
}

func TestReduceSaysNothingAboutEmptyingASmallFileOrAWriteThatHasContent(t *testing.T) {
	small := filepath.Join(t.TempDir(), "small.log")
	writeFileOfSize(t, small, 10)

	emptying := contract.PermissionRequest{
		ToolName: contract.ToolWrite,
		Input:    jsonInput(t, map[string]any{"path": small, "content": ""}),
	}
	if reduced := permission.Reduce(emptying); reduced != "write "+small {
		t.Errorf("the reduced form of emptying a small file is %q, want %q", reduced, "write "+small)
	}

	filling := contract.PermissionRequest{
		ToolName: contract.ToolWrite,
		Input:    jsonInput(t, map[string]any{"path": small, "content": "hello"}),
	}
	if reduced := permission.Reduce(filling); reduced != "write "+small {
		t.Errorf("the reduced form of a write with content is %q, want %q", reduced, "write "+small)
	}
}

func TestReduceSaysWhenAnEditWouldCutOutMoreThanTheSize(t *testing.T) {
	request := contract.PermissionRequest{
		ToolName: contract.ToolEdit,
		Input: jsonInput(t, map[string]any{
			"path": "/home/jared/big.md",
			"old":  strings.Repeat("x", permission.EmptiesFileOverBytes+1),
			"new":  "",
		}),
	}

	if reduced := permission.Reduce(request); !strings.Contains(reduced, "emptying a file of") {
		t.Errorf("the reduced form of an edit that cuts out a large span is %q, and it must say the file is being emptied", reduced)
	}
}

func TestReduceNeverReturnsANewlineAndNeverReturnsMoreThanTheCap(t *testing.T) {
	request := contract.PermissionRequest{
		ToolName: contract.ToolBrowserAct,
		Input:    jsonInput(t, map[string]any{"intent": "buy\nthe\rkayak " + strings.Repeat("and more ", 200)}),
	}

	reduced := permission.Reduce(request)
	if strings.ContainsAny(reduced, "\n\r") {
		t.Errorf("the reduced form %q carries a line break, and it has to fit on one line", reduced)
	}
	if len([]rune(reduced)) > permission.MaxReducedRunes {
		t.Errorf("the reduced form is %d runes, and the cap is %d", len([]rune(reduced)), permission.MaxReducedRunes)
	}
}

func TestReduceFallsBackToTheCommandPrefixTheCallerAlreadyWorkedOut(t *testing.T) {
	request := contract.PermissionRequest{
		ToolName:      contract.ToolShell,
		Input:         json.RawMessage(`{"not": "a command"}`),
		CommandPrefix: "git push",
	}

	if reduced := permission.Reduce(request); reduced != "git push" {
		t.Errorf("the reduced form fell back to %q, want the caller's prefix %q", reduced, "git push")
	}
}

func TestReduceSurvivesInputThatIsNotJSONAtAll(t *testing.T) {
	request := contract.PermissionRequest{ToolName: contract.ToolShell, Input: json.RawMessage("not json")}

	if reduced := permission.Reduce(request); reduced != contract.ToolShell {
		t.Errorf("the reduced form of unreadable input is %q, want the tool's name %q", reduced, contract.ToolShell)
	}
}

// oddShellCommands are the command lines a shell would read strangely and a
// model still might write. None of them may stop the reducer, and each of them
// still has to come back with the program a person would recognise.
var oddShellCommands = []struct {
	command string
	reduced string
}{
	{"rm -rf '/tmp/unclosed", "rm -rf"},
	{"rm -rf \\", "rm -rf"},
	{"rm\\ -rf /tmp/x", "rm -rf"},
	{"rm -rf 'it'\\''s here'", "rm -rf"},
	{"env", "env"},
	{"FOO=bar", ""},
	{"   ", ""},
	{"|||", ""},
	{"-", "-"},
}

func TestReduceSurvivesACommandLineAShellWouldReadStrangely(t *testing.T) {
	for _, odd := range oddShellCommands {
		request := contract.PermissionRequest{
			ToolName: contract.ToolShell,
			Input:    jsonInput(t, map[string]any{"command": odd.command}),
		}
		if reduced := permission.Reduce(request); reduced != odd.reduced {
			t.Errorf("the reduced form of %q is %q, want %q", odd.command, reduced, odd.reduced)
		}
	}
}

func TestReduceIgnoresAFieldWrittenAsTheWrongKindOfThing(t *testing.T) {
	notAString := contract.PermissionRequest{
		ToolName: contract.ToolShell,
		Input:    json.RawMessage(`{"command": 12, "escalate": "yes"}`),
	}
	if reduced := permission.Reduce(notAString); reduced != contract.ToolShell {
		t.Errorf("a command written as a number reduced to %q, want the tool's name %q", reduced, contract.ToolShell)
	}

	notABool := contract.PermissionRequest{
		ToolName: contract.ToolShell,
		Input:    json.RawMessage(`{"command": "apt install ripgrep", "escalate": "yes"}`),
	}
	if reduced := permission.Reduce(notABool); reduced != "apt install" {
		t.Errorf("an escalate field written as words reduced to %q, want %q", reduced, "apt install")
	}
}

func TestReduceStopsAtTheCapsWhenACommandLineNeverEnds(t *testing.T) {
	request := contract.PermissionRequest{
		ToolName: contract.ToolShell,
		Input:    jsonInput(t, map[string]any{"command": strings.Repeat("rm -rf /tmp/x | ", 500)}),
	}

	reduced := permission.Reduce(request)
	if !strings.HasPrefix(reduced, "rm -rf") {
		t.Errorf("a command line of five hundred pipes reduced to %q, want it to start with the first command", reduced)
	}
	if len([]rune(reduced)) > permission.MaxReducedRunes {
		t.Errorf("the reduced form is %d runes, and the cap is %d", len([]rune(reduced)), permission.MaxReducedRunes)
	}
}

// jsonInput encodes a tool call's arguments the way a model would write them.
func jsonInput(t *testing.T, fields map[string]any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("cannot encode the tool input %v: %v", fields, err)
	}
	return encoded
}

func TestAReadableFormCutToTheCapSaysSoAndTheCallAsks(t *testing.T) {
	request := contract.PermissionRequest{
		ToolName: contract.ToolBrowserAct,
		Input:    jsonInput(t, map[string]any{"intent": strings.Repeat("scroll a little further down the page and then ", 40)}),
	}

	reduced := permission.Reduce(request)
	if !strings.HasSuffix(reduced, "(cut short before the end)") {
		t.Errorf("the readable form is %q, and a form cut to the cap has to say that it stopped", reduced)
	}
	if len([]rune(reduced)) > permission.MaxReducedRunes {
		t.Errorf("the readable form is %d runes, and the cap is %d", len([]rune(reduced)), permission.MaxReducedRunes)
	}

	decision := decide(t, newDecider(t, contract.DefaultConfig()), request)
	if decision.Ruling != contract.RulingAsk {
		t.Errorf("a call whose readable form was cut to the cap was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
}

// writeFileOfSize writes a file of exactly the number of bytes asked for.
func writeFileOfSize(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Repeat("x", size)), 0o600); err != nil {
		t.Fatalf("cannot write the file %s for the test: %v", path, err)
	}
}
