package testkit_test

import (
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

func TestTheFixtureTurnsIntoAScriptTheFakeModelCanPlay(t *testing.T) {
	task := loadFixture(t)
	script := task.Script()

	if len(script.Steps) != len(task.Rounds) {
		t.Fatalf("the script has %d steps and the fixture has %d rounds", len(script.Steps), len(task.Rounds))
	}
	withTools := 0
	for _, step := range script.Steps {
		withTools += len(step.ToolCalls)
	}
	if withTools != len(task.Rounds)-1 {
		t.Errorf("the script asks for %d tool calls, want one for every round but the last", withTools)
	}
	if len(script.Steps[len(script.Steps)-1].ToolCalls) != 0 {
		t.Error("the last step of the script asks for a tool, and the last round is the report with tools off")
	}
}

func TestEveryRoundBeforeTheLastHasAResultWithTheNextIdentifier(t *testing.T) {
	task := loadFixture(t)

	results := task.ToolResults()
	if len(results) != len(task.Rounds)-1 {
		t.Fatalf("the fixture has %d results, want one for every round but the last", len(results))
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
