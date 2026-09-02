package testkit

import (
	"context"
	"fmt"
	"sync"

	"github.com/JaredTate/coeus/internal/contract"
)

// RememberedAnswer is one answer a test told the fake permission decider to
// remember for the session.
type RememberedAnswer struct {
	// Request is the call the answer was about.
	Request contract.PermissionRequest
	// Answer is once, always, or reject.
	Answer contract.PreviewAnswer
	// Reason is why, when the answer was reject.
	Reason string
}

// FakePermission rules the way a test told it to, and records every request and
// every answer so that a test can assert what the harness asked about.
type FakePermission struct {
	guard     sync.Mutex
	byDefault contract.PermissionRuling
	byTool    map[string]contract.PermissionDecision
	requests  []contract.PermissionRequest
	answers   []RememberedAnswer
}

// NewFakePermission returns a decider that gives the same ruling to everything
// no rule was written for.
func NewFakePermission(byDefault contract.PermissionRuling) *FakePermission {
	return &FakePermission{
		byDefault: byDefault,
		byTool:    map[string]contract.PermissionDecision{},
	}
}

// Rule says what to decide about one tool.
func (permission *FakePermission) Rule(toolName string, decision contract.PermissionDecision) {
	permission.guard.Lock()
	defer permission.guard.Unlock()
	permission.byTool[toolName] = decision
}

// Requests is every call the harness put to the decider, in order.
func (permission *FakePermission) Requests() []contract.PermissionRequest {
	permission.guard.Lock()
	defer permission.guard.Unlock()
	copied := make([]contract.PermissionRequest, len(permission.requests))
	copy(copied, permission.requests)
	return copied
}

// Answers is every answer the harness told the decider to remember, in order.
func (permission *FakePermission) Answers() []RememberedAnswer {
	permission.guard.Lock()
	defer permission.guard.Unlock()
	copied := make([]RememberedAnswer, len(permission.answers))
	copy(copied, permission.answers)
	return copied
}

// Decide rules on one call.
func (permission *FakePermission) Decide(_ context.Context, request contract.PermissionRequest) (contract.PermissionDecision, error) {
	permission.guard.Lock()
	defer permission.guard.Unlock()
	permission.requests = append(permission.requests, request)

	if decision, ruled := permission.byTool[request.ToolName]; ruled {
		return decision, nil
	}
	return contract.PermissionDecision{
		Ruling: permission.byDefault,
		Reason: fmt.Sprintf("no rule covers %q, so the default ruling applies", request.ToolName),
	}, nil
}

// Remember records the user's answer for the rest of the session.
func (permission *FakePermission) Remember(request contract.PermissionRequest, answer contract.PreviewAnswer, reason string) error {
	if !contract.KnownPreviewAnswer(answer) {
		return fmt.Errorf("the answer %q is not one the harness understands, so use once, always, or reject", answer)
	}
	permission.guard.Lock()
	defer permission.guard.Unlock()
	permission.answers = append(permission.answers, RememberedAnswer{Request: request, Answer: answer, Reason: reason})
	return nil
}
