package context

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestTheRecordSplitsInOrderOfHowOftenEachPieceChanges proves the record is cut
// into four: the goal and the rules, which rarely change and go above the cache
// line; the work and the lessons, which change when the model edits its plan or
// its lists; the list of results, which grows by a line every round; and the
// header, which carries the budget left and the cost of the last call and is
// written anew every turn. The order is what a prompt cache can reuse, so no
// piece may carry another one's lines.
func TestTheRecordSplitsInOrderOfHowOftenEachPieceChanges(t *testing.T) {
	parts := splitRecord(sampleRecord())

	if !strings.HasPrefix(parts.Stable, "## Goal") {
		t.Errorf("the stable piece does not start at the goal:\n%s", parts.Stable)
	}
	for _, wanted := range []string{"Post a tweet", "Corrections:", "Stop and tell the user if:"} {
		if !strings.Contains(parts.Stable, wanted) {
			t.Errorf("the stable piece is missing %q:\n%s", wanted, parts.Stable)
		}
	}
	for _, unwanted := range []string{"## Work", "## Lessons", "budget left", "this turn:", "r3"} {
		if strings.Contains(parts.Stable, unwanted) {
			t.Errorf("the stable piece carries %q, which changes as the task runs:\n%s", unwanted, parts.Stable)
		}
	}

	if !strings.HasPrefix(parts.Body, "## Work") {
		t.Errorf("the body does not start at the work:\n%s", parts.Body)
	}
	for _, wanted := range []string{"## Work", "Plan:", "## Lessons"} {
		if !strings.Contains(parts.Body, wanted) {
			t.Errorf("the body is missing %q:\n%s", wanted, parts.Body)
		}
	}
	for _, unwanted := range []string{"## Goal", "budget left", "this turn:", "r3 read memory/product.md"} {
		if strings.Contains(parts.Body, unwanted) {
			t.Errorf("the body carries %q, which does not change at the same pace as the work:\n%s", unwanted, parts.Body)
		}
	}

	if !strings.HasPrefix(parts.Results, "Results (") {
		t.Errorf("the list of results does not start at its own label:\n%s", parts.Results)
	}
	if !strings.Contains(parts.Results, "r3 read memory/product.md") {
		t.Errorf("the list of results is missing the one result this record holds:\n%s", parts.Results)
	}
	for _, unwanted := range []string{"## Work", "## Lessons", "budget left"} {
		if strings.Contains(parts.Results, unwanted) {
			t.Errorf("the list of results carries %q, which changes at another pace:\n%s", unwanted, parts.Results)
		}
	}

	if !strings.HasPrefix(parts.Standing, "# task 17") {
		t.Errorf("the standing piece does not start at the header:\n%s", parts.Standing)
	}
	for _, wanted := range []string{"budget left", "this turn:"} {
		if !strings.Contains(parts.Standing, wanted) {
			t.Errorf("the standing piece is missing %q:\n%s", wanted, parts.Standing)
		}
	}
	for _, unwanted := range []string{"## Goal", "## Work", "## Lessons"} {
		if strings.Contains(parts.Standing, unwanted) {
			t.Errorf("the standing piece carries %q, which would then be re-read on every call:\n%s", unwanted, parts.Standing)
		}
	}
}

// TestAnAskTooLongForEveryPromptArrivesWithAPointerToTheRest proves the working
// context reads the record through the printer meant for the model. An ask is
// the user's own words and is never cut where it is stored, but a pasted
// specification cannot ride in front of the model on every call, so what arrives
// is its first quarter and one line saying how to read the whole of it.
func TestAnAskTooLongForEveryPromptArrivesWithAPointerToTheRest(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	input := sampleInput()
	held := input.Record
	held.Goal.Ask = "Build the release pipeline. " + strings.Repeat("one more sentence of the specification. ", 400)
	input.Record = held

	request, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}

	whole := testkit.WholeRequestText(request)
	if strings.Contains(whole, held.Goal.Ask) {
		t.Error("a pasted specification is in front of the model whole, on every call of the task")
	}
	if !strings.Contains(whole, "Build the release pipeline.") {
		t.Errorf("the start of the ask never reached the prompt:\n%s", whole)
	}
	if !strings.Contains(whole, record.AskCutNote) {
		t.Error("nothing in the prompt says the ask was shortened or how to read the rest of it")
	}
}

// TestTheFourPiecesHoldTheWholeRecord proves the split loses nothing: every line
// the printer wrote is in one piece or another.
func TestTheFourPiecesHoldTheWholeRecord(t *testing.T) {
	held := sampleRecord()
	parts := splitRecord(held)
	together := strings.Join([]string{parts.Standing, parts.Stable, parts.Body, parts.Results}, "\n")

	for _, line := range strings.Split(strings.TrimSpace(string(record.Print(held))), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.Contains(together, line) {
			t.Errorf("the split lost the line %q", line)
		}
	}
}

// TestAWorkSectionWithNoResultsYetIsHandedBackWhole proves the early rounds of a
// task, when nothing has been run, keep their work in one piece rather than
// growing an empty list beside it.
func TestAWorkSectionWithNoResultsYetIsHandedBackWhole(t *testing.T) {
	held := sampleRecord()
	held.Work.Results = nil

	parts := splitRecord(held)
	if parts.Results != "" {
		t.Errorf("a record with no results built a list anyway:\n%s", parts.Results)
	}
	if !strings.Contains(parts.Body, "## Lessons") {
		t.Errorf("the body lost the lessons when there were no results to cut out:\n%s", parts.Body)
	}
}

// TestARecordThatHasNotBeenMadeYetSplitsIntoNothing proves the first turn of a
// task, before any tool has run and before a record exists, builds no record
// pieces at all rather than four empty ones.
func TestARecordThatHasNotBeenMadeYetSplitsIntoNothing(t *testing.T) {
	if parts := splitRecord(contract.Record{}); parts != (recordParts{}) {
		t.Errorf("a record that does not exist yet split into %+v, want nothing at all", parts)
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
