package record

import (
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// The two numbers behind the size promise of the design: with a budget of a
// hundred rounds a record can hold at most a hundred result lines, so it stays
// small enough that nothing in it is ever squashed or summarized.
const (
	// TokensPerHundredWords is the ratio this package counts tokens with: a
	// hundred words of plain English are about a hundred and thirty tokens. It
	// is an estimate and it is deliberately the only one here, so that the size
	// of a record is always measured the same way. The working-context builder
	// of wave 2 keeps its own estimate for whole prompts.
	TokensPerHundredWords = 130
	// MaxRecordTokens is the size a record never passes, which is what lets the
	// design promise that the record fits in front of any model.
	MaxRecordTokens = 3000
	// MaxSummaryCharacters is the longest the one line a result keeps in the
	// record may be. It is the bound that makes the promise above hold at the
	// budget: a hundred rounds can write a hundred result lines and no more, and
	// each of those lines is this long at most. Nothing is lost by the cut,
	// because the whole text of every result is in the log.
	MaxSummaryCharacters = 70
	// MaxAskTokens is the share of a prompt the ask may take before the model is
	// shown the start of it and told how to read the rest. A quarter leaves room
	// for the work, the lessons and the results beside it.
	//
	// The ask is the user's own words, so it is never cut or rewritten where it
	// is kept: MaxRecordTokens counts everything in a record except the ask, and
	// this is the ask's own rule, held only in what the model is shown.
	MaxAskTokens = MaxRecordTokens / 4
)

// AskLabel is what the model writes to read the whole ask back, the way it reads
// a past result with `read r7`. The read tool routes it to the record.
const AskLabel = "ask"

// AskCutNote opens the one line that stands where the rest of a long ask would
// be. It is the words a reader can search a prompt for, and the line that
// follows says how much more there is and how to fetch it.
const AskCutNote = "[the ask goes on:"

// cutToOneLine keeps a result's summary to the one line a record holds for it,
// and says plainly that it was cut.
func cutToOneLine(summary string) string {
	letters := []rune(summary)
	if len(letters) <= MaxSummaryCharacters {
		return summary
	}
	return string(letters[:MaxSummaryCharacters-3]) + "..."
}

// EstimateTokens counts the tokens in a piece of text by splitting it on white
// space and applying the ratio above. It is an estimate, not a tokenizer: the
// point is a number that can be compared with MaxRecordTokens on every model.
func EstimateTokens(text string) int {
	return len(strings.Fields(text)) * TokensPerHundredWords / 100
}

// checkItStillFits holds the size promise above as a rule rather than a hope. It
// runs beside the round-trip check on every change, so a write that would take
// the record past the size it is promised to stay under is refused before it
// lands, and the refusal names the longest part so that there is something to do
// about it.
// The ask is left out of the count on purpose. It is the user's own words, it is
// never cut or rewritten in storage, and a task whose ask alone filled the
// record would otherwise be a task that could record nothing at all, which is
// worse than a task that is slow. The ask has its own rule instead, in
// askForTheModel, held only in what the model is shown.
func checkItStillFits(held contract.Record) error {
	counted := EstimateTokens(string(Print(withoutTheAsk(held))))
	if counted <= MaxRecordTokens {
		return nil
	}
	name, cost := longestPartOf(held)
	return fmt.Errorf("%w: everything in it but the ask would be about %d tokens and the limit is %d, and its longest part is the %s at about %d",
		ErrRecordTooLarge, counted, MaxRecordTokens, name, cost)
}

// withoutTheAsk is the record with a one-word ask in place of the user's, so
// that the size of everything else can be measured on its own. It stands in
// rather than being emptied because a record with no ask does not print.
func withoutTheAsk(held contract.Record) contract.Record {
	held.Goal.Ask = "-"
	return held
}

// longestPartOf names the part of a record with the most words in it, and says
// how many tokens that part costs. It is what turns "too long" into "shorten
// this".
func longestPartOf(held contract.Record) (string, int) {
	name, cost := "record", 0
	for _, part := range []struct {
		name string
		text string
	}{
		{"why", held.Goal.Why},
		{"done list", linesOfDone(held.Goal.DoneWhen)},
		{"stop list", strings.Join(held.Rules.StopWhen, "\n")},
		{"corrections", linesOfCorrections(held.Rules.Corrections)},
		{"situation", strings.Join(held.Work.Situation, "\n")},
		{"plan", linesOfPlan(held.Work.Plan)},
		{"task list", linesOfTasks(held.Work.Tasks)},
		{"list of results", linesOfResults(held.Work.Results)},
		{"decisions", linesOfDecisions(held.Lessons.Decisions)},
		{"failures", linesOfFailures(held.Lessons.Failures)},
	} {
		if counted := EstimateTokens(part.text); counted > cost {
			name, cost = part.name, counted
		}
	}
	return name, cost
}

// The five list readers below join one list of a record into text, so that
// longestPartOf can weigh every part of a record the same way.

func linesOfDone(lines []contract.DoneLine) string {
	joined := make([]string, 0, len(lines))
	for _, line := range lines {
		joined = append(joined, line.Text)
	}
	return strings.Join(joined, "\n")
}

func linesOfCorrections(lines []contract.Correction) string {
	joined := make([]string, 0, len(lines))
	for _, line := range lines {
		joined = append(joined, line.Text)
	}
	return strings.Join(joined, "\n")
}

func linesOfPlan(steps []contract.PlanStep) string {
	joined := make([]string, 0, len(steps))
	for _, step := range steps {
		joined = append(joined, step.Text)
	}
	return strings.Join(joined, "\n")
}

func linesOfTasks(tasks []contract.JobTask) string {
	joined := make([]string, 0, len(tasks))
	for _, task := range tasks {
		joined = append(joined, task.Text)
	}
	return strings.Join(joined, "\n")
}

func linesOfResults(results []contract.ResultLine) string {
	joined := make([]string, 0, len(results))
	for _, result := range results {
		joined = append(joined, result.Summary)
	}
	return strings.Join(joined, "\n")
}

func linesOfDecisions(decisions []contract.Decision) string {
	joined := make([]string, 0, len(decisions))
	for _, decision := range decisions {
		joined = append(joined, decision.Text+" "+decision.Reason)
	}
	return strings.Join(joined, "\n")
}

func linesOfFailures(failures []contract.Failure) string {
	joined := make([]string, 0, len(failures))
	for _, failure := range failures {
		joined = append(joined, failure.Text+" "+failure.Cause)
	}
	return strings.Join(joined, "\n")
}
