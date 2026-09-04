package memory_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// theCapturedTask is the task every capture test writes its events under.
const theCapturedTask = "17"

// writeEvent puts one event of the captured task into the log.
func (opened openedMemory) writeEvent(t *testing.T, kind contract.EventKind, body any) {
	t.Helper()
	written, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("cannot write the event body: %v", err)
	}
	_, err = opened.eventLog.Append(context.Background(), contract.Event{
		Occurred: theTestDay,
		TaskID:   theCapturedTask,
		Kind:     kind,
		Body:     written,
	})
	if err != nil {
		t.Fatalf("cannot write the event into the log: %v", err)
	}
}

// captureAndSearch captures the finished task and returns what a search finds.
func (opened openedMemory) captureAndSearch(t *testing.T, query string) []contract.Fact {
	t.Helper()
	ctx := context.Background()
	if err := opened.memory.Capture(ctx, theCapturedTask); err != nil {
		t.Fatalf("cannot capture the finished task: %v", err)
	}
	found, err := opened.memory.Search(ctx, query, 10)
	if err != nil {
		t.Fatalf("cannot search after the capture: %v", err)
	}
	return found
}

func TestCaptureWritesDownTheFilesTheTaskChanged(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	opened.writeEvent(t, contract.EventFileChange, contract.FileChangeBody{
		Path: "/home/jared/coeus/blog/anniversary.md", Existed: true,
	})

	found := opened.captureAndSearch(t, "changed the file anniversary")
	if !holdsText(found, "changed the file /home/jared/coeus/blog/anniversary.md") {
		t.Errorf("the file the task changed was not written down, and the search found %v", factTexts(found))
	}
}

func TestCaptureWritesDownTheCommandsTheTaskRan(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	call, err := json.Marshal(map[string]string{"command": "git push origin main"})
	if err != nil {
		t.Fatalf("cannot write the arguments: %v", err)
	}
	opened.writeEvent(t, contract.EventToolCall, contract.ToolCall{
		ID: "call-1", Name: contract.ToolShell, Input: call,
	})

	found := opened.captureAndSearch(t, "ran the command git push")
	if !holdsText(found, "ran the command git push origin main") {
		t.Errorf("the command the task ran was not written down, and the search found %v", factTexts(found))
	}
}

func TestCaptureWritesDownTheSitesTheTaskVisited(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	opened.writeEvent(t, contract.EventToolCall, map[string]any{
		"name":      contract.ToolBrowserOpen,
		"arguments": map[string]string{"url": "https://x.com/compose"},
	})
	opened.writeEvent(t, contract.EventToolCall, map[string]any{
		"name":      contract.ToolWeb,
		"arguments": map[string]string{"url": "https://digibyte.org/news"},
	})

	found := opened.captureAndSearch(t, "visited the site")
	for _, wanted := range []string{
		"visited the site https://x.com/compose",
		"visited the site https://digibyte.org/news",
	} {
		if !holdsText(found, wanted) {
			t.Errorf("%q was not written down, and the search found %v", wanted, factTexts(found))
		}
	}
}

func TestCaptureWritesDownTheJobsTheTaskCreated(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	opened.writeEvent(t, contract.EventToolCall, map[string]any{
		"name":      contract.ToolJob,
		"arguments": map[string]string{"name": "the anniversary campaign"},
	})

	found := opened.captureAndSearch(t, "created the job anniversary campaign")
	if !holdsText(found, "created the job the anniversary campaign") {
		t.Errorf("the job the task created was not written down, and the search found %v", factTexts(found))
	}
}

func TestCaptureKeepsACorrectionWordForWordAsAFactAboutTheUser(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	corrections := []string{
		"no, lead with the date not the features",
		"actually, put the link at the end",
		"always sign off with the project name",
		"never post before nine in the morning",
		"don't use exclamation marks",
	}
	for _, said := range corrections {
		opened.writeEvent(t, contract.EventMessage, map[string]string{"role": "user", "text": said})
	}
	opened.writeEvent(t, contract.EventMessage, map[string]string{
		"role": "user", "text": "nothing about this message is a correction",
	})
	opened.writeEvent(t, contract.EventMessage, map[string]string{
		"role": "assistant", "text": "no, I will not treat my own words as a correction",
	})

	found := opened.captureAndSearch(t, "lead with the date not the features")
	for _, said := range corrections {
		if !holdsText(found, said) && !holdsText(opened.captureAndSearch(t, said), said) {
			t.Errorf("the correction %q was not kept word for word", said)
		}
	}
	held, err := os.ReadFile(opened.home.UserFactsFile())
	if err != nil {
		t.Fatalf("cannot read the user facts file: %v", err)
	}
	for _, said := range corrections {
		if !strings.Contains(string(held), said) {
			t.Errorf("the correction %q is not in USER.md, which holds:\n%s", said, held)
		}
	}
	if strings.Contains(string(held), "nothing about this message") {
		t.Error("a message that only begins with the letters of a correction word was kept")
	}
	if strings.Contains(string(held), "I will not treat my own words") {
		t.Error("a message the model wrote was kept as a correction")
	}
}

func TestACorrectionCanBeReadBackTheSameTurnItWasCaptured(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	opened.writeEvent(t, contract.EventMessage, map[string]string{
		"role": "user", "text": "no, lead with the date not the features",
	})

	if err := opened.memory.Capture(ctx, theCapturedTask); err != nil {
		t.Fatalf("cannot capture the finished task: %v", err)
	}
	hint, err := opened.memory.Hint(ctx, "which comes first in the post, the date or the features")
	if err != nil {
		t.Fatalf("cannot ask for a hint straight after the capture: %v", err)
	}
	if len(hint) == 0 || !strings.Contains(strings.Join(hint, "\n"), "lead with the date") {
		t.Errorf("the hint straight after the capture is %v, and it must carry the correction", hint)
	}
}

func TestCapturingTheSameTaskTwiceChangesNothing(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	opened.writeEvent(t, contract.EventFileChange, contract.FileChangeBody{Path: "/home/jared/coeus/a.md"})
	opened.writeEvent(t, contract.EventMessage, map[string]string{
		"role": "user", "text": "never post twice in one hour",
	})

	if err := opened.memory.Capture(ctx, theCapturedTask); err != nil {
		t.Fatalf("cannot capture the finished task: %v", err)
	}
	world, err := os.ReadFile(opened.home.WorldFactsFile())
	if err != nil {
		t.Fatalf("cannot read the world facts file: %v", err)
	}
	user, err := os.ReadFile(opened.home.UserFactsFile())
	if err != nil {
		t.Fatalf("cannot read the user facts file: %v", err)
	}

	if err := opened.memory.Capture(ctx, theCapturedTask); err != nil {
		t.Fatalf("cannot capture the finished task a second time: %v", err)
	}
	worldAgain, err := os.ReadFile(opened.home.WorldFactsFile())
	if err != nil {
		t.Fatalf("cannot read the world facts file again: %v", err)
	}
	userAgain, err := os.ReadFile(opened.home.UserFactsFile())
	if err != nil {
		t.Fatalf("cannot read the user facts file again: %v", err)
	}
	if string(world) != string(worldAgain) {
		t.Errorf("MEMORY.md changed on the second capture, from\n%s\nto\n%s", world, worldAgain)
	}
	if string(user) != string(userAgain) {
		t.Errorf("USER.md changed on the second capture, from\n%s\nto\n%s", user, userAgain)
	}
}

func TestCaptureLeavesAloneWhatItCannotVerify(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	opened.writeEvent(t, contract.EventToolResult, map[string]string{"text": "the page said hello"})
	opened.writeEvent(t, contract.EventToolCall, map[string]any{"name": contract.ToolRead,
		"arguments": map[string]string{"path": "/home/jared/coeus/a.md"}})
	opened.writeEvent(t, contract.EventToolCall, map[string]any{"name": contract.ToolShell,
		"arguments": map[string]string{"nothing": "useful"}})

	if err := opened.memory.Capture(ctx, theCapturedTask); err != nil {
		t.Fatalf("cannot capture the finished task: %v", err)
	}
	if _, err := os.Stat(opened.home.WorldFactsFile()); err == nil {
		held, readErr := os.ReadFile(opened.home.WorldFactsFile())
		if readErr == nil && strings.TrimSpace(string(held)) != "" {
			t.Errorf("a task with nothing worth writing down left facts behind:\n%s", held)
		}
	}
}

func TestCaptureNeedsATaskToCapture(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	if err := opened.memory.Capture(context.Background(), ""); err == nil {
		t.Error("capturing a task with no id returned no error, and it must say what to pass")
	}
}
