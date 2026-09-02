package repair_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/repair"
	"github.com/JaredTate/coeus/internal/testkit"
)

// testSpecs are the tools every test in this package pretends are real. Six is
// enough to exercise every name rule and short enough that the list of real
// tools stays readable inside a golden file.
func testSpecs() []contract.ToolSpec {
	return []contract.ToolSpec{
		{Name: contract.ToolRead, Description: "Read a file.", Fields: []contract.ToolField{
			{Name: "path", Type: "string", Description: "the file to read", Required: true},
		}},
		{Name: contract.ToolWrite, Description: "Write a file.", Fields: []contract.ToolField{
			{Name: "path", Type: "string", Description: "the file to write", Required: true},
			{Name: "content", Type: "string", Description: "what to write in it", Required: true},
		}},
		{Name: contract.ToolEdit, Description: "Change one span of a file.", Fields: []contract.ToolField{
			{Name: "path", Type: "string", Description: "the file to change", Required: true},
			{Name: "old", Type: "string", Description: "the text to replace", Required: true},
			{Name: "new", Type: "string", Description: "what to put in its place", Required: true},
		}},
		{Name: contract.ToolSearch, Description: "Find lines in files.", Fields: []contract.ToolField{
			{Name: "pattern", Type: "string", Description: "the regular expression to look for", Required: true},
		}},
		{Name: contract.ToolShell, Description: "Run a command."},
		{Name: contract.ToolBrowserOpen, Description: "Open a web page.", Fields: []contract.ToolField{
			{Name: "url", Type: "string", Description: "the address to open", Required: true},
		}},
	}
}

// goldenCase is one reply and the golden file that records what Find made of it.
type goldenCase struct {
	name  string
	reply contract.Reply
}

var goldenCases = []goldenCase{
	{
		name: "structured",
		reply: contract.Reply{
			Text: "I will read the product notes.",
			ToolCalls: []contract.ToolCall{{
				ID:    "call_1",
				Name:  contract.ToolRead,
				Input: []byte(`{"path": "memory/product.md"}`),
			}},
		},
	},
	{
		name: "tag-block",
		reply: contract.Reply{Text: "Before text.\n" +
			"<tool_call>\n" +
			`{"name": "shell", "arguments": {"command": "echo hi"}}` + "\n" +
			"</tool_call>\n" +
			"After text.\n"},
	},
	{
		name: "tag-block-unclosed",
		reply: contract.Reply{Text: "I will call the tool now.\n" +
			"<tool_call>\n" +
			`{"name": "shell", "arguments": {"command": "uptime -p"}}` + "\n"},
	},
	{
		name: "two-calls",
		reply: contract.Reply{Text: "<tool_call>\n" +
			`{"name": "read", "arguments": {"path": "a.txt"}}` + "\n" +
			"</tool_call>\n" +
			"<tool_call>\n" +
			`{"name": "read", "arguments": {"path": "b.txt"}}` + "\n" +
			"</tool_call>\n"},
	},
	{
		name: "fence-with-language",
		reply: contract.Reply{Text: "Here is the call.\n\n" +
			"```json\n" +
			`{"name": "search", "arguments": {"pattern": "anniversary"}}` + "\n" +
			"```\n"},
	},
	{
		name: "fence-bare",
		reply: contract.Reply{Text: "```\n" +
			`[{"name": "read", "arguments": {"path": "a.txt"}}, {"name": "read", "input": {"path": "b.txt"}}]` + "\n" +
			"```\n"},
	},
	{
		name: "fence-not-a-call",
		reply: contract.Reply{Text: "Here is the settings file.\n\n" +
			"```json\n" +
			`{"model": "local", "rounds": 100}` + "\n" +
			"```\n"},
	},
	{
		name: "bare-object",
		reply: contract.Reply{Text: "Sure, creating the file now.\n" +
			`{"name": "write", "arguments": {"path": "hello.txt", "content": "hi"}}` + "\n"},
	},
	{
		name:  "function-line",
		reply: contract.Reply{Text: `read({"path": "notes.md"})` + "\n"},
	},
	{
		name: "name-colon-line",
		reply: contract.Reply{Text: "I will look at the notes.\n" +
			`read: {"path": "notes.md"}` + "\n"},
	},
	{
		name: "arguments-as-string",
		reply: contract.Reply{Text: "<tool_call>\n" +
			`{"name": "read", "arguments": "{\"path\": \"notes.md\"}"}` + "\n" +
			"</tool_call>\n"},
	},
	{
		name: "repaired-name",
		reply: contract.Reply{Text: "<tool_call>\n" +
			`{"name": "Browser-Open", "arguments": {"url": "https://x.com"}}` + "\n" +
			"</tool_call>\n"},
	},
	{
		name: "unknown-name",
		reply: contract.Reply{Text: "<tool_call>\n" +
			`{"name": "frobnicate", "arguments": {}}` + "\n" +
			"</tool_call>\n"},
	},
	{
		name: "broken-json",
		reply: contract.Reply{Text: "<tool_call>\n" +
			`{"name": "read", "arguments": {"path": }}` + "\n" +
			"</tool_call>\n"},
	},
	{
		name: "arguments-not-an-object",
		reply: contract.Reply{Text: "<tool_call>\n" +
			`{"name": "read", "arguments": ["notes.md"]}` + "\n" +
			"</tool_call>\n"},
	},
	{
		name:  "plain-text",
		reply: contract.Reply{Text: "The post is up and it is 236 characters long.\n"},
	},
}

func TestFindMatchesTheGoldenFileForEveryEnvelopeShape(t *testing.T) {
	for _, oneCase := range goldenCases {
		t.Run(oneCase.name, func(t *testing.T) {
			result := repair.Find(oneCase.reply, testSpecs(), 0)
			testkit.Golden(t, oneCase.name+".txt", render(oneCase.reply, result))
		})
	}
}

func TestFindNeverReturnsBothCallsAndAProblem(t *testing.T) {
	for _, oneCase := range goldenCases {
		result := repair.Find(oneCase.reply, testSpecs(), 0)
		if result.Problem != "" && len(result.Calls) > 0 {
			t.Errorf("%s returned %d calls and the problem %q, want one or the other",
				oneCase.name, len(result.Calls), result.Problem)
		}
	}
}

// render writes one reply and what Find made of it as the text a golden file
// holds, so that a difference reads as a difference in behaviour rather than as
// a difference in formatting.
func render(reply contract.Reply, result repair.Result) []byte {
	written := &strings.Builder{}
	section(written, "reply text", reply.Text)
	section(written, "reply calls", callLines(reply.ToolCalls, false))
	section(written, "calls", callLines(result.Calls, true))
	section(written, "text", result.Text)
	repairs := []string{}
	for _, one := range result.Repairs {
		repairs = append(repairs, one.ModelWrote+" -> "+one.RealName)
	}
	section(written, "repairs", strings.Join(repairs, "\n"))
	section(written, "note", result.Note)
	section(written, "problem", result.Problem)
	return []byte(written.String())
}

// callLines writes one line per tool call, numbered when the calls are the ones
// Find returned.
func callLines(calls []contract.ToolCall, numbered bool) string {
	lines := []string{}
	for index, call := range calls {
		line := fmt.Sprintf("%s %s %s", call.ID, call.Name, call.Input)
		if numbered {
			line = fmt.Sprintf("%d %s", index+1, line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// section writes one named part of a golden file, and says so plainly when the
// part is empty.
func section(written *strings.Builder, name string, body string) {
	fmt.Fprintf(written, "=== %s ===\n", name)
	if body == "" {
		written.WriteString("(empty)\n")
		return
	}
	written.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		written.WriteString("\n")
	}
}
