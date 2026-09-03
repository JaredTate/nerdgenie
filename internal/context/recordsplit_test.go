package context

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
)

// TestTheRecordSplitsInOrderOfHowOftenEachPieceChanges proves the record is cut
// into three: the goal and the rules, which rarely change and go above the cache
// line; the work and the lessons, which only grow; and the header, which carries
// the budget left and the cost of the last call and is written anew every turn.
// The order is what a prompt cache can reuse, so the header may not be mixed in
// with either of the other two.
func TestTheRecordSplitsInOrderOfHowOftenEachPieceChanges(t *testing.T) {
	held := sampleRecord()
	stable, body, standing := splitRecord(held)

	if !strings.HasPrefix(stable, "## Goal") {
		t.Errorf("the stable piece does not start at the goal:\n%s", stable)
	}
	for _, wanted := range []string{"Post a tweet", "Corrections:", "Stop and tell the user if:"} {
		if !strings.Contains(stable, wanted) {
			t.Errorf("the stable piece is missing %q:\n%s", wanted, stable)
		}
	}
	for _, unwanted := range []string{"## Work", "## Lessons", "budget left", "this turn:"} {
		if strings.Contains(stable, unwanted) {
			t.Errorf("the stable piece carries %q, which changes as the task runs:\n%s", unwanted, stable)
		}
	}

	if !strings.HasPrefix(body, "## Work") {
		t.Errorf("the body does not start at the work:\n%s", body)
	}
	for _, wanted := range []string{"## Work", "## Lessons", "r3"} {
		if !strings.Contains(body, wanted) {
			t.Errorf("the body is missing %q:\n%s", wanted, body)
		}
	}
	for _, unwanted := range []string{"## Goal", "budget left", "this turn:"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("the body carries %q, which does not change at the same pace as the work:\n%s", unwanted, body)
		}
	}

	if !strings.HasPrefix(standing, "# task 17") {
		t.Errorf("the standing piece does not start at the header:\n%s", standing)
	}
	for _, wanted := range []string{"budget left", "this turn:"} {
		if !strings.Contains(standing, wanted) {
			t.Errorf("the standing piece is missing %q:\n%s", wanted, standing)
		}
	}
	for _, unwanted := range []string{"## Goal", "## Work", "## Lessons"} {
		if strings.Contains(standing, unwanted) {
			t.Errorf("the standing piece carries %q, which would then be re-read on every call:\n%s", unwanted, standing)
		}
	}
}

// TestTheThreePiecesHoldTheWholeRecord proves the split loses nothing: every
// line the printer wrote is in one piece or another.
func TestTheThreePiecesHoldTheWholeRecord(t *testing.T) {
	held := sampleRecord()
	stable, body, standing := splitRecord(held)
	together := standing + "\n" + stable + "\n" + body

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
// pieces at all rather than three empty ones.
func TestARecordThatHasNotBeenMadeYetSplitsIntoNothing(t *testing.T) {
	stable, body, standing := splitRecord(contract.Record{})
	if stable != "" || body != "" || standing != "" {
		t.Errorf("a record that does not exist yet split into %q, %q and %q, want nothing at all", stable, body, standing)
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
