package permission_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/permission"
)

func TestThePreviewOfAnEditNamesTheFileAndHowMuchOfItChanges(t *testing.T) {
	decider := newDecider(t, permission.DefaultSettings())

	decision := decide(t, decider, contract.PermissionRequest{
		ToolName: contract.ToolEdit,
		Input: jsonInput(t, map[string]any{
			"path": "/home/jared/big.md",
			"old":  strings.Repeat("x", permission.EmptiesFileOverBytes+1),
			"new":  "",
		}),
	})

	if decision.Ruling != contract.RulingAsk {
		t.Fatalf("an edit that empties a file was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
	for _, wanted := range []string{"/home/jared/big.md", "4097", "0 bytes"} {
		if !strings.Contains(decision.PreviewText, wanted) {
			t.Errorf("the preview is %q, and it has to carry %q", decision.PreviewText, wanted)
		}
	}
}

func TestThePreviewOfAWebCallSaysWhetherItFetchesOrSearches(t *testing.T) {
	decider := newDecider(t, permission.DefaultSettings())

	fetching := decide(t, decider, contract.PermissionRequest{
		ToolName: contract.ToolWeb,
		Input:    jsonInput(t, map[string]any{"url": "https://example.com/checkout"}),
	})
	if !strings.Contains(fetching.PreviewText, "fetch the page https://example.com/checkout") {
		t.Errorf("the preview of a fetch is %q, and it has to name the page", fetching.PreviewText)
	}

	searching := decide(t, decider, contract.PermissionRequest{
		ToolName: contract.ToolWeb,
		Input:    jsonInput(t, map[string]any{"query": "where to buy a kayak"}),
	})
	if !strings.Contains(searching.PreviewText, "search the web for where to buy a kayak") {
		t.Errorf("the preview of a search is %q, and it has to carry the words searched for", searching.PreviewText)
	}
}

func TestThePreviewOfAnEscalatedCommandCarriesTheReasonTheModelGave(t *testing.T) {
	decider := newDecider(t, permission.DefaultSettings())

	decision := decide(t, decider, contract.PermissionRequest{
		ToolName: contract.ToolShell,
		Input: jsonInput(t, map[string]any{
			"command":  "apt install ripgrep",
			"escalate": true,
			"reason":   "ripgrep is what the search tool runs",
		}),
	})

	if !strings.Contains(decision.PreviewText, "ripgrep is what the search tool runs") {
		t.Errorf("the preview is %q, and it has to carry the reason the model gave for asking", decision.PreviewText)
	}
}

func TestThePreviewOfAWriteWithNoFileThereYetSaysItWritesAnEmptyFile(t *testing.T) {
	settings := permission.DefaultSettings()
	settings.Rules = []permission.Rule{
		{Tool: contract.ToolWrite, Pattern: "*", Action: contract.RulingAsk, Reason: "I check every write"},
	}
	decider := newDecider(t, settings)

	decision := decide(t, decider, contract.PermissionRequest{
		ToolName: contract.ToolWrite,
		Input:    jsonInput(t, map[string]any{"path": "/tmp/there-is-no-such-file-here", "content": ""}),
	})

	if !strings.Contains(decision.PreviewText, "write an empty file at /tmp/there-is-no-such-file-here") {
		t.Errorf("the preview is %q, and it has to say an empty file is about to be written", decision.PreviewText)
	}
}

func TestThePreviewOfACallWithNoFieldTheHarnessKnowsFallsBackToTheReadableForm(t *testing.T) {
	settings := permission.DefaultSettings()
	settings.Rules = []permission.Rule{
		{Tool: "*", Pattern: "*", Action: contract.RulingAsk, Reason: "I check everything"},
	}
	decider := newDecider(t, settings)

	for _, request := range previewFallbackCalls(t) {
		decision := decide(t, decider, request)
		if decision.PreviewText == "" {
			t.Errorf("a %s call came back with no preview at all, and a ruling of ask always shows one", request.ToolName)
		}
	}
}

func TestAPreviewIsNeverLongerThanTheCap(t *testing.T) {
	decider := newDecider(t, permission.DefaultSettings())

	decision := decide(t, decider, shellRequest(t, "rm -rf "+strings.Repeat("/a-very-long-folder-name", 400)))

	if len([]rune(decision.PreviewText)) > permission.MaxPreviewRunes {
		t.Errorf("the preview is %d runes, and the cap is %d", len([]rune(decision.PreviewText)), permission.MaxPreviewRunes)
	}
}

// previewFallbackCalls are the calls whose tools carry no field the preview
// knows how to describe, so each of them falls back to the readable form.
func previewFallbackCalls(t *testing.T) []contract.PermissionRequest {
	t.Helper()
	return []contract.PermissionRequest{
		{ToolName: contract.ToolWrite, Input: jsonInput(t, map[string]any{"nothing": "here"})},
		{ToolName: contract.ToolEdit, Input: jsonInput(t, map[string]any{"nothing": "here"})},
		{ToolName: contract.ToolWeb, Input: jsonInput(t, map[string]any{"nothing": "here"})},
		{ToolName: contract.ToolComputer, Input: jsonInput(t, map[string]any{"nothing": "here"})},
		{ToolName: contract.ToolMemory, Input: jsonInput(t, map[string]any{"nothing": "here"})},
	}
}
