package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// The bounds this package promises, written out as the numbers they are. A
// design number asserted against the constant that holds it is not pinned at
// all: the constant and the test move together and nothing fails. These are the
// literals, so changing one of them fails here and the change is deliberate.
func TestEveryBoundIsTheNumberItIsMeantToBe(t *testing.T) {
	bounds := []struct {
		name string
		held int
		want int
	}{
		{"the longest line of the hint", MaxHintRunes, 120},
		{"the most results one search returns", MaxSearchResults, 50},
		{"how much of a note or a message a result shows", maxSnippetRunes, 300},
		{"the most facts one finished task leaves behind", MaxCapturedFacts, 200},
		{"the most work one run of the indexer does", MaxIndexedPerRun, 2000},
		{"the budget a rebuild starts with", newRunBudget().left, 2000},
		{"the shortest word of a step the hint searches for", minimumHintWordRunes, 4},
		{"the words of the step a hint line has to hold", hintWordsThatMustMatch, 2},
		{"the results the hint scores before it picks its three", maxHintCandidates, 20},
		{"the reads of the log one capture may make", maxCapturePages, 100},
		{"the words one query may carry", maxQueryTokens, 32},
		{"the longest one word of a query may be", maxTokenRunes, 64},
		{"the longest one fact may be", MaxFactTextBytes, 4000},
		{"the biggest one note may be", MaxNoteBytes, 64 * 1024},
	}
	for _, bound := range bounds {
		if bound.held != bound.want {
			t.Errorf("%s is %d, want %d; if the change is meant, change this test with it",
				bound.name, bound.held, bound.want)
		}
	}
}

// aMemoryWithMessages builds a memory on a real database whose event log already
// holds a number of messages, which is what the indexer has to catch up on.
func aMemoryWithMessages(t *testing.T, messages int) *Memory {
	t.Helper()
	remembering := aMemoryOn(t, aDatabase(t))
	store := testkit.NewFakeStore()
	for number := 1; number <= messages; number++ {
		body, err := json.Marshal(map[string]string{
			"role": "user", "text": fmt.Sprintf("message number %d about the kayaks", number),
		})
		if err != nil {
			t.Fatalf("cannot write the message body: %v", err)
		}
		if _, err := store.Append(context.Background(), contract.Event{
			TaskID: "17", Kind: contract.EventMessage, Body: body,
		}); err != nil {
			t.Fatalf("cannot write message number %d into the log: %v", number, err)
		}
	}
	remembering.eventLog = store
	return remembering
}

// messagesIndexed is how many past messages the index holds.
func messagesIndexed(t *testing.T, remembering *Memory) int {
	t.Helper()
	held := 0
	row := remembering.database.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM memory_indexed WHERE kind = ?", messageEntry)
	if err := row.Scan(&held); err != nil {
		t.Fatalf("cannot count the messages the index holds: %v", err)
	}
	return held
}

func TestARebuildStopsAtItsBudgetAndCarriesOnAtTheNextRun(t *testing.T) {
	ctx := context.Background()
	remembering := aMemoryWithMessages(t, 5)

	if err := remembering.rebuildInside(ctx, &runBudget{left: 2}); err != nil {
		t.Fatalf("cannot run the indexer with a budget of two: %v", err)
	}
	if held := messagesIndexed(t, remembering); held != 2 {
		t.Fatalf("a run with a budget of two indexed %d messages, and a run that does not stop at its "+
			"budget can be held open by a very long log", held)
	}
	if err := remembering.rebuildInside(ctx, &runBudget{left: 2}); err != nil {
		t.Fatalf("cannot run the indexer a second time: %v", err)
	}
	if held := messagesIndexed(t, remembering); held != 4 {
		t.Errorf("the second run left %d messages indexed, want four, because a run carries on where "+
			"the one before it stopped", held)
	}
	if err := remembering.rebuildInside(ctx, newRunBudget()); err != nil {
		t.Fatalf("cannot run the indexer with the whole budget: %v", err)
	}
	if held := messagesIndexed(t, remembering); held != 5 {
		t.Errorf("a run with the whole budget left %d messages indexed, want all five", held)
	}
}
