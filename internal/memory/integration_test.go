//go:build integration

// This file is the integration test for memory: the real event log, the real
// SQLite file, and the real filesystem in a temporary home, driven through the
// same calls the loop will make. It runs under the integration build tag, which
// is what "make test" and "make check" pass.
package memory_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/memory"
)

// tightCaps are small enough that one task's worth of facts pushes MEMORY.md
// past its limit, which is what puts the dated notes under test here too.
var tightCaps = contract.MemoryCaps{WorldFactsBytes: 600, UserFactsBytes: 400}

func TestAWholeTaskCapturedAndFoundAgainAfterARestart(t *testing.T) {
	ctx := context.Background()
	opened := newMemory(t, tightCaps)

	for number := 1; number <= 8; number++ {
		opened.writeEvent(t, contract.EventFileChange, contract.FileChangeBody{
			Path: filepath.Join("/home/jared/nerdgenie/blog", "part-"+strings.Repeat("x", number)+".md"),
		})
	}
	opened.writeEvent(t, contract.EventToolCall, map[string]any{
		"name": contract.ToolShell, "arguments": map[string]string{"command": "hugo --minify"},
	})
	opened.writeEvent(t, contract.EventMessage, map[string]string{
		"role": "user", "text": "always put the anniversary date in the first line",
	})

	if err := opened.memory.Capture(ctx, theCapturedTask); err != nil {
		t.Fatalf("cannot capture the finished task: %v", err)
	}
	if err := opened.memory.Close(); err != nil {
		t.Fatalf("cannot close the memory: %v", err)
	}

	remembering := reopenWith(t, opened, tightCaps)
	correction, err := remembering.Search(ctx, "always put the anniversary date in the first line", 5)
	if err != nil {
		t.Fatalf("cannot search after the restart: %v", err)
	}
	if !holdsText(correction, "always put the anniversary date in the first line") {
		t.Errorf("the correction did not survive the restart, and the search found %v", factTexts(correction))
	}

	world, err := os.ReadFile(opened.home.WorldFactsFile())
	if err != nil {
		t.Fatalf("cannot read the world facts file: %v", err)
	}
	if len(world) > tightCaps.WorldFactsBytes {
		t.Errorf("MEMORY.md is %d bytes and the cap is %d", len(world), tightCaps.WorldFactsBytes)
	}
	notes, err := filepath.Glob(filepath.Join(opened.home.MemoryFolder(), "MEMORY-*.md"))
	if err != nil || len(notes) == 0 {
		t.Fatalf("the facts that left MEMORY.md are in no dated note: %v %v", notes, err)
	}
	if _, err := remembering.Get(ctx, "c"+theCapturedTask+"-1"); err != nil {
		t.Errorf("the first captured fact cannot be read back by its id after the restart: %v", err)
	}
}

func TestEveryMemoryFileWrittenIsInTheEventLogWithWhatItHeldBefore(t *testing.T) {
	ctx := context.Background()
	opened := newMemory(t, tightCaps)

	opened.saveWorldFact(t, "the anniversary is on the tenth of January")
	opened.saveWorldFact(t, "the blog is built with Hugo")

	changes, err := opened.eventLog.ByKind(ctx, contract.EventFileChange)
	if err != nil {
		t.Fatalf("cannot read the file changes out of the log: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("the log holds %d file changes, want one for each save", len(changes))
	}
	first, second := contract.FileChangeBody{}, contract.FileChangeBody{}
	if err := json.Unmarshal(changes[0].Body, &first); err != nil {
		t.Fatalf("cannot read the first file change: %v", err)
	}
	if err := json.Unmarshal(changes[1].Body, &second); err != nil {
		t.Fatalf("cannot read the second file change: %v", err)
	}
	if first.Existed {
		t.Errorf("the first change says MEMORY.md was already there, and it was not")
	}
	if !second.Existed || !strings.Contains(string(second.PriorContents), "tenth of January") {
		t.Errorf("the second change does not carry what MEMORY.md held before, which is what an undo puts back: %+v", second)
	}
}

func TestTheIndexFindsAFactSomebodyTypedIntoTheFileByHand(t *testing.T) {
	ctx := context.Background()
	opened := newMemory(t, tightCaps)
	opened.saveWorldFact(t, "the anniversary is on the tenth of January")

	held, err := os.ReadFile(opened.home.WorldFactsFile())
	if err != nil {
		t.Fatalf("cannot read the world facts file: %v", err)
	}
	byHand := string(held) + "- m99 [2026-08-01T09:00:00Z | the user] the office is closed on Fridays\n"
	if err := os.WriteFile(opened.home.WorldFactsFile(), []byte(byHand), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file by hand: %v", err)
	}

	remembering := reopenWith(t, opened, tightCaps)
	found, err := remembering.Get(ctx, "m99")
	if err != nil {
		t.Fatalf("the fact typed in by hand was not picked up: %v", err)
	}
	if found.Text != "the office is closed on Fridays" {
		t.Errorf("the hand-written fact came back as %q", found.Text)
	}
	matches, err := remembering.Search(ctx, "when is the office closed", 3)
	if err != nil {
		t.Fatalf("cannot search for the hand-written fact: %v", err)
	}
	if !holdsText(matches, "the office is closed on Fridays") {
		t.Errorf("the hand-written fact is not searchable, and the search found %v", factTexts(matches))
	}
}

func TestTwoMemoriesOnOneFileBothSeeWhatTheOtherWrote(t *testing.T) {
	ctx := context.Background()
	opened := newMemory(t, tightCaps)
	second := reopenWith(t, opened, tightCaps)

	opened.saveWorldFact(t, "the anniversary is on the tenth of January")
	found, err := second.Search(ctx, "tenth of January", 5)
	if err != nil {
		t.Fatalf("cannot search from the second memory: %v", err)
	}
	if !holdsText(found, "the anniversary is on the tenth of January") {
		t.Errorf("the second memory on the same file did not see the save, and it found %v", factTexts(found))
	}
}

// reopenWith opens the memory again on the same home with the caps given, which
// is what a restart of the agent does.
func reopenWith(t *testing.T, opened openedMemory, caps contract.MemoryCaps) *memory.Memory {
	t.Helper()
	again, err := memory.Open(context.Background(), opened.home, opened.eventLog, opened.clock, caps)
	if err != nil {
		t.Fatalf("cannot open the memory again: %v", err)
	}
	t.Cleanup(func() { _ = again.Close() })
	return again
}
