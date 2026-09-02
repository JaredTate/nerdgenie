package testkit_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// loadFixture reads the forty-step fixture or fails the test.
func loadFixture(t *testing.T) testkit.FortyStepTask {
	t.Helper()
	task, err := testkit.LoadFortyStepTask()
	if err != nil {
		t.Fatalf("loading the forty-step fixture failed: %v", err)
	}
	return task
}

func TestTheFortyStepFixtureHasFortyRounds(t *testing.T) {
	task := loadFixture(t)

	if len(task.Rounds) != 40 {
		t.Errorf("the fixture has %d rounds, want 40", len(task.Rounds))
	}
	for at, round := range task.Rounds {
		if round.Number != at+1 {
			t.Errorf("the round at position %d says it is number %d", at, round.Number)
		}
		if round.Orient == "" {
			t.Errorf("round %d has no orient line, and the model writes one before every action", round.Number)
		}
	}
	last := task.Rounds[len(task.Rounds)-1]
	if last.ToolName != "" {
		t.Errorf("the last round calls the tool %q, and it is supposed to be the final report with tools off", last.ToolName)
	}
	if last.Reply == "" {
		t.Error("the last round has no reply, and it is supposed to be the final report")
	}
}

func TestTheFortyStepFixtureCarriesTheAskAndTheWhyFromTheDesign(t *testing.T) {
	task := loadFixture(t)

	if !strings.Contains(task.Ask, "DigiByte anniversary") || !strings.Contains(task.Ask, "280 characters") {
		t.Errorf("the ask is %q, want the tweet example from design section 4", task.Ask)
	}
	if task.Why == "" {
		t.Error("the fixture has no why, and the why is what lets the model act when the plan breaks")
	}
	if len(task.DoneWhen) < 2 {
		t.Errorf("the done list has %d lines, want the two the design shows", len(task.DoneWhen))
	}
	if len(task.StopWhen) < 2 {
		t.Errorf("the stop list has %d lines, want the two the design shows", len(task.StopWhen))
	}
}

func TestTheCorrectionArrivesAtRoundTwelveWordForWord(t *testing.T) {
	task := loadFixture(t)

	if task.CorrectionRound != 12 {
		t.Errorf("the correction arrives at round %d, want 12", task.CorrectionRound)
	}
	if task.Correction != "no, lead with the date not the features" {
		t.Errorf("the correction is %q, want it word for word from the design", task.Correction)
	}
}

func TestTheStopConditionFiresAtRoundThirtyAndTheUserLetsItResume(t *testing.T) {
	task := loadFixture(t)

	if task.StopRound != 30 {
		t.Errorf("the stop condition fires at round %d, want 30", task.StopRound)
	}
	round := task.Rounds[task.StopRound-1]
	if !strings.Contains(strings.ToLower(round.ResultText), "log in") && !strings.Contains(strings.ToLower(round.ResultText), "login") {
		t.Errorf("the result of round %d is %q, want the login page that fires the stop condition", task.StopRound, round.ResultText)
	}
	if task.UserReplyAfterStop == "" {
		t.Error("the fixture has no user reply after the stop, and the task cannot resume without one")
	}
}

// carrying builds the one request the fixture tests drive the fake model with:
// a system block holding whatever the harness is supposed to still be carrying.
func carrying(pieces ...string) contract.Request {
	return contract.Request{SystemBlocks: []contract.SystemBlock{{
		Name: "record",
		Text: strings.Join(pieces, "\n"),
	}}}
}

func TestAScriptDrivenWithoutTheUsersReplyFailsAtTheRoundAfterTheStop(t *testing.T) {
	task := loadFixture(t)
	model := testkit.NewFakeModel(task.Script())
	request := carrying(task.Ask, task.Correction)

	for round := 1; round <= task.StopRound; round++ {
		if _, err := model.Send(context.Background(), request, nil); err != nil {
			t.Fatalf("round %d failed, and the harness had not lost anything yet: %v", round, err)
		}
	}

	_, err := model.Send(context.Background(), request, nil)

	if err == nil {
		t.Fatalf("round %d ran without the user's reply, and the task waits until they answer", task.StopRound+1)
	}
	if !strings.Contains(err.Error(), task.UserReplyAfterStop) {
		t.Errorf("the failure after the stop does not name the reply the harness lost: %v", err)
	}
}

func TestTheSameScriptRunsToTheEndOnceTheUserHasReplied(t *testing.T) {
	task := loadFixture(t)
	model := testkit.NewFakeModel(task.Script())
	before := carrying(task.Ask, task.Correction)
	after := carrying(task.Ask, task.Correction, task.UserReplyAfterStop)

	for round := 1; round <= len(task.Rounds); round++ {
		request := before
		if round > task.StopRound {
			request = after
		}
		if _, err := model.Send(context.Background(), request, nil); err != nil {
			t.Fatalf("round %d failed with the reply in the request: %v", round, err)
		}
	}

	if model.StepsLeft() != 0 {
		t.Errorf("the script has %d steps left after forty rounds", model.StepsLeft())
	}
}

func TestTheFixtureTurnsIntoAScriptTheFakeModelCanPlay(t *testing.T) {
	task := loadFixture(t)
	script := task.Script()

	if len(script.Steps) != len(task.Rounds) {
		t.Fatalf("the script has %d steps and the fixture has %d rounds", len(script.Steps), len(task.Rounds))
	}
	calls := 0
	for _, step := range script.Steps {
		calls += len(step.ToolCalls)
	}
	if calls != toolCallsInTheFixture(task) {
		t.Errorf("the script asks for %d tool calls, want %d: one for every round but the last, plus one record write per round that writes",
			calls, toolCallsInTheFixture(task))
	}
	if len(script.Steps[len(script.Steps)-1].ToolCalls) != 0 {
		t.Error("the last step of the script asks for a tool, and the last round is the report with tools off")
	}
}

// toolCallsInTheFixture is how many tool calls the whole fixture asks for: one
// for each round that uses a tool, and one more for each round that writes to
// the record, because the model writes the record in the same reply.
func toolCallsInTheFixture(task testkit.FortyStepTask) int {
	calls := 0
	for _, round := range task.Rounds {
		if round.ToolName != "" {
			calls++
		}
		if round.TaskUpdate != nil {
			calls++
		}
	}
	return calls
}

func TestTheScriptWritesToTheRecordThroughTheTaskTool(t *testing.T) {
	task := loadFixture(t)
	script := task.Script()

	writes := 0
	for at, round := range task.Rounds {
		step := script.Steps[at]
		if round.TaskUpdate == nil {
			for _, call := range step.ToolCalls {
				if call.Name == contract.ToolTask {
					t.Errorf("round %d writes nothing to the record, and the script asks for the task tool anyway", round.Number)
				}
			}
			continue
		}
		writes++
		if len(step.ToolCalls) != 2 {
			t.Errorf("round %d writes to the record, so the step wants two tool calls and it has %d",
				round.Number, len(step.ToolCalls))
			continue
		}
		second := step.ToolCalls[1]
		if second.Name != contract.ToolTask {
			t.Errorf("the second tool call of round %d is %q, want the task tool", round.Number, second.Name)
			continue
		}
		for key := range round.TaskUpdate {
			if !strings.Contains(string(second.Input), key) {
				t.Errorf("the task call of round %d does not carry %q, so the record write is not what the fixture says",
					round.Number, key)
			}
		}
	}
	if writes < 4 {
		t.Errorf("the fixture writes to the record %d times, want the why and done list, the plan, the failure, and the decision", writes)
	}
}

func TestEveryRoundBeforeTheLastHasAResultWithTheNextIdentifier(t *testing.T) {
	task := loadFixture(t)

	results := task.ToolResults()
	if len(results) != toolCallsInTheFixture(task) {
		t.Fatalf("the fixture has %d results, want one for every tool call the script makes, which is %d",
			len(results), toolCallsInTheFixture(task))
	}
	for at, result := range results {
		if result.ID != contract.ResultID(at+1) {
			t.Errorf("the result at position %d has the id %q, want %q", at, result.ID, contract.ResultID(at+1))
		}
		if result.Summary == "" || result.Text == "" {
			t.Errorf("the result %s has no summary or no full text, and the record needs one line and the log needs the rest", result.ID)
		}
	}
}

func TestEveryDoneLinePointsAtAResultTheFixtureActuallyProduced(t *testing.T) {
	task := loadFixture(t)

	produced := map[string]bool{}
	for _, result := range task.ToolResults() {
		produced[result.ID] = true
	}
	for _, line := range task.DoneWhen {
		if !produced[line.ResultID] {
			t.Errorf("the done line %q points at %s, and the fixture never produced a result with that id",
				line.Text, line.ResultID)
		}
	}
}

func TestTheFinishedRecordCarriesThePlanTheDecisionAndTheFailure(t *testing.T) {
	task := loadFixture(t)

	record := task.RecordAtTheEnd()

	if len(record.Work.Plan) != 10 {
		t.Errorf("the finished record has %d plan steps, want the ten the model wrote at round 4", len(record.Work.Plan))
	}
	for at, step := range record.Work.Plan {
		if step.Number != at+1 || step.Text == "" {
			t.Errorf("plan step at position %d is numbered %d and says %q", at, step.Number, step.Text)
		}
		// The fixture never ticks a plan step: the harness ticks one when it sees
		// the step finish, and the fixture says nothing about that. The record
		// this fixture leaves behind therefore has an unticked plan, and this must
		// say the same as what internal/record ends up holding.
		if step.Done || step.ResultID != "" {
			t.Errorf("plan step %d is ticked, and the fixture never ticks one: %+v", step.Number, step)
		}
	}

	if len(record.Lessons.Decisions) != 1 {
		t.Fatalf("the finished record holds %d decisions, want the one from round 12", len(record.Lessons.Decisions))
	}
	decision := record.Lessons.Decisions[0]
	if decision.ID != contract.DecisionID(1) || decision.Text == "" || decision.Reason == "" {
		t.Errorf("the decision is %+v, and the task tool refuses a decision with no reason", decision)
	}

	if len(record.Lessons.Failures) != 1 {
		t.Fatalf("the finished record holds %d failures, want the one from round 6", len(record.Lessons.Failures))
	}
	failure := record.Lessons.Failures[0]
	if failure.ID != contract.FailureID(1) || failure.Text == "" || failure.Cause == "" {
		t.Errorf("the failure is %+v, and the task tool refuses a failure with no cause", failure)
	}
}

func TestOneRoundsRecordWriteReadsAsTheContractsOwnShapes(t *testing.T) {
	task := loadFixture(t)

	first, err := task.Rounds[0].Update()
	if err != nil {
		t.Fatalf("reading round one's record write failed: %v", err)
	}
	if first.Why != task.Why || len(first.DoneWhen) != 2 || len(first.StopWhen) != 2 {
		t.Errorf("round one writes %+v, want the why, the done list, and the stop list", first)
	}

	planned, err := task.Rounds[3].Update()
	if err != nil {
		t.Fatalf("reading round four's record write failed: %v", err)
	}
	if len(planned.Plan) != 10 {
		t.Errorf("round four writes %d plan steps, want the ten the design shows", len(planned.Plan))
	}

	failed, err := task.Rounds[5].Update()
	if err != nil {
		t.Fatalf("reading round six's record write failed: %v", err)
	}
	if failed.Failure == nil || failed.Failure.Text == "" || failed.Failure.Cause == "" {
		t.Errorf("round six writes %+v, want the failure with its cause", failed.Failure)
	}

	decided, err := task.Rounds[11].Update()
	if err != nil {
		t.Fatalf("reading round twelve's record write failed: %v", err)
	}
	if decided.Decision == nil || decided.Decision.Text == "" || decided.Decision.Reason == "" {
		t.Errorf("round twelve writes %+v, want the decision with its reason", decided.Decision)
	}

	quiet, err := task.Rounds[1].Update()
	if err != nil {
		t.Fatalf("reading round two's record write failed: %v", err)
	}
	if quiet.Why != "" || quiet.Plan != nil || quiet.Decision != nil || quiet.Failure != nil {
		t.Errorf("round two writes %+v, and it writes nothing to the record", quiet)
	}
}

func TestARecordWriteTheLoaderDoesNotKnowIsRefused(t *testing.T) {
	round := testkit.FortyStepRound{Number: 1, TaskUpdate: map[string]any{"nonsense": "something new"}}

	_, err := round.Update()

	if err == nil {
		t.Fatal("a record write nobody knows how to read was accepted, and the fixture and its reader must stay in step")
	}
	if !strings.Contains(err.Error(), "nonsense") {
		t.Errorf("the refusal does not name what it could not read: %v", err)
	}
}

func TestTheDoneLinesAtTheEndAreTheOnesTheFixtureWrote(t *testing.T) {
	task := loadFixture(t)

	lines := task.DoneLinesAtTheEnd()

	if len(lines) != len(task.DoneWhen) {
		t.Fatalf("the done list has %d lines, want the %d the fixture wrote", len(lines), len(task.DoneWhen))
	}
	for at, line := range lines {
		if line.Text != task.DoneWhen[at].Text || line.ResultID != task.DoneWhen[at].ResultID || !line.Done {
			t.Errorf("the done line at position %d is %+v, want the fixture's own", at, line)
		}
	}
}

func TestTheAskAndCorrectionCheckCatchesAnEditedRecord(t *testing.T) {
	task := loadFixture(t)

	good := task.RecordAtTheEnd()
	if err := task.CheckAskAndCorrections(good); err != nil {
		t.Fatalf("the record the fixture built does not pass its own check: %v", err)
	}

	edited := task.RecordAtTheEnd()
	edited.Goal.Ask = edited.Goal.Ask + " Also make it funny."
	if err := task.CheckAskAndCorrections(edited); err == nil {
		t.Error("an edited ask passed the check, and the ask is never edited by anyone")
	}

	rewritten := task.RecordAtTheEnd()
	rewritten.Rules.Corrections[0].Text = "lead with the date"
	if err := task.CheckAskAndCorrections(rewritten); err == nil {
		t.Error("a rewritten correction passed the check, and a correction is kept word for word")
	}
}

func TestTheDoneCheckCatchesALineWithNothingBehindIt(t *testing.T) {
	task := loadFixture(t)

	if err := task.CheckDoneList(task.RecordAtTheEnd()); err != nil {
		t.Fatalf("the finished record does not pass the done-check: %v", err)
	}

	unproven := task.RecordAtTheEnd()
	unproven.Goal.DoneWhen[0].ResultID = ""
	unproven.Goal.DoneWhen[0].UserReply = ""
	if err := task.CheckDoneList(unproven); err == nil {
		t.Error("a done line with nothing behind it passed the done-check, and that is what the check is for")
	}
}

func TestEveryResultIdentifierIsStillReadableAtTheEnd(t *testing.T) {
	task := loadFixture(t)

	held := map[string]string{}
	for _, result := range task.ToolResults() {
		held[result.ID] = result.Text
	}
	read := func(id string) (string, error) {
		text, found := held[id]
		if !found {
			return "", errNoSuchResult
		}
		return text, nil
	}

	if err := task.CheckResultsReadable(read); err != nil {
		t.Fatalf("a result the fixture wrote cannot be read back: %v", err)
	}

	delete(held, contract.ResultID(7))
	if err := task.CheckResultsReadable(read); err == nil {
		t.Error("a result that has gone missing passed the check, and every result must stay readable by its id")
	}
}

// errNoSuchResult is what a test's reader returns for an id it does not hold.
var errNoSuchResult = missingResult{}

// missingResult is the error a test's reader returns.
type missingResult struct{}

// Error says what went wrong and what to do about it.
func (missingResult) Error() string {
	return "there is no result with that id in the log, so check the record"
}
