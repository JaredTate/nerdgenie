package record

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// TestAPinnedResultIsWrittenIntoTheLog proves the pin is a change to the record
// like any other: it goes through the change path, so the checkpoint behind it
// carries the mark and a reader of the log alone can see what was pinned.
func TestAPinnedResultIsWrittenIntoTheLog(t *testing.T) {
	keeper, store := newKeeper(t, taskStart())
	id, err := keeper.AddResult(t.Context(), "the brand rule", "never write the word cheap")
	if err != nil {
		t.Fatalf("cannot add the result to pin: %v", err)
	}
	before := store.Count()

	if err := keeper.Pin(t.Context(), id, true); err != nil {
		t.Fatalf("cannot pin the result %s: %v", id, err)
	}

	if store.Count() <= before {
		t.Errorf("pinning wrote %d events and there were %d before it, and a pin is saved like every other change",
			store.Count()-before, before)
	}
	if !keeper.Record().Work.Results[0].Pinned {
		t.Error("the result is not marked pinned in the record the keeper holds")
	}
}

// TestAPinnedResultReadsBackOutOfTheLog is the whole point of writing the pin
// down: a task put down with evidence pinned is picked up with it still pinned,
// however long the wait and whichever model comes back to it.
func TestAPinnedResultReadsBackOutOfTheLog(t *testing.T) {
	keeper, store := newKeeper(t, taskStart())
	first, err := keeper.AddResult(t.Context(), "the brand rule", "never write the word cheap")
	if err != nil {
		t.Fatalf("cannot add the first result: %v", err)
	}
	if _, err := keeper.AddResult(t.Context(), "the draft post", "a draft nobody pinned"); err != nil {
		t.Fatalf("cannot add the second result: %v", err)
	}
	if err := keeper.Pin(t.Context(), first, true); err != nil {
		t.Fatalf("cannot pin the result %s: %v", first, err)
	}

	loaded, err := Load(t.Context(), store, contract.RecordTask, keeper.ID())
	if err != nil {
		t.Fatalf("cannot load the record back: %v", err)
	}

	results := loaded.Record().Work.Results
	if len(results) != 2 {
		t.Fatalf("the loaded record holds %d results, want the two that were written", len(results))
	}
	if !results[0].Pinned {
		t.Errorf("the result %s was pinned before the record was put down and is not pinned after it was picked up", first)
	}
	if results[1].Pinned {
		t.Errorf("the result %s was never pinned and came back pinned", results[1].ID)
	}
}

// TestUnpinningTakesTheMarkOffAgain proves the other half: a pin the model lets
// go of is let go of in the record too, so a resume does not put it back.
func TestUnpinningTakesTheMarkOffAgain(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	id, err := keeper.AddResult(t.Context(), "the brand rule", "never write the word cheap")
	if err != nil {
		t.Fatalf("cannot add the result to pin: %v", err)
	}
	if err := keeper.Pin(t.Context(), id, true); err != nil {
		t.Fatalf("cannot pin the result %s: %v", id, err)
	}

	if err := keeper.Pin(t.Context(), id, false); err != nil {
		t.Fatalf("cannot unpin the result %s: %v", id, err)
	}

	if keeper.Record().Work.Results[0].Pinned {
		t.Error("the result is still marked pinned after it was unpinned")
	}
}

// TestPinningAResultTheRecordNeverWroteIsRefused proves a pin that points at
// nothing is refused by name, the way every other label that points at nothing
// is.
func TestPinningAResultTheRecordNeverWroteIsRefused(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())

	err := keeper.Pin(t.Context(), "r99", true)

	if err == nil {
		t.Fatal("the record pinned a result it never wrote")
	}
	if !strings.Contains(err.Error(), "r99") {
		t.Errorf("the refusal reads %q, and it should name the label that is not there", err)
	}
}

// TestAPinnedResultPrintsAndReadsBackTheSame holds the promise this package is
// built on for the new mark too: the record and its text always say the same
// thing.
func TestAPinnedResultPrintsAndReadsBackTheSame(t *testing.T) {
	held := contract.Record{
		Header: contract.Header{Kind: contract.RecordTask, ID: "17", Status: contract.StatusRunning},
		Goal:   contract.Goal{Ask: "Post a tweet about the DigiByte anniversary."},
		Work: contract.Work{Results: []contract.ResultLine{
			{ID: contract.ResultID(1), Summary: "the brand rule", Pinned: true},
			{ID: contract.ResultID(2), Summary: "the draft post"},
		}},
	}

	printed := Print(held)
	read, err := Parse(printed)
	if err != nil {
		t.Fatalf("a record holding a pinned result does not read back: %v\n%s", err, printed)
	}

	if !read.Work.Results[0].Pinned {
		t.Errorf("the pinned result read back unpinned from:\n%s", printed)
	}
	if read.Work.Results[1].Pinned {
		t.Errorf("the result nobody pinned read back pinned from:\n%s", printed)
	}
}

// TestTheKeeperOfAJobPinsAReportTheSameWay proves the mark belongs to a result
// line rather than to a task, because a job's reports are result lines too.
func TestTheKeeperOfAJobPinsAReportTheSameWay(t *testing.T) {
	keeper, store := newKeeper(t, jobStart())
	id, err := keeper.AddReport(t.Context(), "task 31 posted", "the whole report")
	if err != nil {
		t.Fatalf("cannot add the report to pin: %v", err)
	}

	if err := keeper.Pin(t.Context(), id, true); err != nil {
		t.Fatalf("cannot pin the report %s: %v", id, err)
	}

	loaded, err := Load(t.Context(), store, contract.RecordJob, keeper.ID())
	if err != nil {
		t.Fatalf("cannot load the job back: %v", err)
	}
	if !loaded.Record().Work.Results[0].Pinned {
		t.Errorf("the report %s was pinned and came back unpinned", id)
	}
}

// TestAPinnedResultInAFuzzedRecordNeverChangesItsSummary is the small guard on
// the mark's own text: the mark sits before the label, where no summary can
// reach, so pinning cannot quietly rewrite the line it is on.
func TestAPinnedResultInAFuzzedRecordNeverChangesItsSummary(t *testing.T) {
	summaries := []string{"[pinned] r1 something that looks like the mark", "- [pinned] and a dash too", "plain words"}
	for _, summary := range summaries {
		keeper, _ := newKeeper(t, taskStart())
		id, err := keeper.AddResult(t.Context(), summary, "the whole text")
		if err != nil {
			t.Fatalf("cannot add a result summarised %q: %v", summary, err)
		}
		if err := keeper.Pin(t.Context(), id, true); err != nil {
			t.Fatalf("cannot pin the result summarised %q: %v", summary, err)
		}
		read, err := Parse(Print(keeper.Record()))
		if err != nil {
			t.Fatalf("a record whose result is summarised %q does not read back: %v", summary, err)
		}
		if read.Work.Results[0].Summary != summary || !read.Work.Results[0].Pinned {
			t.Errorf("the result summarised %q read back as %q, pinned %t",
				summary, read.Work.Results[0].Summary, read.Work.Results[0].Pinned)
		}
	}
}
