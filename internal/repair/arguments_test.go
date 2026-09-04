package repair_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/repair"
)

func TestArgumentsAreReadAsAnObjectHoweverTheyWereWritten(t *testing.T) {
	cases := []struct {
		about   string
		written string
		want    string
	}{
		{about: "an object", written: `{"name": "read", "arguments": {"path": "a.txt"}}`, want: `{"path":"a.txt"}`},
		{about: "an empty object", written: `{"name": "read", "arguments": {}}`, want: `{}`},
		{about: "no arguments at all", written: `{"name": "read"}`, want: `{}`},
		{about: "nothing where the arguments go", written: `{"name": "read", "arguments": null}`, want: `{}`},
		{about: "the word the other provider uses", written: `{"name": "read", "input": {"path": "a.txt"}}`, want: `{"path":"a.txt"}`},
		{about: "an object wrapped in a string", written: `{"name": "read", "arguments": "{\"path\": \"a.txt\"}"}`, want: `{"path":"a.txt"}`},
		{about: "a brace inside a value", written: `{"name": "shell", "arguments": {"command": "echo {hi}"}}`, want: `{"command":"echo {hi}"}`},
	}

	for _, oneCase := range cases {
		t.Run(oneCase.about, func(t *testing.T) {
			reply := contract.Reply{Text: contract.ToolCallOpenTag + oneCase.written + contract.ToolCallCloseTag}
			result := repair.Find(reply, testSpecs(), 0)
			if result.Problem != "" {
				t.Fatalf("%s was refused with %q", oneCase.about, result.Problem)
			}
			if len(result.Calls) != 1 {
				t.Fatalf("%s produced %d calls, want one", oneCase.about, len(result.Calls))
			}
			if string(result.Calls[0].Input) != oneCase.want {
				t.Errorf("%s was read as %s, want %s", oneCase.about, result.Calls[0].Input, oneCase.want)
			}
		})
	}
}

func TestArgumentsThatAreNotAnObjectAreRefusedWithTheShapeTheToolExpects(t *testing.T) {
	cases := map[string]string{
		"a list":                  `{"name": "read", "arguments": ["a.txt"]}`,
		"a number":                `{"name": "read", "arguments": 5}`,
		"a string of no JSON":     `{"name": "read", "arguments": "a.txt"}`,
		"a string of a JSON list": `{"name": "read", "arguments": "[1, 2]"}`,
		"a string of an object with more written after it": `{"name": "read", ` +
			`"arguments": "{\"path\": \"a.txt\"} and then some"}`,
	}

	for about, written := range cases {
		t.Run(about, func(t *testing.T) {
			reply := contract.Reply{Text: contract.ToolCallOpenTag + written + contract.ToolCallCloseTag}
			result := repair.Find(reply, testSpecs(), 0)
			if len(result.Calls) != 0 {
				t.Fatalf("%s produced %d calls, want none", about, len(result.Calls))
			}
			for _, wanted := range []string{"read", "path", "string", "required", "the file to read"} {
				if !strings.Contains(result.Problem, wanted) {
					t.Errorf("the problem %q does not mention %q", result.Problem, wanted)
				}
			}
		})
	}
}

func TestAToolWithNoFieldsSaysSoWhenItsArgumentsAreWrong(t *testing.T) {
	reply := contract.Reply{Text: contract.ToolCallOpenTag +
		`{"name": "shell", "arguments": 5}` + contract.ToolCallCloseTag}

	result := repair.Find(reply, testSpecs(), 0)

	if !strings.Contains(result.Problem, "takes no fields") {
		t.Errorf("the problem %q does not say the tool takes no fields", result.Problem)
	}
}

func TestArgumentsFromTheProviderAreReadTheSameWay(t *testing.T) {
	reply := contract.Reply{ToolCalls: []contract.ToolCall{{
		ID:    "call_1",
		Name:  "read",
		Input: []byte(`"{\"path\": \"a.txt\"}"`),
	}}}

	result := repair.Find(reply, testSpecs(), 0)

	if result.Problem != "" {
		t.Fatalf("a provider call whose arguments were wrapped in a string was refused with %q", result.Problem)
	}
	if string(result.Calls[0].Input) != `{"path":"a.txt"}` {
		t.Errorf("the arguments were read as %s, want the object inside the string", result.Calls[0].Input)
	}
}
