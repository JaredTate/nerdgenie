package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/log"
	"github.com/JaredTate/nerdgenie/internal/memory"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestTheRealMemoryKeepsTheContractTheFakeKeeps(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	if err := testkit.CheckMemory(context.Background(), opened.memory); err != nil {
		t.Fatalf("the real memory does not keep the contract: %v", err)
	}
}

func TestOpeningTheMemoryNeedsAnEventLogAClockAndCapsAboveZero(t *testing.T) {
	ctx := context.Background()
	home := testkit.NewTempHome(t)
	eventLog, err := log.Open(ctx, home.DatabaseFile())
	if err != nil {
		t.Fatalf("cannot open the event log: %v", err)
	}
	t.Cleanup(func() { _ = eventLog.Close() })
	clock := testkit.NewFakeClock(theTestDay)

	refused := map[string]func() (*memory.Memory, error){
		"no event log": func() (*memory.Memory, error) {
			return memory.Open(ctx, home, nil, clock, shippedCaps)
		},
		"no clock": func() (*memory.Memory, error) {
			return memory.Open(ctx, home, eventLog, nil, shippedCaps)
		},
		"a world limit of zero": func() (*memory.Memory, error) {
			return memory.Open(ctx, home, eventLog, clock, contract.MemoryCaps{UserFactsBytes: 10})
		},
		"a user limit of zero": func() (*memory.Memory, error) {
			return memory.Open(ctx, home, eventLog, clock, contract.MemoryCaps{WorldFactsBytes: 10})
		},
	}
	for what, opening := range refused {
		opened, err := opening()
		if err == nil {
			_ = opened.Close()
			t.Errorf("opening the memory with %s returned no error, and it must say what is missing", what)
		}
	}
}

func TestOpeningTheMemoryOnAFileThatIsNotAnEventLogSaysSo(t *testing.T) {
	ctx := context.Background()
	home := testkit.NewTempHome(t)
	clock := testkit.NewFakeClock(theTestDay)

	opened, err := memory.Open(ctx, home, testkit.NewFakeStore(), clock, shippedCaps)
	if err == nil {
		_ = opened.Close()
		t.Fatal("opening the memory on a file with no event log in it returned no error")
	}

	withQuestionMark := contract.NewHome(home.Root + "/what?")
	if opened, err := memory.Open(ctx, withQuestionMark, testkit.NewFakeStore(), clock, shippedCaps); err == nil {
		_ = opened.Close()
		t.Error("opening the memory at a path holding a question mark returned no error")
	}
}

func TestClosingTheMemoryTwiceIsNotAnError(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	if err := opened.memory.Close(); err != nil {
		t.Fatalf("cannot close the memory: %v", err)
	}
	if err := opened.memory.Close(); err != nil {
		t.Errorf("closing the memory a second time gave the error %v", err)
	}
}

func TestSavingNothingAtAllDoesNothingAtAll(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	if err := opened.memory.Save(context.Background(), nil); err != nil {
		t.Errorf("saving an empty batch gave the error %v", err)
	}
}

func TestAFactWithNoDateOfItsOwnIsDatedByTheClock(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	opened.clock.Advance(2 * time.Hour)

	opened.saveWorldFact(t, "the anniversary is on the tenth of January")
	found, err := opened.memory.Get(ctx, "m1")
	if err != nil {
		t.Fatalf("cannot read the fact back: %v", err)
	}
	if !found.Recorded.Equal(theTestDay.Add(2 * time.Hour)) {
		t.Errorf("the fact is dated %s, want the time the clock reads, %s", found.Recorded, theTestDay.Add(2*time.Hour))
	}
}

func TestAnEventThatIsNotAMessageIsNotIndexed(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	err := opened.memory.IndexEvent(ctx, contract.Event{
		Sequence: 1, Occurred: theTestDay, Kind: contract.EventCheckpoint, Body: []byte(`{"text":"kayaks"}`),
	})
	if err != nil {
		t.Fatalf("indexing a checkpoint gave the error %v", err)
	}
	found, err := opened.memory.Search(ctx, "kayaks", 5)
	if err != nil {
		t.Fatalf("cannot search: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("a checkpoint was indexed, and the search found %v", factTexts(found))
	}
}
