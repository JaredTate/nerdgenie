package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// The bodies these tests expect are kept as JSON files under testdata/codex,
// and both sides are decoded to maps before they are compared, so that the
// order the keys are written in can never fail a test. What matters is what is
// sent, not where in the body it lands.

// codexModelName is the model every golden body names.
const codexModelName = "gpt-5.5"

// readTool is the one-field tool most of the golden bodies carry.
var readTool = contract.ToolSpec{
	Name:        "read",
	Description: "Read one file.",
	Fields: []contract.ToolField{
		{Name: "path", Type: "string", Description: "The file to read.", Required: true},
	},
}

// rulesBlock is the first system block of every golden body.
var rulesBlock = contract.SystemBlock{Name: "rules", Text: "Follow the harness rules."}

// userSaying is a user message holding only text.
func userSaying(text string) contract.Message {
	return contract.Message{Role: contract.RoleUser, Text: text}
}

// requestWithEverythingForCodex has two system blocks, a user message, an
// assistant reply that speaks and makes two tool calls, two results of which
// one failed, three tools with required and optional fields, and medium effort.
func requestWithEverythingForCodex() contract.Request {
	return contract.Request{
		SystemBlocks: []contract.SystemBlock{
			rulesBlock,
			{Name: "goal", Text: "The goal is to find the port the daemon listens on."},
		},
		Messages: []contract.Message{
			userSaying("Read the config and tell me the port."),
			{
				Role: contract.RoleAssistant,
				Text: "I will read the config and search it.",
				ToolCalls: []contract.ToolCall{
					{ID: "call_1", Name: "read", Input: json.RawMessage(`{"path":"config.toml"}`)},
					{ID: "call_2", Name: "grep", Input: json.RawMessage(`{"pattern":"port","path":"config.toml"}`)},
				},
			},
			{
				Role: contract.RoleUser,
				ToolResults: []contract.ToolResult{
					{CallID: "call_1", Text: "port = 19091"},
					{CallID: "call_2", Text: "no such file: config.toml", Failed: true},
				},
			},
		},
		Tools: []contract.ToolSpec{
			readTool,
			{
				Name:        "grep",
				Description: "Search files for a pattern.",
				Fields: []contract.ToolField{
					{Name: "pattern", Type: "string", Description: "What to look for.", Required: true},
					{Name: "path", Type: "string", Description: "Where to look, or everywhere when empty."},
				},
			},
			{
				Name:        "shell",
				Description: "Run one command.",
				Fields: []contract.ToolField{
					{Name: "command", Type: "string", Description: "The command line.", Required: true},
					{Name: "timeout", Type: "integer", Description: "Seconds to wait."},
					{Name: "background", Type: "boolean"},
				},
			},
		},
		MaxOutputTokens: 4096,
		Think:           contract.ThinkMedium,
	}
}

// decodedJSON turns JSON into the map shape reflect.DeepEqual can compare.
func decodedJSON(t *testing.T, what string, raw []byte) map[string]any {
	t.Helper()
	decoded := map[string]any{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("%s is not a JSON object: %v\n%s", what, err, raw)
	}
	return decoded
}

// goldenCodexBody reads one expected body from testdata/codex.
func goldenCodexBody(t *testing.T, name string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "codex", name))
	if err != nil {
		t.Fatalf("reading the golden body failed: %v", err)
	}
	return decodedJSON(t, "the golden body "+name, raw)
}

// codexBodyFor builds the body and decodes it, failing the test on any error.
func codexBodyFor(t *testing.T, request contract.Request) map[string]any {
	t.Helper()
	raw, err := codexRequestBody(request, codexModelName)
	if err != nil {
		t.Fatalf("building the codex body failed: %v", err)
	}
	return decodedJSON(t, "the built body", raw)
}

// wantSameBody fails the test when the built body says anything different from
// the golden one, printing both with their keys sorted so the difference is
// easy to find.
func wantSameBody(t *testing.T, got map[string]any, want map[string]any) {
	t.Helper()
	if reflect.DeepEqual(got, want) {
		return
	}
	gotText, _ := json.MarshalIndent(got, "", "  ")
	wantText, _ := json.MarshalIndent(want, "", "  ")
	t.Errorf("the body differs from the golden one\n got: %s\nwant: %s", gotText, wantText)
}

func TestCodexBodyCarriesTheWholeConversationToolsAndEffort(t *testing.T) {
	got := codexBodyFor(t, requestWithEverythingForCodex())

	wantSameBody(t, got, goldenCodexBody(t, "everything.json"))
}

func TestCodexBodyLeavesTheToolsOutWhenTheyAreOff(t *testing.T) {
	request := contract.Request{
		SystemBlocks: []contract.SystemBlock{rulesBlock},
		Messages:     []contract.Message{userSaying("Write the final report.")},
		Tools:        []contract.ToolSpec{readTool},
		ToolsOff:     true,
		Think:        contract.ThinkHigh,
	}

	got := codexBodyFor(t, request)

	wantSameBody(t, got, goldenCodexBody(t, "tools_off.json"))
}

func TestCodexBodyLeavesTheToolsOutWhenThereAreNone(t *testing.T) {
	request := contract.Request{
		SystemBlocks: []contract.SystemBlock{rulesBlock},
		Messages:     []contract.Message{userSaying("Write the final report.")},
		Think:        contract.ThinkHigh,
	}

	got := codexBodyFor(t, request)

	wantSameBody(t, got, goldenCodexBody(t, "tools_off.json"))
}

func TestCodexBodySaysNothingAboutReasoningByDefaultOrWhenThinkingIsOff(t *testing.T) {
	for _, level := range []contract.Think{contract.ThinkDefault, contract.ThinkOff} {
		t.Run(string(level), func(t *testing.T) {
			request := contract.Request{
				SystemBlocks: []contract.SystemBlock{rulesBlock},
				Messages:     []contract.Message{userSaying("Say hello.")},
				Tools:        []contract.ToolSpec{readTool},
				Think:        level,
			}

			got := codexBodyFor(t, request)

			wantSameBody(t, got, goldenCodexBody(t, "no_reasoning.json"))
		})
	}
}

func TestCodexBodyAsksForEveryEffortLevelAboveOffByName(t *testing.T) {
	for _, level := range []contract.Think{
		contract.ThinkLow, contract.ThinkMedium, contract.ThinkHigh, contract.ThinkXHigh, contract.ThinkMax,
	} {
		t.Run(string(level), func(t *testing.T) {
			request := contract.Request{Messages: []contract.Message{userSaying("Think.")}, Think: level}

			got := codexBodyFor(t, request)

			reasoning, _ := got["reasoning"].(map[string]any)
			if reasoning["effort"] != string(level) {
				t.Errorf("the reasoning came out as %v, want effort %q", got["reasoning"], level)
			}
			if !reflect.DeepEqual(got["include"], []any{"reasoning.encrypted_content"}) {
				t.Errorf("the include list came out as %v, want the encrypted reasoning", got["include"])
			}
		})
	}
}

func TestCodexBodyWritesEmptyArgumentsAsAnEmptyObject(t *testing.T) {
	request := contract.Request{
		SystemBlocks: []contract.SystemBlock{rulesBlock},
		Messages: []contract.Message{
			userSaying("List the folder."),
			{
				Role: contract.RoleAssistant,
				ToolCalls: []contract.ToolCall{
					{ID: "call_1", Name: "list"},
					{ID: "call_2", Name: "list", Input: json.RawMessage("  \n")},
				},
			},
			{
				Role: contract.RoleUser,
				Text: "Now count them.",
				ToolResults: []contract.ToolResult{
					{CallID: "call_1", Text: "doc.go\nmain.go"},
					{CallID: "call_2"},
				},
			},
		},
		Tools: []contract.ToolSpec{{Name: "list", Description: "List the working folder."}},
	}

	got := codexBodyFor(t, request)

	wantSameBody(t, got, goldenCodexBody(t, "empty_arguments.json"))
}

func TestCodexBodySkipsAnAssistantMessageThatSaysNothing(t *testing.T) {
	request := contract.Request{
		Messages: []contract.Message{
			userSaying("Hello."),
			{Role: contract.RoleAssistant},
			userSaying("Are you there?"),
		},
	}

	got := codexBodyFor(t, request)

	input, _ := got["input"].([]any)
	if len(input) != 2 {
		t.Errorf("the input has %d items, want the two user messages only: %v", len(input), input)
	}
}
