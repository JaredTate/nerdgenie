package permission_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/permission"
	"github.com/JaredTate/coeus/internal/testkit"
)

// A remembered answer must win whatever the rulebook would have said, because
// the user's word for the session outranks a rule that would have allowed the
// call without asking.
func TestARememberedRejectRefusesACallNoRuleWouldHaveAskedAbout(t *testing.T) {
	decider, err := permission.New(contract.DefaultConfig(), testkit.NewFakeClock(time.Unix(0, 0).UTC()))
	if err != nil {
		t.Fatalf("building the decider failed: %v", err)
	}
	ctx := context.Background()
	read := contract.PermissionRequest{ToolName: contract.ToolRead, Input: json.RawMessage(`{"path":"/tmp/notes.txt"}`)}

	before, err := decider.Decide(ctx, read)
	if err != nil || before.Ruling != contract.RulingAllow {
		t.Fatalf("a plain read should be allowed before any answer, got %+v err=%v", before, err)
	}
	if err := decider.Remember(read, contract.AnswerReject, "never read my notes"); err != nil {
		t.Fatalf("remembering a reject failed: %v", err)
	}
	after, err := decider.Decide(ctx, read)
	if err != nil {
		t.Fatalf("deciding after the reject failed: %v", err)
	}
	if after.Ruling != contract.RulingDeny {
		t.Errorf("after the user rejected the read its ruling is %q, want deny", after.Ruling)
	}
	if !strings.Contains(after.Reason, "never read my notes") {
		t.Errorf("the refusal does not carry the user's reason: %q", after.Reason)
	}
}
