package record

import (
	"reflect"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestTheFortyStepFixtureRunsThroughTheRecord is the proof the design rests on,
// run for the first time here without a loop or a model: for each of the forty
// rounds the scripted task operations and the scripted tool result go into the
// record, the user's correction arrives during round twelve, the stop condition
// fires on round thirty, and at the end the fixture's own three assertions hold.
func TestTheFortyStepFixtureRunsThroughTheRecord(t *testing.T) {
	fixture, err := testkit.LoadFortyStepTask()
	if err != nil {
		t.Fatalf("cannot load the forty-step fixture: %v", err)
	}
	keeper, _ := newKeeper(t, Start{
		Kind: contract.RecordTask, ID: fixture.TaskID, Origin: fixture.Origin,
		Ask: fixture.Ask, RoundsLeft: 100, MinutesLeft: 60,
	})
	ctx := t.Context()

	for _, round := range fixture.Rounds {
		runFixtureRound(t, keeper, fixture, round)
	}
	closeTheFixtureTask(t, keeper, fixture)

	held := keeper.Record()
	if err := fixture.CheckAskAndCorrections(held); err != nil {
		t.Errorf("the first assertion of the fixture fails: %v", err)
	}
	if err := fixture.CheckDoneList(held); err != nil {
		t.Errorf("the second assertion of the fixture fails: %v", err)
	}
	readBack := func(id string) (string, error) { return keeper.Read(ctx, id) }
	if err := fixture.CheckResultsReadable(readBack); err != nil {
		t.Errorf("the third assertion of the fixture fails: %v", err)
	}
	if wanted := fixtureRecordAtTheEnd(t, fixture); !reflect.DeepEqual(held, wanted) {
		t.Errorf("the record at the end is not the one the fixture describes.\nwant %+v\ngot  %+v", wanted, held)
	}
}

// runFixtureRound puts one round of the fixture through the record: the user's
// correction if this is the round it arrives in, what the model wrote into the
// record, the result the tool returned, and the budget the harness spent.
func runFixtureRound(t *testing.T, keeper *Keeper, fixture testkit.FortyStepTask, round testkit.FortyStepRound) {
	t.Helper()
	ctx := t.Context()

	if round.Number == fixture.CorrectionRound {
		if _, err := keeper.AddCorrection(ctx, fixture.Correction); err != nil {
			t.Fatalf("cannot add the user's correction at round %d: %v", round.Number, err)
		}
	}
	if err := keeper.Apply(ctx, updateFromFixture(t, round.TaskUpdate)); err != nil {
		t.Fatalf("the model's writing at round %d was refused: %v", round.Number, err)
	}
	if round.ToolName == "" {
		return
	}
	// A round's tool result and, when the round wrote to the record, the record
	// write's own result both get a line, in the order the fixture numbers them.
	for _, result := range fixture.ResultsOfRound(round.Number) {
		if _, err := keeper.AddResult(ctx, result.Summary, result.Text); err != nil {
			t.Fatalf("cannot add the result %s of round %d: %v", result.ID, round.Number, err)
		}
	}
	if round.Number == fixture.StopRound {
		checkTheStopConditionFires(t, keeper, fixture, round)
	}
	if err := keeper.SetBudget(ctx, Budget{RoundsLeft: 100 - round.Number, MinutesLeft: 60}); err != nil {
		t.Fatalf("cannot spend a round of the budget at round %d: %v", round.Number, err)
	}
}

// checkTheStopConditionFires proves the stop list the model wrote in round one is
// the one the round-thirty result sets off, and that the harness stops the work
// and starts it again when the user has dealt with it.
func checkTheStopConditionFires(t *testing.T, keeper *Keeper, fixture testkit.FortyStepTask, round testkit.FortyStepRound) {
	t.Helper()
	ctx := t.Context()

	fired := stopLineTheResultFires(keeper.Record().Rules.StopWhen, round.ResultText)
	if fired == "" {
		t.Fatalf("round %d returned %q and no stop line in the record catches it", round.Number, round.ResultText)
	}
	if fired != fixture.StopWhen[0] {
		t.Errorf("the stop line that fired reads %q, and the fixture wrote %q first", fired, fixture.StopWhen[0])
	}
	if err := keeper.SetStatus(ctx, contract.StatusStopped); err != nil {
		t.Fatalf("cannot stop the task when its stop condition fired: %v", err)
	}
	if err := keeper.SetStatus(ctx, contract.StatusRunning); err != nil {
		t.Fatalf("cannot start the task again after the user replied %q: %v", fixture.UserReplyAfterStop, err)
	}
}

// stopLineTheResultFires returns the line of the stop list this result sets off,
// which is the one whose words are in the page the tool brought back.
func stopLineTheResultFires(stopWhen []string, resultText string) string {
	said := strings.ToLower(resultText)
	for _, line := range stopWhen {
		for _, phrase := range []string{"login page", "captcha"} {
			if strings.Contains(line, phrase) && strings.Contains(said, phrase) {
				return line
			}
		}
	}
	return ""
}

// closeTheFixtureTask is the last thing the model does: it points every line of
// the done list at the result that proves it, and the harness closes the record.
func closeTheFixtureTask(t *testing.T, keeper *Keeper, fixture testkit.FortyStepTask) {
	t.Helper()
	ctx := t.Context()
	if err := keeper.Apply(ctx, Update{DoneWhen: fixtureDoneLines(fixture)}); err != nil {
		t.Fatalf("cannot point the done list at its results: %v", err)
	}
	if err := keeper.SetStatus(ctx, contract.StatusDone); err != nil {
		t.Fatalf("the done-check would not let the finished task close: %v", err)
	}
}

// fixtureDoneLines is the done list as it stands at the end, each line pointing
// at the result that proves it.
func fixtureDoneLines(fixture testkit.FortyStepTask) []contract.DoneLine {
	lines := make([]contract.DoneLine, 0, len(fixture.DoneWhen))
	for _, line := range fixture.DoneWhen {
		lines = append(lines, contract.DoneLine{Text: line.Text, Done: true, ResultID: line.ResultID})
	}
	return lines
}

// updateFromFixture turns one round's scripted task operations into the update
// the task tool would hand this package.
func updateFromFixture(t *testing.T, written map[string]any) Update {
	t.Helper()
	update := Update{}
	for key, value := range written {
		switch key {
		case "why":
			update.Why = fixtureText(t, value)
		case "doneWhen":
			for _, line := range fixtureList(t, value) {
				update.DoneWhen = append(update.DoneWhen, contract.DoneLine{Text: line})
			}
		case "stopWhen":
			update.StopWhen = fixtureList(t, value)
		case "plan":
			update.Plan = fixtureList(t, value)
		case "decision":
			pair := fixturePair(t, value)
			update.Decision = &NewDecision{Text: pair["text"], Reason: pair["reason"]}
		case "failure":
			pair := fixturePair(t, value)
			update.Failure = &NewFailure{Text: pair["text"], Cause: pair["cause"]}
		default:
			t.Fatalf("the fixture writes %q into the record, and this test does not know what that is", key)
		}
	}
	return update
}

// fixtureText reads one piece of text out of the fixture.
func fixtureText(t *testing.T, value any) string {
	t.Helper()
	text, isText := value.(string)
	if !isText {
		t.Fatalf("the fixture holds %v where this test wants a line of text", value)
	}
	return text
}

// fixtureList reads a list of lines out of the fixture.
func fixtureList(t *testing.T, value any) []string {
	t.Helper()
	items, isList := value.([]any)
	if !isList {
		t.Fatalf("the fixture holds %v where this test wants a list of lines", value)
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, fixtureText(t, item))
	}
	return lines
}

// fixturePair reads a decision or a failure, which is two named pieces of text.
func fixturePair(t *testing.T, value any) map[string]string {
	t.Helper()
	fields, isPair := value.(map[string]any)
	if !isPair {
		t.Fatalf("the fixture holds %v where this test wants a decision or a failure", value)
	}
	pair := map[string]string{}
	for name, held := range fields {
		pair[name] = fixtureText(t, held)
	}
	return pair
}

// fixtureRecordAtTheEnd is what this test says the record should hold after the
// forty rounds, built from the fixture data alone.
func fixtureRecordAtTheEnd(t *testing.T, fixture testkit.FortyStepTask) contract.Record {
	results := []contract.ResultLine{}
	for _, result := range fixture.ToolResults() {
		results = append(results, contract.ResultLine{ID: result.ID, Summary: result.Summary})
	}
	plan := []contract.PlanStep{}
	for at, step := range fixturePlan(t, fixture) {
		plan = append(plan, contract.PlanStep{Number: at + 1, Text: step})
	}
	return contract.Record{
		Header: contract.Header{
			Kind: contract.RecordTask, ID: fixture.TaskID, Status: contract.StatusDone,
			Origin: fixture.Origin, RoundsLeft: 100 - lastRoundWithATool(fixture), MinutesLeft: 60,
		},
		Goal: contract.Goal{Ask: fixture.Ask, Why: fixture.Why, DoneWhen: fixtureDoneLines(fixture)},
		Rules: contract.Rules{
			Corrections: []contract.Correction{{ID: contract.CorrectionID(1), Text: fixture.Correction}},
			StopWhen:    fixture.StopWhen,
		},
		Work:    contract.Work{Plan: plan, Results: results},
		Lessons: fixtureLessons(t, fixture),
	}
}

// fixturePlan is the plan the model wrote, whichever round it wrote it in.
func fixturePlan(t *testing.T, fixture testkit.FortyStepTask) []string {
	t.Helper()
	for _, round := range fixture.Rounds {
		if written, found := round.TaskUpdate["plan"]; found {
			return fixtureList(t, written)
		}
	}
	return nil
}

// fixtureLessons are the decisions and failures the model wrote, in the rounds it
// wrote them in.
func fixtureLessons(t *testing.T, fixture testkit.FortyStepTask) contract.Lessons {
	t.Helper()
	lessons := contract.Lessons{}
	for _, round := range fixture.Rounds {
		if written, found := round.TaskUpdate["decision"]; found {
			pair := fixturePair(t, written)
			lessons.Decisions = append(lessons.Decisions, contract.Decision{
				ID: contract.DecisionID(len(lessons.Decisions) + 1), Text: pair["text"], Reason: pair["reason"],
			})
		}
		if written, found := round.TaskUpdate["failure"]; found {
			pair := fixturePair(t, written)
			lessons.Failures = append(lessons.Failures, contract.Failure{
				ID: contract.FailureID(len(lessons.Failures) + 1), Text: pair["text"], Cause: pair["cause"],
			})
		}
	}
	return lessons
}

// TestTheFortyStepRecordStaysSmallAndReadable proves the two promises the fixture
// is there to test, on the record it leaves behind.
func TestTheFortyStepRecordStaysSmallAndReadable(t *testing.T) {
	fixture, err := testkit.LoadFortyStepTask()
	if err != nil {
		t.Fatalf("cannot load the forty-step fixture: %v", err)
	}
	text := string(Print(fixtureRecordAtTheEnd(t, fixture)))
	if counted := EstimateTokens(text); counted >= MaxRecordTokens {
		t.Errorf("the record after forty rounds counts as %d tokens", counted)
	}
	t.Logf("the record after the forty-step fixture is about %d tokens", EstimateTokens(text))

	parsed, err := Parse([]byte(text))
	if err != nil {
		t.Fatalf("the record after forty rounds does not read back: %v", err)
	}
	if !reflect.DeepEqual(parsed, fixtureRecordAtTheEnd(t, fixture)) {
		t.Error("the record after forty rounds does not survive the trip through its text form")
	}

}

// lastRoundWithATool is the number of the last round that ran a tool, which is
// the last round the driver spends budget on.
func lastRoundWithATool(fixture testkit.FortyStepTask) int {
	last := 0
	for _, round := range fixture.Rounds {
		if round.ToolName != "" {
			last = round.Number
		}
	}
	return last
}
