package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestAStopReportedWithEveryDoneLineMarkedIsADone: GLM 5.3 wrote "the full
// suite is green" into its own stop list, then met it, and the harness
// recorded a finished task as stopped and the job put it down. The stop list
// is for the things that must reach the person; a task whose done list is all
// proved has nothing to stop for, so a stop reported then is a done, and the
// report says the stop line was taken as the finish.
func TestAStopReportedWithEveryDoneLineMarkedIsADone(t *testing.T) {
	stopLine := "the full suite is green"
	built := newHarness(t, []testkit.Step{
		callStep("I will run the tests.",
			callFor("c1", "read", `{"path":"tests.log"}`),
			taskCall("c1t", `{"why":"the user wants the tests green","stopWhen":["`+stopLine+`"],"doneWhen":["the tests pass"]}`)),
		callStep("The suite is green, so the done line is proved and my stop line has come true.",
			taskCall("c2t", `{"doneWhen":[{"text":"the tests pass","done":true,"resultId":"r1"}]}`),
			taskCall("c2s", `{"operation":"stop_now","text":"`+stopLine+`"}`)),
		aReviewReply("A finish line belongs on the done list, not the stop list."),
		answerStep("This reply is never played, because the task ended on the stop."),
	}, scriptedTool("read", "all 12 tests passing"))

	outcome := built.ask(t, "make the tests pass")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done: every done line was marked when the stop was reported (%s)", outcome.Status, outcome.Report)
	}
	if outcome.StopLine != stopLine {
		t.Errorf("the outcome names the stop line as %q, want the one the model reported", outcome.StopLine)
	}
	if held := built.held(t, outcome.TaskID); held.Header.Status != contract.StatusDone {
		t.Errorf("the record stands at %q, want done", held.Header.Status)
	}
	sent := built.channel.Sent()
	if !sentSomethingLike(sent, stopLine) || !sentSomethingLike(sent, "taken as the finish") {
		t.Errorf("the person was sent %v, want the report to say the stop line was taken as the finish", sent)
	}
	if sentSomethingLike(sent, "I stopped this task") {
		t.Errorf("the person was sent %v, and a task with every done line proved was not stopped", sent)
	}
	if built.model.StepsLeft() < 1 {
		t.Error("the model was called again after the stop was taken as the finish")
	}
}

// TestAStopReportedWithADoneLineUnmarkedStaysAStop is the other half: the
// stop list still stops a task whose done list is not all proved.
func TestAStopReportedWithADoneLineUnmarkedStaysAStop(t *testing.T) {
	stopLine := "the full suite is green"
	built := newHarness(t, []testkit.Step{
		callStep("I will run the tests.",
			callFor("c1", "read", `{"path":"tests.log"}`),
			taskCall("c1t", `{"why":"the user wants the tests green","stopWhen":["`+stopLine+`"],"doneWhen":["the tests pass","the change is committed"]}`)),
		callStep("The suite is green, so my stop line has come true.",
			taskCall("c2t", `{"doneWhen":[{"text":"the tests pass","done":true,"resultId":"r1"},{"text":"the change is committed"}]}`),
			taskCall("c2s", `{"operation":"stop_now","text":"`+stopLine+`"}`)),
		aReviewReply("Commit before reporting."),
		answerStep("This reply is never played, because the task stopped."),
	}, scriptedTool("read", "all 12 tests passing"))

	outcome := built.ask(t, "make the tests pass and commit")

	if outcome.Status != contract.StatusStopped || outcome.StopLine != stopLine {
		t.Errorf("the task ended %+v, want it stopped on the reported line, because a done line is still unproved", outcome)
	}
	if !strings.Contains(outcome.Report, "I stopped this task") {
		t.Errorf("the report reads %q, want the stopped report", outcome.Report)
	}
}
