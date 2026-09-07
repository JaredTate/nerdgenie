package memory_test

import (
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// writeShellCall puts a shell call of the captured task into the log.
func (opened openedMemory) writeShellCall(t *testing.T, callID string, command string) {
	t.Helper()
	opened.writeEvent(t, contract.EventToolCall, map[string]any{
		"id": callID, "name": contract.ToolShell,
		"arguments": map[string]string{"command": command},
	})
}

// writeResult puts the result of one call into the log the way the record's
// keeper stores it: the label, the id of the call, the one-line summary, and
// the whole text.
func (opened openedMemory) writeResult(t *testing.T, callID string, summary string, text string) {
	t.Helper()
	opened.writeEvent(t, contract.EventToolResult, map[string]string{
		"id": "r1", "callId": callID, "summary": summary, "text": text,
	})
}

// TestCaptureWritesACommandThatRanAsRan holds that a command whose result came
// back with a zero exit code is written down as run.
func TestCaptureWritesACommandThatRanAsRan(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	opened.writeShellCall(t, "call-1", "git push origin main")
	opened.writeResult(t, "call-1", "pushed", "finished with exit code 0\nstdout:\nEverything up-to-date")

	found := opened.captureAndSearch(t, "git push origin main")
	if !holdsText(found, "ran the command git push origin main") {
		t.Errorf("the command that ran was not written as run, and the search found %v", factTexts(found))
	}
}

// TestCaptureWritesAFailedCommandAsFailed holds that a command whose result
// came back with a non-zero exit code is written down as having failed, so
// memory never says a thing was done that the command did not do.
func TestCaptureWritesAFailedCommandAsFailed(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	opened.writeShellCall(t, "call-1", "go build ./...")
	opened.writeResult(t, "call-1", "shell: finished with exit code 1", "finished with exit code 1\nstderr:\nmain.go:3: undefined: x")

	found := opened.captureAndSearch(t, "go build")
	if !holdsText(found, "ran the command go build ./..., which failed") {
		t.Errorf("the failed command was not written as failed, and the search found %v", factTexts(found))
	}
	if holdsText(found, "ran the command go build ./...") {
		t.Error("the failed command was also written down as if it had run cleanly")
	}
}

// TestCaptureWritesARefusedCommandAsRefused holds that a command the tool
// refused, whose result says so, is written down as refused and never as run.
func TestCaptureWritesARefusedCommandAsRefused(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	opened.writeShellCall(t, "call-1", "rm -rf build")
	opened.writeResult(t, "call-1", "shell was refused: the command is on the ask-me-first list",
		"the command is on the ask-me-first list and nobody was there to answer")

	found := opened.captureAndSearch(t, "rm -rf build")
	if !holdsText(found, "was refused the command rm -rf build") {
		t.Errorf("the refused command was not written as refused, and the search found %v", factTexts(found))
	}
	if holdsText(found, "ran the command rm -rf build") {
		t.Error("the refused command was written down as if it had run")
	}
}

// TestCaptureWritesADeniedCommandAsRefused holds that a command the permission
// function denied, which leaves a denial in the log and no result at all, is
// written down as refused.
func TestCaptureWritesADeniedCommandAsRefused(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	opened.writeShellCall(t, "call-1", "git push --force")
	opened.writeEvent(t, contract.EventPermissionDecision, map[string]any{
		"tool": contract.ToolShell, "call": "call-1", "ruling": contract.RulingDeny,
		"reason": "a forced push is on the reject list",
	})

	found := opened.captureAndSearch(t, "git push --force")
	if !holdsText(found, "was refused the command git push --force") {
		t.Errorf("the denied command was not written as refused, and the search found %v", factTexts(found))
	}
}

// TestCaptureLeavesOutACallThatGotNoResult holds that a call the log holds no
// result for, because the turn ended before it ran, is not written down at all:
// nothing happened, so there is nothing to remember.
func TestCaptureLeavesOutACallThatGotNoResult(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	opened.writeShellCall(t, "call-1", "make release")
	opened.writeShellCall(t, "call-2", "make build")
	opened.writeResult(t, "call-2", "built", "finished with exit code 0")

	found := opened.captureAndSearch(t, "make release")
	for _, fact := range found {
		if fact.Text == "ran the command make release" || fact.Text == "was refused the command make release" {
			t.Errorf("a call that got no result was written down as %q", fact.Text)
		}
	}
	if !holdsText(opened.captureAndSearch(t, "make build"), "ran the command make build") {
		t.Error("the call after it, which did run, was not written down")
	}
}

// TestCaptureWritesARefusedVisitAsNotAllowed holds that a site the browser was
// refused is written down as not allowed rather than as visited.
func TestCaptureWritesARefusedVisitAsNotAllowed(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	opened.writeEvent(t, contract.EventToolCall, map[string]any{
		"id": "call-1", "name": contract.ToolBrowserOpen,
		"arguments": map[string]string{"url": "https://x.com/compose"},
	})
	opened.writeResult(t, "call-1", contract.ToolBrowserOpen+" was refused: the site is not on the allowed list",
		"the site is not on the allowed list")

	found := opened.captureAndSearch(t, "x.com/compose")
	if !holdsText(found, "was not allowed to visit https://x.com/compose") {
		t.Errorf("the refused visit was not written as not allowed, and the search found %v", factTexts(found))
	}
	if holdsText(found, "visited the site https://x.com/compose") {
		t.Error("the refused visit was written down as a visit")
	}
}
