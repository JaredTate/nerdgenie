package record

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// TestEstimatesTokensFromWords proves the one ratio this package counts with.
func TestEstimatesTokensFromWords(t *testing.T) {
	cases := map[string]int{
		"":                     0,
		"one":                  1,
		"one two three":        3,
		"one two three four\n": 5,
	}
	for text, wanted := range cases {
		if counted := EstimateTokens(text); counted != wanted {
			t.Errorf("%q counts as %d tokens, and the ratio makes it %d", text, counted, wanted)
		}
	}
}

// TestARecordFilledToTheBudgetStaysUnderThreeThousandTokens is the size promise
// of the design: with a budget of a hundred rounds a record can hold at most a
// hundred result lines, so it never needs squashing or summarizing.
func TestARecordFilledToTheBudgetStaysUnderThreeThousandTokens(t *testing.T) {
	keeper := recordFilledToTheBudget(t)
	text := keeper.Text()
	counted := EstimateTokens(text)

	t.Logf("a record filled to the hundred-round budget is %d characters and about %d tokens", len(text), counted)
	if counted >= MaxRecordTokens {
		t.Errorf("a record filled to the budget counts as %d tokens, and the design promises under %d:\n%s",
			counted, MaxRecordTokens, text)
	}
	if held := keeper.Record(); len(held.Work.Results) != 100 {
		t.Errorf("the filled record holds %d results, and a hundred rounds were spent", len(held.Work.Results))
	}
}

// TestCutsAResultSummaryToOneLine proves the bound that makes the size promise
// hold: a result keeps one line in the record however much the tool returned, and
// the whole text is in the log either way.
func TestCutsAResultSummaryToOneLine(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()
	long := strings.Repeat("a very wordy summary that will not fit on one line. ", 10)

	id, err := keeper.AddResult(ctx, long, "the whole text")
	if err != nil {
		t.Fatalf("cannot add the result: %v", err)
	}
	line := keeper.Record().Work.Results[0]
	if len([]rune(line.Summary)) != MaxSummaryCharacters {
		t.Errorf("the summary of %s is %d characters, and a summary is cut to %d",
			id, len([]rune(line.Summary)), MaxSummaryCharacters)
	}
	if !strings.HasSuffix(line.Summary, "...") {
		t.Errorf("the summary that was cut does not say so: %q", line.Summary)
	}
	if text, err := keeper.Read(ctx, id); err != nil || text != "the whole text" {
		t.Errorf("the whole text was not kept in the log: %q with the error %v", text, err)
	}
}

// recordFilledToTheBudget spends a whole hundred-round budget on one record: a
// result every round, each with a summary as long as one is allowed to be, and a
// plan, a done list, corrections, decisions, and failures on top.
func recordFilledToTheBudget(t *testing.T) *Keeper {
	t.Helper()
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()

	plan := []string{}
	for step := range 12 {
		plan = append(plan, fmt.Sprintf("step %d of the plan, which says what to do in about eight words", step+1))
	}
	done := []contract.DoneLine{}
	for line := range 6 {
		done = append(done, contract.DoneLine{Text: fmt.Sprintf("done line %d, one thing that must be true at the end", line+1)})
	}
	stop := []string{}
	for line := range 5 {
		stop = append(stop, fmt.Sprintf("stop line %d, one thing that must stop the work at once", line+1))
	}
	if err := keeper.Apply(ctx, Update{Why: "why the user wants this, in one line", DoneWhen: done, StopWhen: stop, Plan: plan}); err != nil {
		t.Fatalf("cannot write the model's half of the filled record: %v", err)
	}
	fillTheLessons(t, keeper)

	for round := range 100 {
		summary := fmt.Sprintf("round %d ", round+1) + strings.Repeat("summary word ", 20)
		if _, err := keeper.AddResult(ctx, summary, "the whole text of the result, which lives in the log"); err != nil {
			t.Fatalf("cannot add the result of round %d: %v", round+1, err)
		}
	}
	return keeper
}

// fillTheLessons writes the corrections, decisions, and failures a hard task
// gathers, which is the rest of what a record holds at the end of its budget.
func fillTheLessons(t *testing.T, keeper *Keeper) {
	t.Helper()
	ctx := t.Context()
	for correction := range 5 {
		said := fmt.Sprintf("no, do it the other way round, correction number %d", correction+1)
		if _, err := keeper.AddCorrection(ctx, said); err != nil {
			t.Fatalf("cannot add correction %d: %v", correction+1, err)
		}
	}
	for lesson := range 8 {
		update := Update{
			Decision: &NewDecision{
				Text:   fmt.Sprintf("decision %d, the choice made in about ten words", lesson+1),
				Reason: fmt.Sprintf("the reason for decision %d, in about ten words", lesson+1),
			},
			Failure: &NewFailure{
				Text:  fmt.Sprintf("failure %d, what went wrong in about ten words", lesson+1),
				Cause: fmt.Sprintf("the cause of failure %d, in about ten words", lesson+1),
			},
		}
		if err := keeper.Apply(ctx, update); err != nil {
			t.Fatalf("cannot add lesson %d: %v", lesson+1, err)
		}
	}
}
