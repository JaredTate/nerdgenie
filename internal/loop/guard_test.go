package loop_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/testkit"
)

// TestRule3TheBudgetBuysOneLastCallWithTheToolsOff proves rule 3 of design
// section 3: when the rounds run out the model gets one call with no tools and
// the user gets the report.
func TestRule3TheBudgetBuysOneLastCallWithTheToolsOff(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read one.", callFor("c1", "read", `{"path":"one.md"}`)),
		callStep("I will read two.", callFor("c2", "read", `{"path":"two.md"}`)),
		answerStep("I read two files. What is left: the third one."),
	}, scriptedTool("read", "the first file", "the second file"))

	task := built.task("read the three files")
	task.Budget = loop.Budget{Rounds: 2}
	outcome, err := built.loop.Run(t.Context(), task)
	if err != nil {
		t.Fatalf("the loop could not run the task: %v", err)
	}

	if outcome.Status != contract.StatusStopped {
		t.Errorf("the task ended %q, want stopped, because its budget ran out", outcome.Status)
	}
	requests := built.model.Requests()
	last := requests[len(requests)-1]
	if !last.ToolsOff || len(last.Tools) != 0 {
		t.Error("the last call was made with the tools still on, and the final report is asked for with them off")
	}
	if !sentSomethingLike(built.channel.Sent(), "What is left: the third one") {
		t.Errorf("the user was sent %v, want the model's report of what is left", built.channel.Sent())
	}
}

// TestTheHourBudgetEndsTheTaskTheSameWay proves the other half of rule 3: the
// clock stops a task as surely as the rounds do.
func TestTheHourBudgetEndsTheTaskTheSameWay(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will wait.", callFor("c1", "read", `{"path":"one.md"}`)),
		answerStep("The hour is up. What is left: everything after the first file."),
	})
	built.tools.Add(&clockTool{name: "read", clock: built.clock, moves: 90 * time.Minute})

	task := built.task("read the files")
	task.Budget = loop.Budget{Time: time.Hour}
	outcome, err := built.loop.Run(t.Context(), task)
	if err != nil {
		t.Fatalf("the loop could not run the task: %v", err)
	}

	if outcome.Status != contract.StatusStopped {
		t.Errorf("the task ended %q, want stopped, because its hour was up", outcome.Status)
	}
	if !sentSomethingLike(built.channel.Sent(), "budget of 1h0m0s is used up") {
		t.Errorf("the user was sent %v, want a message saying the time budget ran out", built.channel.Sent())
	}
}

// clockTool moves the fake clock on, which is how a test spends a task's hour
// without waiting for one.
type clockTool struct {
	name  string
	clock *testkit.FakeClock
	moves time.Duration
}

// Spec is what the model is told about the clock tool.
func (tool *clockTool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name:        tool.name,
		Description: "A tool that takes a long time, so that a task's time budget can be seen running out.",
		Classes:     []contract.PermissionClass{contract.ClassRead},
	}
}

// Run moves the clock on and returns.
func (tool *clockTool) Run(_ context.Context, _ json.RawMessage) (contract.ToolOutput, error) {
	tool.clock.Advance(tool.moves)
	return contract.ToolOutput{Text: "that took a while"}, nil
}

// TestRule4TheSameCallOverAndOverIsRefusedAndThenEndsTheTurn proves rule 4 of
// design section 3 across the rounds of one task.
func TestRule4TheSameCallOverAndOverIsRefusedAndThenEndsTheTurn(t *testing.T) {
	same := func(id string) testkit.Step {
		return callStep("I will read it again.", callFor(id, "read", `{"path":"notes.md"}`))
	}
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the notes", "the notes")
	built := newHarness(t, []testkit.Step{same("c1"), same("c2"), same("c3"), same("c4")}, reading)

	outcome := built.ask(t, "read the notes")

	if len(reading.Inputs()) != loop.IdenticalCallsAllowed {
		t.Errorf("the tool ran %d times, and only the first %d identical calls in a row are run",
			len(reading.Inputs()), loop.IdenticalCallsAllowed)
	}
	if !strings.Contains(requestsJoined(built.model.Requests()), "Do something different") {
		t.Error("the model was never told to do something different")
	}
	if outcome.Status != contract.StatusStopped {
		t.Errorf("the task ended %q, want stopped, because the model would not stop asking", outcome.Status)
	}
}

// TestTheSameCallTwiceInsideOneReplyIsCaughtToo proves the detector reads a
// reply that asks for the same thing several times at once.
func TestTheSameCallTwiceInsideOneReplyIsCaughtToo(t *testing.T) {
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the notes", "the notes")
	built := newHarness(t, []testkit.Step{
		callStep("I will read it three times at once.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			callFor("c2", "read", `{"path":"notes.md"}`),
			callFor("c3", "read", `{"path":"notes.md"}`)),
		answerStep("I read it. Shall I go on?"),
	}, reading)

	built.ask(t, "read the notes")

	if len(reading.Inputs()) != loop.IdenticalCallsAllowed {
		t.Errorf("the tool ran %d times inside one reply, and only %d identical calls in a row are run",
			len(reading.Inputs()), loop.IdenticalCallsAllowed)
	}
}

// TestADifferentCallInBetweenClearsTheRun proves the detector is about a run of
// the same call and not about a call ever being made twice, because reading the
// same page again after doing something else is ordinary work.
func TestADifferentCallInBetweenClearsTheRun(t *testing.T) {
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "one", "two", "three", "four")
	built := newHarness(t, []testkit.Step{
		callStep("Read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		callStep("Read the notes.", callFor("c2", "read", `{"path":"notes.md"}`)),
		callStep("Read the brand file.", callFor("c3", "read", `{"path":"brand.md"}`)),
		callStep("Read the notes again.", callFor("c4", "read", `{"path":"notes.md"}`)),
		answerStep("I have read them. Shall I go on?"),
	}, reading)

	built.ask(t, "read the files")

	if len(reading.Inputs()) != 4 {
		t.Errorf("the tool ran %d times, want all four, because a different call in between clears the run", len(reading.Inputs()))
	}
}

// TestTheStopListStopsTheTaskAndNamesTheLine proves the guard's first check:
// every line of the record's stop list is checked against what the harness can
// see, and a hit stops the task and tells the user which line.
func TestTheStopListStopsTheTaskAndNamesTheLine(t *testing.T) {
	stopLine := "the account shows a login page or a captcha"
	built := newHarness(t, []testkit.Step{
		callStep("I will open the page.",
			callFor("c1", "browser_open", `{"url":"https://x.com/home"}`),
			taskCall("c1t", `{"stopWhen":["`+stopLine+`"],"doneWhen":["the post is up"]}`)),
		callStep("I will read the page.", callFor("c2", "browser_read", `{}`)),
		answerStep("I am stuck. Shall I stop?"),
	},
		scriptedTool("browser_open", "the home timeline is open"),
		scriptedTool("browser_read", "The page is a login page. It asks you to log in with a username and a password."),
	)

	outcome := built.ask(t, "post the tweet")

	if outcome.StopLine != stopLine {
		t.Errorf("the line that fired reads %q, want the one the model wrote", outcome.StopLine)
	}
	if outcome.Status != contract.StatusStopped {
		t.Errorf("the task ended %q, want stopped", outcome.Status)
	}
	if !sentSomethingLike(built.channel.Sent(), stopLine) {
		t.Errorf("the user was sent %v, and a stopped task says which line fired", built.channel.Sent())
	}
	if built.model.StepsLeft() != 1 {
		t.Error("the model was called again after the stop fired, and nothing runs after a stop")
	}
}

// TestTheHarnessAddsItsOwnStopLineForALoginWall proves the second of the two
// lines the harness adds to every stop list.
func TestTheHarnessAddsItsOwnStopLineForALoginWall(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the page.", callFor("c1", "browser_read", `{}`)),
		answerStep("Shall I go on?"),
	}, scriptedTool("browser_read", "This page is a captcha, so it wants to know whether you are a person."))

	outcome := built.ask(t, "post the tweet")

	if outcome.StopLine != loop.HarnessStopLineWall {
		t.Errorf("the line that fired reads %q, want the harness's own line about a wall", outcome.StopLine)
	}
}

// sentSomethingLike says whether any message the user was sent holds the words.
func sentSomethingLike(sent []string, wanted string) bool {
	for _, message := range sent {
		if strings.Contains(message, wanted) {
			return true
		}
	}
	return false
}
