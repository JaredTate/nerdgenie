package browserhandoff_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/tool/browserhandoff"
)

// newTool builds the handoff tool over a stand-in for the user, and returns the
// tool and the reasons the user was given.
func newTool(t *testing.T, reply string, refuse error) (*browserhandoff.Tool, *[]string) {
	t.Helper()
	reasons := &[]string{}
	tool := browserhandoff.New(browserhandoff.Settings{
		AskUser: func(_ context.Context, reason string) (string, error) {
			*reasons = append(*reasons, reason)
			return reply, refuse
		},
	})
	return tool, reasons
}

// run calls the tool with the fields written as JSON.
func run(t *testing.T, tool *browserhandoff.Tool, fields map[string]any) (contract.ToolOutput, error) {
	t.Helper()
	written, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("cannot write the tool's input as JSON: %v", err)
	}
	return tool.Run(context.Background(), written)
}

func TestTheDescriptionFitsInTheCapAndTakesTheFixedFieldNames(t *testing.T) {
	tool, _ := newTool(t, "done", nil)
	spec := tool.Spec()

	if spec.Name != contract.ToolBrowserHandoff {
		t.Errorf("the tool calls itself %q, want %q", spec.Name, contract.ToolBrowserHandoff)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		t.Errorf("the description is %d words, and the cap is %d", words, contract.MaxToolDescriptionWords)
	}
	names := []string{}
	for _, field := range spec.Fields {
		names = append(names, field.Name)
	}
	if strings.Join(names, ",") != "intent,reason" {
		t.Errorf("the tool takes the fields %v, and the permission function reduces a handoff by intent and reason", names)
	}
}

func TestTheUserIsGivenTheReasonAndTheirReplyComesBack(t *testing.T) {
	tool, reasons := newTool(t, "done", nil)

	output, err := run(t, tool, map[string]any{
		"intent": "get past the captcha", "reason": "the page is asking whether the visitor is a person",
	})
	if err != nil {
		t.Fatalf("handing the browser to the user failed: %v", err)
	}
	if len(*reasons) != 1 || !strings.Contains((*reasons)[0], "is a person") {
		t.Errorf("the user was given %v, want the reason the model wrote", *reasons)
	}
	testkit.Golden(t, "a_handoff.txt", []byte(output.Text))
}

func TestAUserWhoNeverAnswersIsReportedRatherThanWaitedOnForEver(t *testing.T) {
	tool, _ := newTool(t, "", errors.New("nobody answered before the handoff timeout, so the task stopped"))

	_, err := run(t, tool, map[string]any{"intent": "get past the wall", "reason": "the page wants a code"})
	if err == nil {
		t.Fatalf("a handoff nobody answered was treated as an answer")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("the failure reads %q and does not say why nothing came back", err)
	}
}

func TestBadInputIsRefusedWithALineTheModelCanActOn(t *testing.T) {
	tool, _ := newTool(t, "done", nil)

	if _, err := tool.Run(context.Background(), json.RawMessage("not json")); err == nil {
		t.Errorf("input that is not JSON was treated as a call")
	}
	for _, broken := range []map[string]any{
		{"reason": "the page wants a code"},
		{"intent": "get past the wall"},
		{"intent": "get past the wall", "reason": strings.Repeat("y", browserhandoff.MaxReasonRunes+1)},
	} {
		if _, err := run(t, tool, broken); err == nil {
			t.Errorf("the call %v was treated as something the tool could do", broken)
		}
	}
}

func TestAToolWithNoWayToReachTheUserSaysSo(t *testing.T) {
	tool := browserhandoff.New(browserhandoff.Settings{})

	_, err := run(t, tool, map[string]any{"intent": "get past the wall", "reason": "the page wants a code"})
	if err == nil {
		t.Fatalf("the browser was handed over with no way to reach the user")
	}
	if !strings.Contains(err.Error(), "user") {
		t.Errorf("the refusal reads %q and does not say what is missing", err)
	}
}
