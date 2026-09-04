package permission_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/permission"
)

func FuzzReduce(f *testing.F) {
	f.Add(contract.ToolShell, []byte(`{"command":"rm -rf /tmp/x"}`))
	f.Add(contract.ToolShell, []byte(`{"command":"git commit -m \"one; two\"","escalate":true}`))
	f.Add(contract.ToolBrowserAct, []byte(`{"intent":"buy the blue kayak"}`))
	f.Add(contract.ToolWrite, []byte(`{"path":"/tmp/x","content":""}`))
	f.Add("", []byte(""))
	f.Add("shell", []byte("not json at all"))

	f.Fuzz(func(t *testing.T, toolName string, input []byte) {
		reduced := permission.Reduce(contract.PermissionRequest{
			ToolName: toolName,
			Input:    json.RawMessage(input),
		})
		checkReadableForm(t, reduced)
	})
}

func FuzzReduceShellCommand(f *testing.F) {
	for _, reduction := range shellReductions {
		f.Add(reduction.command)
	}
	f.Add("rm -rf $(echo /tmp)")
	f.Add("'unclosed quote")
	f.Add("a\\")

	f.Fuzz(func(t *testing.T, command string) {
		encoded, err := json.Marshal(map[string]string{"command": command})
		if err != nil {
			t.Skip("this command cannot be written as JSON, so no model could have sent it")
		}
		reduced := permission.Reduce(contract.PermissionRequest{
			ToolName: contract.ToolShell,
			Input:    encoded,
		})
		checkReadableForm(t, reduced)
	})
}

func FuzzRulebookMatch(f *testing.F) {
	f.Add("*rm -r*", "rm -rf")
	f.Add("*", "")
	f.Add("?", "x")
	f.Add("[", "[")
	f.Add("(*)", "()")

	f.Fuzz(func(t *testing.T, pattern string, reduced string) {
		book, err := permission.NewRulebook([]permission.Rule{
			{Tool: "*", Pattern: pattern, Action: contract.RulingAsk, Reason: "a pattern under test"},
		})
		if err != nil {
			t.Skip("this pattern is refused when the rulebook is built, which is the point of building it once")
		}
		book.Match(contract.ToolShell, reduced)
	})
}

// checkReadableForm holds the two promises the readable form makes to everything
// that shows it: it fits on one line, and it fits inside the cap.
func checkReadableForm(t *testing.T, reduced string) {
	t.Helper()
	if strings.ContainsAny(reduced, "\n\r") {
		t.Fatalf("the readable form %q carries a line break, and it has to fit on one line", reduced)
	}
	if len([]rune(reduced)) > permission.MaxReducedRunes {
		t.Fatalf("the readable form is %d runes, and the cap is %d", len([]rune(reduced)), permission.MaxReducedRunes)
	}
}
