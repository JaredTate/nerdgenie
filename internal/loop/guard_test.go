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
// third is refused, the fourth buys a rethink and a fresh window, and only when
// the model has stalled RewindsAllowed times and stalls again does the turn
// end. Each stall is on a file of its own, because the call a rethink was
// made over is closed for a while after it.
func TestRule4TheSameCallOverAndOverIsRefusedAndThenEndsTheTurn(t *testing.T) {
	stalls := loop.RewindsAllowed + 1
	steps := []testkit.Step{}
	answers := []string{}
	for stall := 0; stall < stalls; stall++ {
		path := fmt.Sprintf("notes%d.md", stall)
		for call := 1; call <= 4; call++ {
			steps = append(steps, sameReadOf(fmt.Sprintf("c%d", stall*4+call), path))
		}
		answers = append(answers, "the notes", "the notes")
		if stall < loop.RewindsAllowed {
			steps = append(steps, theUsualRethink())
		}
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
// not, and the third refusal ended the task. Now the stall buys a rethink and
// clears the conversation: the record stays, the stall is written into it as
// a failure from the model's own answer, and the model, reading the record,
// the newest results and its rethink, goes on with something different.
func TestARunOfTheSameCallClearsTheConversationAndWritesTheStallIntoTheRecord(t *testing.T) {
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the notes", "the notes", "the brand file")
	built := newHarness(t, []testkit.Step{
		sameReadAgain("c1"), sameReadAgain("c2"), sameReadAgain("c3"), sameReadAgain("c4"),
		aRethinkAnswer("notes.md read four times, saying the same thing", "the notes never change",
			"it is the wrong file, or the answer is in the brand file", "read brand.md"),
		callStep("I will read the brand file instead.", callFor("c5", "read", `{"path":"brand.md"}`)),
		answerStep("The notes and the brand file are read."),
	}, reading)

	outcome := built.ask(t, "read the notes")

	if outcome.Status == contract.StatusStopped {
		t.Fatalf("the task stopped on the first stall, and the first stall is a rethink instead")
	}
	if len(reading.Inputs()) != 3 {
		t.Errorf("the tool ran %d times, want 3: two of the stalled read and the different read after the rethink",
			len(reading.Inputs()))
	}
	requests := built.model.Requests()
	if len(requests) < 6 {
		t.Fatalf("the model was called %d times, want at least 6: four stalled rounds, the rethink, and the one after it", len(requests))
	}
	// The rethink opens a fresh window: the stalled rounds go, and so does
	// everything before them, because the record and the newest results in
	// full are what the model reads; the rethink line is the last message.
	after := requests[5]
	if !strings.Contains(theLastMessageOf(after), loop.TheRethinkLine) {
		t.Errorf("the call after the rethink does not carry the rethink line in its last message: %+v", after.Messages)
	}
	if _, keptTheFirstRound, keptAStalledRound := whatSurvivedTheCut(after); keptTheFirstRound || keptAStalledRound {
		t.Errorf("a round from before the rethink survived into the fresh window: %+v", after.Messages)
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

// whatSurvivedTheCut reads the request after a rewind for the rewind line, the
// first round's call c1, and any of the stalled calls c2 to c4.
func whatSurvivedTheCut(after contract.Request) (sawTheLine bool, keptTheFirstRound bool, keptAStalledRound bool) {
	for _, message := range after.Messages {
		if message.Text == loop.TheRewindLine {
			sawTheLine = true
		}
		for _, call := range message.ToolCalls {
			switch call.ID {
			case "c1":
				keptTheFirstRound = true
			case "c2", "c3", "c4":
				keptAStalledRound = true
			}
		}
	}
	return sawTheLine, keptTheFirstRound, keptAStalledRound
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

// stepsSpelling turns a pattern such as "ABABA" into the model's calls, one a
// round: an A is the same read of the notes every time, a B is a search, and
// the model answers after the last one.
func stepsSpelling(pattern string) []testkit.Step {
	steps := []testkit.Step{}
	for at, letter := range pattern {
		id := fmt.Sprintf("c%d", at+1)
		if letter == 'A' {
			steps = append(steps, sameReadAgain(id))
			continue
		}
		steps = append(steps, callStep("I will search the folder.", callFor(id, "search", `{"pattern":"date"}`)))
	}
	return append(steps, answerStep("The date is in the notes. Shall I go on?"))
}

// TestTheSameCallWithTheSameAnswerIsCaughtAcrossOtherCalls is what task 8 of
// the night of 6 September 2026 showed the first rule was blind to. The model
// alternated the same refused edit with a write, and the same read with a test
// run: nineteen identical refused edits and seventeen identical reads over
// twenty-seven minutes, and the rule never fired, because it counted only
// calls in a row. The same call with the same answer now counts anywhere in
// the window: A, B, A, B, A, where every A answers the same thing, and the
// third A is refused and not run.
func TestTheSameCallWithTheSameAnswerIsCaughtAcrossOtherCalls(t *testing.T) {
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the notes", "the notes", "the notes")
	built := newHarness(t, stepsSpelling("ABABA"), reading, scriptedTool("search", "3 matches", "3 matches"))

	outcome := built.ask(t, "find the date in the notes")

	if len(reading.Inputs()) != 2 {
		t.Errorf("the read ran %d times, want 2: the third read of the same file with the same answer is refused, though a search came between each",
			len(reading.Inputs()))
	}
	said := requestsJoined(built.model.Requests())
	if !strings.Contains(said, "Do something different") {
		t.Error("the model was never told to do something different")
	}
	if strings.Contains(said, loop.TheRewindLine) {
		t.Error("the third of the same call was already a rewind, and the third is only refused")
	}
	if outcome.Status == contract.StatusStopped {
		t.Errorf("the task ended stopped on %q, and a refused third call is not a stop", outcome.StopLine)
	}
}

// TestAFourthSameAnswerAcrossOtherCallsRewinds proves one more A after the
// refusal sets the rewind, the way a fourth in a row does: the refused third
// counts as one more asking, with no answer of its own.
func TestAFourthSameAnswerAcrossOtherCallsRewinds(t *testing.T) {
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the notes", "the notes", "the notes", "the notes")
	built := newHarness(t, stepsSpelling("ABABAA"), reading, scriptedTool("search", "3 matches", "3 matches"))

	outcome := built.ask(t, "find the date in the notes")

	if len(reading.Inputs()) != 2 {
		t.Errorf("the read ran %d times, want 2: the third and the fourth of the same call with the same answer are not run",
			len(reading.Inputs()))
	}
	// The fourth earns a stall, which is a rethink when the model answers
	// the rethink's question and the plain cut when it does not; either way
	// the model reads a fresh start rather than its own loop.
	joined := requestsJoined(built.model.Requests())
	if !strings.Contains(joined, loop.TheRethinkLine) && !strings.Contains(joined, loop.TheRewindLine) {
		t.Error("the model was never handed a rethink or the rewind line after the fourth of the same call with the same answer")
	}
	if outcome.Status == contract.StatusStopped {
		t.Errorf("the task ended stopped on %q, and the first stall clears the conversation rather than ending the task", outcome.StopLine)
	}
}

// TestAnEditAndTestCycleWithImprovingResultsIsNotARepeat holds the second rule
// where it stands: the same test command seven times in the window, an edit
// before each and a new answer every time, is honest work, and every run runs.
func TestAnEditAndTestCycleWithImprovingResultsIsNotARepeat(t *testing.T) {
	runs := loop.SameCallHardCap + 1
	steps := []testkit.Step{}
	testAnswers, editAnswers := []string{}, []string{}
	for run := 1; run <= runs; run++ {
		steps = append(steps,
			callStep("I will fix the code.", callFor(fmt.Sprintf("e%d", run), "edit",
				fmt.Sprintf(`{"path":"game.md","from":"try %d","to":"try %d"}`, run-1, run))),
			callStep("I will run the tests.", callFor(fmt.Sprintf("t%d", run), contract.ToolShell, `{"command":"npm test"}`)))
		testAnswers = append(testAnswers, fmt.Sprintf("run %d: %d still red", run, runs-run))
		editAnswers = append(editAnswers, "edited game.md")
	}
	steps = append(steps, answerStep("The tests are green. Shall I go on?"))
	testing := testkit.NewScriptedTool(contract.ToolSpec{
		Name: contract.ToolShell, Description: "A tool the test scripted, which answers with what the test gave it.",
	}, testAnswers...)
	built := newHarness(t, steps, testing, scriptedTool("edit", editAnswers...))

	outcome := built.ask(t, "make the tests pass")

	if len(testing.Inputs()) != runs {
		t.Errorf("the tests ran %d times, want all %d: a test run after each edit with a new answer every time is not a repeat",
			len(testing.Inputs()), runs)
	}
	if strings.Contains(requestsJoined(built.model.Requests()), "Do something different") {
		t.Error("the model was told to do something different in the middle of honest work")
	}
	if outcome.Status == contract.StatusStopped {
		t.Errorf("the task ended stopped on %q, and an edit-and-test cycle is not a stall", outcome.StopLine)
	}
}

// TestTheSameCallIsNotALoopWhenSomethingNewCameBetween: on run 24 the model
// clicked New Match between games, and every click answered the same fresh
// board, so the first rule read three resets as a loop and the guard stopped
// a task that was playing games. The same call with the same answer counts
// as a repeat only when nothing new came back in between: A, B, A, B, A where
// each B answers something the window has not seen is work, and the third A
// runs. The alternating loop above, where every B answers the same thing,
// is still caught.
func TestTheSameCallIsNotALoopWhenSomethingNewCameBetween(t *testing.T) {
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the notes", "the notes", "the notes")
	built := newHarness(t, stepsSpelling("ABABA"), reading, scriptedTool("search", "3 matches", "5 matches"))

	outcome := built.ask(t, "find the date in the notes")

	if len(reading.Inputs()) != 3 {
		t.Errorf("the read ran %d times, want 3: each search in between answered something new, so the reads are work, not a loop", len(reading.Inputs()))
	}
	if said := requestsJoined(built.model.Requests()); strings.Contains(said, "Do something different") {
		t.Error("the model was told to do something different, and it was doing something different between the reads")
	}
	if outcome.Status == contract.StatusStopped {
		t.Errorf("the task ended stopped on %q", outcome.StopLine)
	}
}
