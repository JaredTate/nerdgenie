package record

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
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

// TestARecordIsRefusedTheChangeThatWouldTakeItPastItsSize is finding 22 of the
// wave 6 gate review. MaxRecordTokens was documented as "the size a record never
// passes" and nothing anywhere enforced it: a long ask, a handful of corrections
// and the hundred result lines the budget allows came to about 3,305 tokens and
// not one write was refused. A record over its size is worse than a slow one,
// because the working-context builder then refuses the whole prompt with
// ErrNoRoomForTheRecord and the loop turns that into a task that cannot run at
// all. So the promise is now a rule, checked where every other rule is checked.
func TestARecordIsRefusedTheChangeThatWouldTakeItPastItsSize(t *testing.T) {
	keeper := recordFilledToTheBudget(t)
	ctx := t.Context()

	// The results give first, so a change the results can make room for lands;
	// a change no trimming can make room for is refused, naming the part.
	plan := []string{}
	for range MaxPlanSteps {
		plan = append(plan, strings.Repeat("a very long step ", 100))
	}
	err := keeper.Apply(ctx, Update{Plan: plan})
	if err == nil {
		t.Fatalf("a plan of four thousand words was written into a record promised to stay under %d tokens, and it now counts as %d",
			MaxRecordTokens, EstimateTokens(keeper.Text()))
	}
	if !errors.Is(err, ErrRecordTooLarge) {
		t.Fatalf("the write was refused, but not for its size: %v", err)
	}
	t.Logf("the refusal reads: %v", err)
	if !strings.Contains(err.Error(), "plan") {
		t.Errorf("the refusal does not name the part to shorten, so nobody knows what to do about it: %v", err)
	}
	if counted := EstimateTokens(keeper.Text()); counted > MaxRecordTokens {
		t.Errorf("the refused change was written anyway: the record counts as %d tokens and the limit is %d", counted, MaxRecordTokens)
	}
	if held := keeper.Record(); len(held.Work.Results) < MinResultLinesKept {
		t.Errorf("the refused change cost the record its results: %d lines are left", len(held.Work.Results))
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

	// The plan is as long as a task's may be, which is the ten steps one sitting
	// works through; a longer one is a job's task list in disguise and is refused.
	plan := []string{}
	for step := range MaxPlanSteps {
		plan = append(plan, fmt.Sprintf("step %d of the plan, which says what to do in about eight words", step+1))
	}
	// The done list is as long as a task's may be, which is the five lines a
	// sitting can prove; a longer one is a job and is refused.
	done := []contract.DoneLine{}
	for line := range MaxDoneLines {
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

// TestTheResultsListTrimsItsOldestLinesSoALongTaskNeverOverflowsTheRecord
// is what killed the fifth game build at round 223. Every tool result adds a
// line to the record, the lines were never dropped, and at about two hundred
// of them a harness write of the situation took the record past its size and
// the task failed on the harness's own bookkeeping. The list of results is
// the one part of a record that grows with the work, so it is the part that
// gives: when a change would take the record past its size, the oldest result
// lines leave the record first, and every one of them is still in the log to
// be read back by its label.
func TestTheResultsListTrimsItsOldestLinesSoALongTaskNeverOverflowsTheRecord(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()
	summary := strings.Repeat("word ", 12) + "tests: all 62 passing"
	for at := 1; at <= 260; at++ {
		if _, err := keeper.AddResult(ctx, summary, fmt.Sprintf("the whole text of result %d", at)); err != nil {
			t.Fatalf("result %d was refused: %v, and a task's results never overflow its record", at, err)
		}
		if err := keeper.SetSituation(ctx, []string{"files changed in this task: engine.js and 30 more", "tests: all 62 passing", "where the work stands: round " + fmt.Sprint(at)}); err != nil {
			t.Fatalf("the situation after result %d was refused: %v, and the harness's own bookkeeping never fails a task", at, err)
		}
	}

	held := keeper.Record()
	if got := EstimateTokens(string(Print(withoutTheAsk(held)))); got > MaxRecordTokens {
		t.Errorf("the record is about %d tokens after 260 results, and it is promised to stay under %d", got, MaxRecordTokens)
	}
	if len(held.Work.Results) == 0 || held.Work.Results[len(held.Work.Results)-1].ID != "r260" {
		t.Fatalf("the newest result is not the last line of the list: %+v", held.Work.Results)
	}
	if len(held.Work.Results) > MaxResultLinesKept || held.Work.Results[0].ID == "r1" {
		t.Errorf("the list holds %d lines beginning with %s, and the oldest lines leave first", len(held.Work.Results), held.Work.Results[0].ID)
	}
	first, err := keeper.Read(ctx, "r1")
	if err != nil || first != "the whole text of result 1" {
		t.Errorf("read r1 gave %q (%v), and a result that left the record is still in the log", first, err)
	}
}
