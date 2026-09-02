package testkit_test

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestTheFakeMemoryFindsFactsByTheWordsInThem(t *testing.T) {
	ctx := context.Background()
	memory := testkit.NewFakeMemory(
		contract.Fact{ID: "1", Text: "the user prefers short posts", Source: "correction C1", Recorded: time.Unix(100, 0)},
		contract.Fact{ID: "2", Text: "the DigiByte anniversary is in January", Source: "memory/product.md", Recorded: time.Unix(200, 0)},
	)

	found, err := memory.Search(ctx, "anniversary", 10)
	if err != nil {
		t.Fatalf("searching failed: %v", err)
	}
	if len(found) != 1 || found[0].ID != "2" {
		t.Errorf("searching for \"anniversary\" found %+v, want the one fact that mentions it", found)
	}
}

func TestTheFakeMemoryReturnsTheNewestFactsFirstAndHonoursTheLimit(t *testing.T) {
	ctx := context.Background()
	memory := testkit.NewFakeMemory(
		contract.Fact{ID: "1", Text: "post one is up", Recorded: time.Unix(100, 0)},
		contract.Fact{ID: "2", Text: "post two is up", Recorded: time.Unix(300, 0)},
		contract.Fact{ID: "3", Text: "post three is up", Recorded: time.Unix(200, 0)},
	)

	found, err := memory.Search(ctx, "post", 2)
	if err != nil {
		t.Fatalf("searching failed: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("searching with a limit of two found %d facts", len(found))
	}
	if found[0].ID != "2" || found[1].ID != "3" {
		t.Errorf("the facts came back as %s then %s, want the newest first", found[0].ID, found[1].ID)
	}
}

func TestTheFakeMemorySavesABatchAndReadsOneBackByItsID(t *testing.T) {
	ctx := context.Background()
	memory := testkit.NewFakeMemory()

	err := memory.Save(ctx, []contract.Fact{
		{ID: "7", Text: "the user says no emoji", Source: "correction C2", Recorded: time.Unix(400, 0)},
	})
	if err != nil {
		t.Fatalf("saving a batch failed: %v", err)
	}

	found, err := memory.Get(ctx, "7")
	if err != nil {
		t.Fatalf("reading the saved fact failed: %v", err)
	}
	if found.Text != "the user says no emoji" {
		t.Errorf("the fact came back as %q, want the one that was saved", found.Text)
	}
	if _, err := memory.Get(ctx, "nothing"); err == nil {
		t.Error("reading a fact that is not there was reported as a success, want an error naming the id")
	}
}

func TestTheFakeMemoryHintIsAtMostThreeLinesAndEmptyWhenNothingMatches(t *testing.T) {
	ctx := context.Background()
	memory := testkit.NewFakeMemory(
		contract.Fact{ID: "1", Text: "post one is up", Recorded: time.Unix(100, 0)},
		contract.Fact{ID: "2", Text: "post two is up", Recorded: time.Unix(200, 0)},
		contract.Fact{ID: "3", Text: "post three is up", Recorded: time.Unix(300, 0)},
		contract.Fact{ID: "4", Text: "post four is up", Recorded: time.Unix(400, 0)},
	)

	lines, err := memory.Hint(ctx, "post")
	if err != nil {
		t.Fatalf("asking for a hint failed: %v", err)
	}
	if len(lines) != contract.MemoryHintLines {
		t.Errorf("the hint is %d lines, want %d", len(lines), contract.MemoryHintLines)
	}

	empty, err := memory.Hint(ctx, "nothing matches this")
	if err != nil {
		t.Fatalf("asking for a hint that matches nothing failed: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("the hint for a query that matches nothing is %v, want no lines at all", empty)
	}
}

func TestTheFakeMemoryKeepsTheMemoryContract(t *testing.T) {
	if err := testkit.CheckMemory(context.Background(), testkit.NewFakeMemory()); err != nil {
		t.Fatalf("the fake memory does not keep the memory contract: %v", err)
	}
}
