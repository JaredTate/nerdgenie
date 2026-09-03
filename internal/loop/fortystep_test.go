package loop_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/record"
	"github.com/JaredTate/coeus/internal/testkit"
)

// TestTheFortyStepFixtureRunsEndToEndThroughTheLoop is the proof the whole
// design rests on, driven this time through the loop itself: forty rounds
// against the fake model, the user's correction at round twelve, the stop
// condition firing at round thirty, the task picked up again when the user has
// dealt with it, and the fixture's own three assertions at the end.
func TestTheFortyStepFixtureRunsEndToEndThroughTheLoop(t *testing.T) {
	fixture, err := testkit.LoadFortyStepTask()
	if err != nil {
		t.Fatalf("cannot load the forty-step fixture: %v", err)
	}
	tools := &fixtureTools{fixture: fixture}
	built := newHarness(t, scriptForTheFixture(fixture), tools.registered()...)
	tools.holder = built

	stopped := runTheFixtureTask(t, built, fixture)
	finished := pickTheFixtureTaskUp(t, built, fixture, stopped.TaskID)

	if built.model.StepsLeft() != 0 {
		t.Errorf("%d scripted replies were never played, so the loop did not run the whole fixture", built.model.StepsLeft())
	}
	held := built.held(t, finished.TaskID)
	if err := fixture.CheckAskAndCorrections(held); err != nil {
		t.Errorf("the first assertion of the fixture fails: %v", err)
	}
	if err := fixture.CheckDoneList(held); err != nil {
		t.Errorf("the second assertion of the fixture fails: %v", err)
	}
	checkEveryResultReadsBack(t, built, fixture, finished.TaskID)
	checkTheResultsAreLabelledInOrder(t, fixture, held)
}

// runTheFixtureTask plays the first thirty rounds, which end with the stop
// condition the model wrote in round one firing on the login page.
func runTheFixtureTask(t *testing.T, built *harness, fixture testkit.FortyStepTask) loop.Outcome {
	t.Helper()
	outcome, err := built.loop.Run(t.Context(), loop.Task{
		Message: contract.Inbound{ID: "m1", Sender: "the user", Text: fixture.Ask, Channel: fixture.Origin},
		Channel: built.channel,
	})
	if err != nil {
		t.Fatalf("the loop could not run the fixture task: %v", err)
	}
	if outcome.Status != contract.StatusStopped {
		t.Fatalf("the task ended %q at the stop round, want stopped: %s", outcome.Status, outcome.Report)
	}
	if outcome.StopLine != fixture.StopWhen[0] {
		t.Errorf("the line that fired reads %q, and the fixture wrote %q first", outcome.StopLine, fixture.StopWhen[0])
	}
	return outcome
}

// pickTheFixtureTaskUp answers the stop the way the user does and runs the rest
// of the task, which ends with the done list proven.
func pickTheFixtureTaskUp(t *testing.T, built *harness, fixture testkit.FortyStepTask, taskID string) loop.Outcome {
	t.Helper()
	outcome, err := built.loop.Run(t.Context(), loop.Task{
		Message:  contract.Inbound{ID: "m3", Sender: "the user", Text: fixture.UserReplyAfterStop, Channel: fixture.Origin},
		Channel:  built.channel,
		ResumeID: taskID,
	})
	if err != nil {
		t.Fatalf("the loop could not pick the fixture task up again: %v", err)
	}
	if outcome.Status != contract.StatusDone {
		t.Fatalf("the picked-up task ended %q, want done: %s", outcome.Status, outcome.Report)
	}
	return outcome
}

// checkEveryResultReadsBack is the fixture's third assertion: every result the
// task produced can still be read in full by its id, long after the text itself
// left the model's window.
func checkEveryResultReadsBack(t *testing.T, built *harness, fixture testkit.FortyStepTask, taskID string) {
	t.Helper()
	keeper, err := record.Load(t.Context(), built.store, contract.RecordTask, taskID)
	if err != nil {
		t.Fatalf("cannot load the record of task %s: %v", taskID, err)
	}
	readBack := func(id string) (string, error) { return keeper.Read(t.Context(), id) }
	if err := fixture.CheckResultsReadable(readBack); err != nil {
		t.Errorf("the third assertion of the fixture fails: %v", err)
	}
}

// checkTheResultsAreLabelledInOrder proves the loop gave every result the label
// the fixture expects, in the order the rounds produced them, with the record
// write of a round labelled right after the call it rode with.
func checkTheResultsAreLabelledInOrder(t *testing.T, fixture testkit.FortyStepTask, held contract.Record) {
	t.Helper()
	wanted := fixture.ToolResults()
	if len(held.Work.Results) < len(wanted) {
		t.Fatalf("the record holds %d results and the fixture produced %d", len(held.Work.Results), len(wanted))
	}
	for at, result := range wanted {
		if held.Work.Results[at].ID != result.ID {
			t.Fatalf("result %d is labelled %q and the fixture calls it %q", at+1, held.Work.Results[at].ID, result.ID)
		}
	}
	last := fixture.ResultsOfRound(len(fixture.Rounds) - 1)
	if len(last) == 0 {
		t.Error("the fixture says round thirty-nine produced no results, and it runs a tool")
	}
}

// scriptForTheFixture is the fixture's own forty rounds with three replies of
// the test's own added: the review the stop earns, the record write that points
// the done list at the results that prove it once the harness has sent the model
// back, and the report and review that close the task.
func scriptForTheFixture(fixture testkit.FortyStepTask) []testkit.Step {
	rounds := fixture.Script().Steps
	steps := slices.Clone(rounds[:fixture.StopRound])
	steps = append(steps, aReviewReply("Keep watching for the login page on that account."))
	steps = append(steps, rounds[fixture.StopRound:]...)
	steps = append(steps,
		callStep("The harness sent me back, so I will name the result behind each done line.",
			taskCall("close", closingRecordWrite(fixture))),
		answerStep("Posted. What changed: one post. What I checked: the count and the page. What is left: nothing."),
		aReviewReply("Keep one fact per post."))
	return steps
}

// closingRecordWrite is the done list as it stands at the end, each line
// pointing at the result that proves it.
func closingRecordWrite(fixture testkit.FortyStepTask) string {
	lines := make([]map[string]any, 0, len(fixture.DoneWhen))
	for _, line := range fixture.DoneWhen {
		lines = append(lines, map[string]any{"text": line.Text, "done": true, "resultId": line.ResultID})
	}
	written, err := json.Marshal(map[string]any{"doneWhen": lines})
	if err != nil {
		return "{}"
	}
	return string(written)
}

// fixtureTools plays the fixture's tool results in order, whatever tool the
// round asked for, and hands the loop the user's correction while the tool of
// round twelve is running.
type fixtureTools struct {
	fixture testkit.FortyStepTask
	holder  *harness
	guard   sync.Mutex
	at      int
}

// registered is one tool per name the fixture uses, all sharing one place in
// the script.
func (tools *fixtureTools) registered() []contract.Tool {
	names := []string{}
	for _, round := range tools.fixture.Rounds {
		if round.ToolName != "" && !slices.Contains(names, round.ToolName) {
			names = append(names, round.ToolName)
		}
	}
	made := make([]contract.Tool, 0, len(names))
	for _, name := range names {
		made = append(made, &fixtureTool{name: name, tools: tools})
	}
	return made
}

// next returns the result of the round the fixture is up to, and refuses a call
// the fixture did not ask for.
func (tools *fixtureTools) next(name string) (string, error) {
	tools.guard.Lock()
	defer tools.guard.Unlock()
	for tools.at < len(tools.fixture.Rounds) && tools.fixture.Rounds[tools.at].ToolName == "" {
		tools.at++
	}
	if tools.at >= len(tools.fixture.Rounds) {
		return "", fmt.Errorf("the fixture has %d rounds and the loop asked for one more tool, so the loop ran past the script",
			len(tools.fixture.Rounds))
	}
	round := tools.fixture.Rounds[tools.at]
	tools.at++
	if round.ToolName != name {
		return "", fmt.Errorf("round %d of the fixture asks for %s and the loop ran %s, so the rounds have slipped",
			round.Number, round.ToolName, name)
	}
	if round.Number == tools.fixture.CorrectionRound {
		if err := tools.holder.loop.Deliver(contract.Inbound{
			ID: "m2", Sender: "the user", Text: tools.fixture.Correction, Channel: tools.fixture.Origin,
		}); err != nil {
			return "", err
		}
	}
	return round.ResultText, nil
}

// fixtureTool is one of the fixture's tools, which all share one place in the
// script.
type fixtureTool struct {
	name  string
	tools *fixtureTools
}

// Spec is what the model is told about this tool.
func (tool *fixtureTool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name:        tool.name,
		Description: "One of the tools the forty-step fixture drives, which answers with the result the fixture recorded.",
		Classes:     []contract.PermissionClass{contract.ClassRead},
	}
}

// Run answers with the fixture's next recorded result.
func (tool *fixtureTool) Run(_ context.Context, _ json.RawMessage) (contract.ToolOutput, error) {
	text, err := tool.tools.next(tool.name)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	return contract.ToolOutput{Text: text}, nil
}
