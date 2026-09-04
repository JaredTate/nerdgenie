package memory_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// runMemoryCommand runs "/memory" with the words after it and fails the test
// when it will not run at all.
func (opened openedMemory) runMemoryCommand(t *testing.T, arguments string) string {
	t.Helper()
	reply, err := opened.memory.Command().Run(context.Background(), arguments, contract.CommandContext{})
	if err != nil {
		t.Fatalf("cannot run /memory %q: %v", arguments, err)
	}
	return reply
}

func TestTheMemoryCommandIsNamedAndExplained(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	command := opened.memory.Command()
	if command.Name != "memory" {
		t.Errorf("the command is called %q, want %q", command.Name, "memory")
	}
	if command.Help == "" || command.Run == nil {
		t.Errorf("the command has the help line %q and a run function that is nil: %v", command.Help, command.Run == nil)
	}
	if command.TerminalOnly {
		t.Error("the memory command is marked terminal only, and there is nothing secret in it")
	}
}

func TestTheMemoryCommandShowsTheTwoFileSizesAndTheLastFacts(t *testing.T) {
	opened := newMemory(t, shippedCaps)

	empty := opened.runMemoryCommand(t, "")
	if !strings.Contains(empty, "MEMORY.md holds 0 of") || !strings.Contains(empty, "no facts yet") {
		t.Errorf("a memory with nothing in it says:\n%s", empty)
	}

	for number := 1; number <= 12; number++ {
		opened.saveWorldFact(t, "the anniversary campaign fact number "+strings.Repeat("x", number))
	}
	reply := opened.runMemoryCommand(t, "  ")
	if !strings.Contains(reply, "MEMORY.md holds") || !strings.Contains(reply, "USER.md holds") {
		t.Errorf("the listing does not give both file sizes:\n%s", reply)
	}
	if !strings.Contains(reply, "The last 10 facts") {
		t.Errorf("the listing does not show the last ten facts:\n%s", reply)
	}
	if strings.Count(reply, "- m") != 10 {
		t.Errorf("the listing shows %d facts, want ten:\n%s", strings.Count(reply, "- m"), reply)
	}
}

func TestTheMemoryCommandSearches(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	opened.saveWorldFact(t, "the anniversary is on the tenth of January")

	found := opened.runMemoryCommand(t, "search tenth of January")
	if !strings.Contains(found, "the anniversary is on the tenth of January") {
		t.Errorf("the search does not show the fact that matches:\n%s", found)
	}
	nothing := opened.runMemoryCommand(t, "search kayaks and canoes")
	if !strings.Contains(nothing, "Nothing in memory matches") {
		t.Errorf("a search that matches nothing says:\n%s", nothing)
	}
}

func TestTheMemoryCommandForgetsAFactWithoutDeletingIt(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	if err := opened.memory.Save(ctx, []contract.Fact{
		{ID: "u1", Text: "the user posts in the morning", Source: "task 17"},
	}); err != nil {
		t.Fatalf("cannot save the fact to be forgotten: %v", err)
	}

	reply := opened.runMemoryCommand(t, "forget u1")
	if !strings.Contains(reply, "u1 is withdrawn") {
		t.Errorf("forgetting a fact says:\n%s", reply)
	}
	withdrawn, err := opened.memory.Get(ctx, "u1")
	if err != nil {
		t.Fatalf("the forgotten fact cannot be read back, and nothing is ever deleted: %v", err)
	}
	if !strings.Contains(withdrawn.Text, "superseded by") {
		t.Errorf("the forgotten fact came back as %q, and it must say it was superseded", withdrawn.Text)
	}
	found, err := opened.memory.Search(ctx, "the user withdrew the fact", 5)
	if err != nil {
		t.Fatalf("cannot search for the withdrawal: %v", err)
	}
	if !holdsText(found, "the user withdrew the fact u1, so what it said is no longer true") {
		t.Errorf("the withdrawal itself is not searchable, and the search found %v", factTexts(found))
	}
}

func TestTheMemoryCommandSaysWhatItCannotDo(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	command := opened.memory.Command()
	refused := []string{"dance", "search", "forget", "forget m404"}
	for _, arguments := range refused {
		if _, err := command.Run(context.Background(), arguments, contract.CommandContext{}); err == nil {
			t.Errorf("/memory %q returned no error, and it must say what to write instead", arguments)
		}
	}
}
