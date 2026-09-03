package context

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
)

// TestTheRecordSplitsAtTheCacheLine proves the record is cut into the half that
// rarely changes and the half that changes every turn, and that the header goes
// with the changing half, because the budget and the cost line are written anew
// on every turn and nothing above the cache line may move.
func TestTheRecordSplitsAtTheCacheLine(t *testing.T) {
	held := sampleRecord()
	stable, live := splitRecord(held)

	if !strings.HasPrefix(stable, "## Goal") {
		t.Errorf("the stable half does not start at the goal:\n%s", stable)
	}
	for _, wanted := range []string{"Post a tweet", "Corrections:", "Stop and tell the user if:"} {
		if !strings.Contains(stable, wanted) {
			t.Errorf("the stable half is missing %q:\n%s", wanted, stable)
		}
	}
	for _, unwanted := range []string{"## Work", "## Lessons", "budget left", "this turn:"} {
		if strings.Contains(stable, unwanted) {
			t.Errorf("the stable half carries %q, which changes every turn:\n%s", unwanted, stable)
		}
	}

	if !strings.HasPrefix(live, "# task 17") {
		t.Errorf("the live half does not start at the header:\n%s", live)
	}
	for _, wanted := range []string{"this turn:", "budget left", "## Work", "## Lessons", "r3"} {
		if !strings.Contains(live, wanted) {
			t.Errorf("the live half is missing %q:\n%s", wanted, live)
		}
	}
	if strings.Contains(live, "## Goal") {
		t.Errorf("the live half carries the goal, which belongs above the cache line:\n%s", live)
	}
}

// TestTheTwoHalvesHoldTheWholeRecord proves the split loses nothing: every line
// the printer wrote is in one half or the other.
func TestTheTwoHalvesHoldTheWholeRecord(t *testing.T) {
	held := sampleRecord()
	stable, live := splitRecord(held)
	together := live + "\n" + stable

	for _, line := range strings.Split(strings.TrimSpace(string(record.Print(held))), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.Contains(together, line) {
			t.Errorf("the split lost the line %q", line)
		}
	}
}

// TestARecordThatHasNotBeenMadeYetSplitsIntoNothing proves the first turn of a
// task, before any tool has run and before a record exists, builds no record
// halves at all rather than two empty ones.
func TestARecordThatHasNotBeenMadeYetSplitsIntoNothing(t *testing.T) {
	stable, live := splitRecord(contract.Record{})
	if stable != "" || live != "" {
		t.Errorf("a record that does not exist yet split into %q and %q, want nothing at all", stable, live)
	}
}

// sampleRecord is the task record of design section 4, which is the shape every
// part of Coeus reads and writes.
func sampleRecord() contract.Record {
	return contract.Record{
		Header: contract.Header{
			Kind: contract.RecordTask, ID: "17", Status: contract.StatusRunning, Origin: "Signal",
			RoundsLeft: 86, MinutesLeft: 51,
			Cost: contract.CostLine{InputTokens: 6100, CachedInputTokens: 5200, OutputTokens: 400},
		},
		Goal: contract.Goal{
			Ask: "Post a tweet about the DigiByte anniversary. Use the product notes and keep it under 280 characters.",
			Why: "mark the anniversary publicly today",
			DoneWhen: []contract.DoneLine{
				{Text: "one post is up on the DigiByte account"},
				{Text: "it is under 280 characters and mentions the date", Done: true, ResultID: "r6"},
			},
		},
		Rules: contract.Rules{
			Corrections: []contract.Correction{{ID: "C1", Text: "no, lead with the date not the features"}},
			StopWhen:    []string{"the account shows a login page or a captcha"},
		},
		Work: contract.Work{
			Situation: []string{"browser tab t1: x.com/compose"},
			Plan:      []contract.PlanStep{{Number: 1, Text: "read the product notes", Done: true, ResultID: "r3"}},
			Results:   []contract.ResultLine{{ID: "r3", Summary: "read memory/product.md, 2,100 characters"}},
		},
		Lessons: contract.Lessons{
			Decisions: []contract.Decision{{ID: "D1", Text: "Lead with the date", Reason: "correction C1"}},
			Failures:  []contract.Failure{{ID: "F1", Text: "Draft 1 was 312 characters", Cause: "three facts in one post"}},
		},
	}
}
