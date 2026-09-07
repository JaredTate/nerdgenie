package loop_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
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
	if !last.ToolsOff || len(last.Tools) == 0 {
		t.Error("the last call was made without the tools-off flag, or without the tools kept on the request for the cache")
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

// sameReadAgain is the model asking for the same read, which is the shape of
// every stall the guard is about.
func sameReadAgain(id string) testkit.Step {
	return callStep("I will read it again.", callFor(id, "read", `{"path":"notes.md"}`))
}

// TestRule4TheSameCallOverAndOverIsRefusedAndThenEndsTheTurn proves rule 4 of
// design section 3 across the rounds of one task: two identical calls run, the
// third is refused, the fourth clears the conversation, and only when the model
// has stalled RewindsAllowed times and stalls again does the turn end.
func TestRule4TheSameCallOverAndOverIsRefusedAndThenEndsTheTurn(t *testing.T) {
	stalls := loop.RewindsAllowed + 1
	steps := []testkit.Step{}
	answers := []string{}
	for stall := 0; stall < stalls; stall++ {
		for call := 1; call <= 4; call++ {
			steps = append(steps, sameReadAgain(fmt.Sprintf("c%d", stall*4+call)))
		}
		answers = append(answers, "the notes", "the notes")
	}
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, answers...)
	built := newHarness(t, steps, reading)

	outcome := built.ask(t, "read the notes")

	if len(reading.Inputs()) != 2*stalls {
		t.Errorf("the tool ran %d times, want %d: the first two identical calls of each stall are run and no more",
			len(reading.Inputs()), 2*stalls)
	}
	if !strings.Contains(requestsJoined(built.model.Requests()), "Do something different") {
		t.Error("the model was never told to do something different")
	}
	if outcome.Status != contract.StatusStopped {
		t.Errorf("the task ended %q, want stopped, because the model would not stop asking", outcome.Status)
	}
}

// TestARunOfTheSameCallClearsTheConversationAndWritesTheStallIntoTheRecord
// holds what the live game build needed. The model read one file four times
// running because a tool had told it a click worked when the page said it had
// not, and the third refusal ended the task. Now the stall clears the
// conversation instead: the record stays, the stall is written into it as a
// failure, and the model, reading the record and one line telling it what
// happened, goes on with something different.
func TestARunOfTheSameCallClearsTheConversationAndWritesTheStallIntoTheRecord(t *testing.T) {
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the notes", "the notes", "the brand file")
	built := newHarness(t, []testkit.Step{
		sameReadAgain("c1"), sameReadAgain("c2"), sameReadAgain("c3"), sameReadAgain("c4"),
		callStep("I will read the brand file instead.", callFor("c5", "read", `{"path":"brand.md"}`)),
		answerStep("The notes and the brand file are read."),
	}, reading)

	outcome := built.ask(t, "read the notes")

	if outcome.Status == contract.StatusStopped {
		t.Fatalf("the task stopped on the first stall, and the first stall clears the conversation instead")
	}
	if len(reading.Inputs()) != 3 {
		t.Errorf("the tool ran %d times, want 3: two of the stalled read and the different read after the clearing",
			len(reading.Inputs()))
	}
	requests := built.model.Requests()
	if len(requests) < 5 {
		t.Fatalf("the model was called %d times, want at least 5: four stalled rounds and the one after the clearing", len(requests))
	}
	after := requests[4]
	sawTheLine := false
	for _, message := range after.Messages {
		if message.Text == loop.TheRewindLine {
			sawTheLine = true
		}
		if message.Role == contract.RoleAssistant || len(message.ToolResults) > 0 {
			t.Errorf("the call after the clearing still carries the stalled rounds: %+v", message)
		}
	}
	if !sawTheLine {
		t.Errorf("the call after the clearing does not carry the rewind line as a message of its own: %+v", after.Messages)
	}
	if !strings.Contains(testkit.WholeRequestText(after), "read the notes") {
		t.Errorf("the ask left with the messages, and the record is what stands; the call reads:\n%s", testkit.WholeRequestText(after))
	}
	held := built.held(t, outcome.TaskID)
	stalled := false
	for _, failure := range held.Lessons.Failures {
		if strings.Contains(failure.Text, "stalled") && strings.Contains(failure.Text, "notes.md") {
			stalled = true
		}
	}
	if !stalled {
		t.Errorf("the record's failures read %+v, and a stall is written there naming the call so it is not forgotten", held.Lessons.Failures)
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

	if len(reading.Inputs()) != 2 {
		t.Errorf("the tool ran %d times inside one reply, and two identical calls in a row are run and no more",
			len(reading.Inputs()))
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
// the one wall the harness recognises for itself is checked against what a
// browser result shows, and a hit stops the task and tells the user the line of
// their own stop list that the wall answers, rather than the harness's words
// for it.
func TestTheStopListStopsTheTaskAndNamesTheLine(t *testing.T) {
	stopLine := "the account shows a login page or a captcha"
	built := newHarness(t, []testkit.Step{
		callStep("I will open the page.",
			callFor("c1", "browser_open", `{"url":"https://x.com/home"}`),
			taskCall("c1t", `{"stopWhen":["`+stopLine+`"],"doneWhen":["the post is up"]}`)),
		callStep("I will read the page.", callFor("c2", "browser_read", `{}`)),
		aReviewReply("Keep an eye out for the login page."),
		answerStep("This reply is never played, because the task stopped before it."),
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

// TestTheSameCallWithADifferentIntentIsStillTheSameCall is what the fifth game
// build showed the guard was blind to. The model asked the desktop tool to
// launch an application it never named eleven times in a row, every call
// refused with the same line, and the guard never saw a repeat because each
// call's intent and expectation, the free text that says why, was worded a
// little differently. What a call does is its fingerprint; what it says about
// itself is not.
func TestTheSameCallWithADifferentIntentIsStillTheSameCall(t *testing.T) {
	launching := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "computer", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "this call names no application, so say which program to open", "this call names no application, so say which program to open")
	built := newHarness(t, []testkit.Step{
		callStep("I will launch Chrome.", callFor("c1", "computer", `{"action":"launch","intent":"Launch Chrome to bring the game window to focus","expectation":"Chrome comes to the front"}`)),
		callStep("I will launch Chrome.", callFor("c2", "computer", `{"action":"launch","intent":"Launch Chrome to bring the game window to focus so I can interact with it","expectation":"Chrome window with the game page comes to the front"}`)),
		callStep("I will launch Chrome.", callFor("c3", "computer", `{"action":"launch","intent":"Launch Google Chrome to bring the game window to focus","expectation":"the game is in front"}`)),
		answerStep("I cannot bring the window forward; the page is on port 8091."),
	}, launching)

	built.ask(t, "open the game")

	if len(launching.Inputs()) != 2 {
		t.Errorf("the tool ran %d times, want 2: the third launch, worded differently but the same call, is refused",
			len(launching.Inputs()))
	}
	if !strings.Contains(requestsJoined(built.model.Requests()), "Do something different") {
		t.Error("the model was never told to do something different")
	}
}
