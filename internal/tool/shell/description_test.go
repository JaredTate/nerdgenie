package shell_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestTheDescriptionFitsInTheCapAndTakesTheFixedFieldNames holds the tool's
// specification still: its name, a description under the cap that keeps the
// poll, tail and kill facts, and the fixed field names the permission
// function reduces a call by.
func TestTheDescriptionFitsInTheCapAndTakesTheFixedFieldNames(t *testing.T) {
	tool := newTool(t, testkit.NewFakeSandbox(), testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))
	spec := tool.Spec()

	if spec.Name != contract.ToolShell {
		t.Errorf("the tool calls itself %q, want %q", spec.Name, contract.ToolShell)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		t.Errorf("the description is %d words, and the cap is %d", words, contract.MaxToolDescriptionWords)
	}
	// Runs 18, 20 and 21 listed and searched files through the shell, so the
	// description sends that work to read and search, keeps the poll, tail
	// and kill facts, and says nothing against a pipeline over a command's
	// output, because "npm test | grep ✗" is a good shell command.
	for _, words := range []string{"poll, tail or kill", "read and search"} {
		if !strings.Contains(spec.Description, words) {
			t.Errorf("the description does not say %q: %q", words, spec.Description)
		}
	}
	if strings.Contains(spec.Description, "grep") || strings.Contains(spec.Description, "pipe") {
		t.Errorf("the description speaks against grep or a pipe, and a pipeline over a command's output is a good use of the shell: %q", spec.Description)
	}
	names := []string{}
	for _, field := range spec.Fields {
		names = append(names, field.Name)
	}
	if strings.Join(names, ",") != "command,action,id,escalate,reason,expect,port,path" {
		t.Errorf("the tool takes the fields %v, and the permission function reduces a shell call by command, escalate, and reason", names)
	}
}
