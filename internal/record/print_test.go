package record

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// TestPrintsTheTaskExampleFromTheDesign holds the printer to the task record in
// section 4 of the design, byte for byte.
func TestPrintsTheTaskExampleFromTheDesign(t *testing.T) {
	testkit.Golden(t, "task.txt", Print(goldenTaskRecord()))
}

// TestPrintsTheJobExampleFromTheDesign holds the printer to the job record in
// section 4 of the design, byte for byte.
func TestPrintsTheJobExampleFromTheDesign(t *testing.T) {
	testkit.Golden(t, "job.txt", Print(goldenJobRecord()))
}

// TestPrintsTheFourPartsInOrder proves rule five: the goal and the rules come
// first, because everything above the cache line must not move.
func TestPrintsTheFourPartsInOrder(t *testing.T) {
	text := string(Print(contract.Record{Header: contract.Header{Kind: contract.RecordTask, ID: "1", Status: contract.StatusRunning}}))
	wanted := []string{"## Goal", "## Rules", "## Work", "## Lessons"}
	at := 0
	for _, heading := range wanted {
		found := strings.Index(text[at:], heading)
		if found < 0 {
			t.Fatalf("the printed record has no %q heading, and every record has all four:\n%s", heading, text)
		}
		at += found + len(heading)
	}
}

// TestPrintsNoSubsectionThatHasNothingInIt proves an empty record prints its
// four headings and nothing else, so a fresh record costs almost no tokens.
func TestPrintsNoSubsectionThatHasNothingInIt(t *testing.T) {
	text := string(Print(contract.Record{Header: contract.Header{Kind: contract.RecordTask, ID: "1", Status: contract.StatusRunning}}))
	for _, unwanted := range []string{"Done when:", "Corrections:", "Situation:", "Plan:", "Decisions:", "Failures:"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("the printed record has the empty subsection %q in it:\n%s", unwanted, text)
		}
	}
}

// TestPrintsTheArrowOnEveryDoneLineOnceOneIsProven is the rule that reproduces
// both examples from the design: the arrow is drawn on every line once any line
// has a result behind it, so the lines still waiting are visible.
func TestPrintsTheArrowOnEveryDoneLineOnceOneIsProven(t *testing.T) {
	proven := contract.Record{
		Header: contract.Header{Kind: contract.RecordTask, ID: "1", Status: contract.StatusRunning},
		Goal: contract.Goal{DoneWhen: []contract.DoneLine{
			{Text: "the first thing"},
			{Text: "the second thing", Done: true, ResultID: "r1"},
		}},
	}
	text := string(Print(proven))
	if !strings.Contains(text, "- [ ] the first thing ->\n") {
		t.Errorf("the unproven line has no arrow, and one of its neighbours is proven:\n%s", text)
	}
	if !strings.Contains(text, "- [x] the second thing -> r1\n") {
		t.Errorf("the proven line does not point at its result:\n%s", text)
	}

	nothingProven := proven
	nothingProven.Goal.DoneWhen = []contract.DoneLine{{Text: "the first thing"}}
	if bare := string(Print(nothingProven)); !strings.Contains(bare, "- [ ] the first thing\n") {
		t.Errorf("an arrow was drawn although no line has a result behind it:\n%s", bare)
	}
}

// TestPrintsAUserReplyAsTheProofOnADoneLine proves the second kind of proof: the
// user's own words, in quotes, so a reader can tell them from a result id.
func TestPrintsAUserReplyAsTheProofOnADoneLine(t *testing.T) {
	record := contract.Record{
		Header: contract.Header{Kind: contract.RecordTask, ID: "1", Status: contract.StatusRunning},
		Goal:   contract.Goal{DoneWhen: []contract.DoneLine{{Text: "the user is happy", Done: true, UserReply: "yes, that is right"}}},
	}
	if text := string(Print(record)); !strings.Contains(text, `- [x] the user is happy -> "yes, that is right"`) {
		t.Errorf("the user's reply is not printed as the proof of the line:\n%s", text)
	}
}

// TestPrintsTheCostLineInThousands proves the header's second line, which the
// harness writes after every turn and the model never touches.
func TestPrintsTheCostLineInThousands(t *testing.T) {
	cases := []struct {
		cost   contract.CostLine
		wanted string
	}{
		{contract.CostLine{}, "this turn: 0.0k tokens in, 0.0k of them cached, 0.0k out"},
		{contract.CostLine{InputTokens: 6100, CachedInputTokens: 5200, OutputTokens: 400}, "this turn: 6.1k tokens in, 5.2k of them cached, 0.4k out"},
		{contract.CostLine{InputTokens: 123400, OutputTokens: 1000}, "this turn: 123.4k tokens in, 0.0k of them cached, 1.0k out"},
	}
	for _, one := range cases {
		record := contract.Record{Header: contract.Header{Kind: contract.RecordTask, ID: "1", Status: contract.StatusRunning, Cost: one.cost}}
		if text := string(Print(record)); !strings.Contains(text, one.wanted) {
			t.Errorf("the cost line for %+v is not %q:\n%s", one.cost, one.wanted, text)
		}
	}
}

// TestPrintsAHeaderWithNothingOptionalOnIt proves the header still reads when the
// origin and the next due task are unknown.
func TestPrintsAHeaderWithNothingOptionalOnIt(t *testing.T) {
	task := contract.Record{Header: contract.Header{Kind: contract.RecordTask, ID: "9", Status: contract.StatusWaiting, RoundsLeft: 5, MinutesLeft: 3}}
	if first := firstLine(string(Print(task))); first != "# task 9   waiting   budget left: 5 rounds, 3 minutes" {
		t.Errorf("the task header with no origin reads %q", first)
	}

	job := contract.Record{Header: contract.Header{Kind: contract.RecordJob, ID: "2", Status: contract.StatusStopped, TasksDone: 1, TasksTotal: 4}}
	if first := firstLine(string(Print(job))); first != "# job 2   stopped   1 of 4 tasks done" {
		t.Errorf("the job header with no next due task reads %q", first)
	}
}

// TestPrintsTextWithNewlinesFoldedOntoOneLine proves the record stays one item
// per line even when the user's message has more than one line in it.
func TestPrintsTextWithNewlinesFoldedOntoOneLine(t *testing.T) {
	record := contract.Record{
		Header: contract.Header{Kind: contract.RecordTask, ID: "1", Status: contract.StatusRunning},
		Goal:   contract.Goal{Ask: "first line\nsecond line\\here"},
	}
	if text := string(Print(record)); !strings.Contains(text, `Ask: "first line\nsecond line\\here"`) {
		t.Errorf("the ask was not folded onto one line:\n%s", text)
	}
}

// firstLine returns the first line of some text, which is the header of a record.
func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}
