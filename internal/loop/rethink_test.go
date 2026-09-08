// The tests of the rethink. On the night of 6 September 2026 the three-bit
// model was told by the rewind line to do something different, wrote "I have
// been looping, time to stop circling" on its first line for fifty rounds, and
// made the same call on every one of them. Words do not change what a small
// model does; what is in front of it does. So a stall now buys one call with
// the tools off over the record and the thing the model kept asking for, the
// answer goes into the record and in front of the model, and the call it was
// repeating is closed for a while.
package loop_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/orientation"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// sameReadOf is the model asking for the same read of one file, which is the
// shape of every stall the guard is about.
func sameReadOf(id string, path string) testkit.Step {
	return callStep("I will read it again.", callFor(id, "read", `{"path":"`+path+`"}`))
}

// aRethinkAnswer is the model's answer to the rethink's question: the five
// labelled lines, one each.
func aRethinkAnswer(showed string, symptom string, causes string, next string) testkit.Step {
	return answerStep("Learning: whether the file holds the answer.\nShowed: " + showed +
		"\nSymptom: " + symptom + "\nCauses: " + causes + "\nNext: " + next)
}

// theUsualRethink is a rethink answer for a test that only needs one given.
func theUsualRethink() testkit.Step {
	return aRethinkAnswer("the same file four times, saying the same thing", "the file does not change",
		"it is the wrong file, or the answer is in another one", "read brand.md")
}

// theRethinkRequests are the model calls made with the tools off whose last
// message is the rethink's question.
func theRethinkRequests(built *harness) []contract.Request {
	found := []contract.Request{}
	for _, request := range built.model.Requests() {
		if len(request.Messages) == 0 {
			continue
		}
		last := request.Messages[len(request.Messages)-1]
		if request.ToolsOff && last.Text == loop.TheRethinkQuestion {
			found = append(found, request)
		}
	}
	return found
}

// aFailureBeginning says whether the record holds a failure whose text begins
// with the words.
func aFailureBeginning(held contract.Record, words string) bool {
	for _, failure := range held.Lessons.Failures {
		if strings.HasPrefix(failure.Text, words) {
			return true
		}
	}
	return false
}

// aDecisionBeginning finds the decision on the record whose text begins with
// the words.
func aDecisionBeginning(held contract.Record, words string) (contract.Decision, bool) {
	for _, decision := range held.Lessons.Decisions {
		if strings.HasPrefix(decision.Text, words) {
			return decision, true
		}
	}
	return contract.Decision{}, false
}

// theMessagesAround finds the message of a request whose text carries the
// words, and the message before it. The working context puts the record's
// parts around the conversation as messages of their own, so the loop's own
// last message is found by what it says rather than by its place.
func theMessagesAround(request contract.Request, words string) (before contract.Message, found contract.Message, there bool) {
	for at, message := range request.Messages {
		if !strings.Contains(message.Text, words) {
			continue
		}
		if at > 0 {
			before = request.Messages[at-1]
		}
		return before, message, true
	}
	return contract.Message{}, contract.Message{}, false
}

// theLastMessageOf is the text of the loop's own last message in a request,
// which is the one carrying the rethink line after a rethink.
func theLastMessageOf(request contract.Request) string {
	_, found, _ := theMessagesAround(request, loop.TheRethinkLine)
	return found.Text
}

// TestARunOfTheSameCallRunsOneRethinkWithTheToolsOff: the fourth of the same
// call used to cut the conversation and hand the model the rewind line. Now
// it buys one call with the tools off, in a small window of its own, whose
// background is the record, the newest result in full, and the sentence that
// every cause on the record was tried; the answer's five lines go into the
// record as a failure and a decision through the record's own rules; and the
// conversation after is a fresh window whose one message is the rethink and
// the instruction to do its Next line.
func TestARunOfTheSameCallRunsOneRethinkWithTheToolsOff(t *testing.T) {
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the notes say the meeting is at noon", "the notes say the meeting is at noon", "the brand file names the room")
	built := newHarness(t, []testkit.Step{
		sameReadOf("c1", "notes.md"), sameReadOf("c2", "notes.md"), sameReadOf("c3", "notes.md"), sameReadOf("c4", "notes.md"),
		aRethinkAnswer("the notes say the meeting is at noon, four times over", "the notes never name the room",
			"the room is in the brand file, or nobody wrote it down", "read brand.md"),
		callStep("I will read the brand file, as the rethink says.", callFor("c5", "read", `{"path":"brand.md"}`)),
		answerStep("The room is in the brand file."),
	}, reading)

	outcome := built.ask(t, "read the notes and find the room")

	if outcome.Status == contract.StatusStopped {
		t.Fatalf("the task stopped on the first stall, and the first stall is a rethink")
	}
	rethinks := theRethinkRequests(built)
	if len(rethinks) != 1 {
		t.Fatalf("the model was asked the rethink's question %d times with the tools off, want once", len(rethinks))
	}
	asked := rethinks[0]
	if len(asked.Messages) != 2 {
		t.Errorf("the rethink's request carries %d messages, want two: the background and the question", len(asked.Messages))
	}
	background := asked.Messages[0].Text
	for _, words := range []string{"the notes say the meeting is at noon", "read the notes and find the room", "r2", "was tried and did not hold"} {
		if !strings.Contains(background, words) {
			t.Errorf("the rethink's background does not carry %q:\n%s", words, background)
		}
	}
	held := built.held(t, outcome.TaskID)
	if !aFailureBeginning(held, "stalled: the notes say the meeting is at noon, four times over") {
		t.Errorf("the record's failures read %+v, want the stall written from the Showed line", held.Lessons.Failures)
	}
	decision, written := aDecisionBeginning(held, "Rethink: next read brand.md")
	if !written || !strings.HasPrefix(decision.Reason, "two causes: the room is in the brand file") {
		t.Errorf("the record's decisions read %+v, want the Next line as a decision with the two causes as its reason", held.Lessons.Decisions)
	}
	first, _ := requestsCarrying(built, loop.TheRethinkLine)
	if first != 5 {
		t.Fatalf("the rethink line first rode on model call %d, want call 5: the one right after the rethink's own", first)
	}
	after := built.model.Requests()[first]
	opening, last, _ := theMessagesAround(after, loop.TheRethinkLine)
	if !strings.HasSuffix(strings.TrimSpace(last.Text), "Do the Next line now.") {
		t.Errorf("the fresh window's last message does not end with the instruction to do the Next line:\n%s", last.Text)
	}
	if !strings.Contains(last.Text, "Next: read brand.md") {
		t.Errorf("the fresh window's last message does not hold the model's whole answer:\n%s", last.Text)
	}
	if _, keptTheFirstRound, keptAStalledRound := whatSurvivedTheCut(after); keptTheFirstRound || keptAStalledRound {
		t.Errorf("a round from before the rethink survived into the fresh window: %+v", after.Messages)
	}
	if !strings.HasPrefix(opening.Text, orientation.TheHeading) || !strings.Contains(opening.Text, "the notes say the meeting is at noon") {
		t.Errorf("the fresh window does not open with the orientation and the newest results in full:\n%s", opening.Text)
	}
	if len(reading.Inputs()) != 3 {
		t.Errorf("the tool ran %d times, want 3: two of the stalled read and the Next line after the rethink", len(reading.Inputs()))
	}
}

// TestTwentyRoundsWithoutProgressRunOneRethinkToo: the meter's path is the
// same rethink. Twenty rounds in which nothing measurable moved used to cut
// the stalled rounds; now they buy the one call, the record gains the
// failure and the decision, and the fresh window opens on the answer.
func TestTwentyRoundsWithoutProgressRunOneRethinkToo(t *testing.T) {
	trip := loop.RewindAfterRoundsWithoutProgress + 1
	steps := []testkit.Step{}
	outputs := []string{}
	for at := 1; at <= trip; at++ {
		steps = append(steps, anEditRound(at))
		outputs = append(outputs, fmt.Sprintf("edited /game/src/engine.js by 1 line (%d)", at))
	}
	steps = append(steps,
		aRethinkAnswer("twenty edits of engine.js and the test never went green", "the test reads a value the edits never touch",
			"the test imports a stale copy, or the value is set in another file", "run grep -n stale /game/src"),
		answerStep("Found it. What changed: the engine. What I checked: the tests. What is left: nothing."))
	built := newHarness(t, steps, scriptedTool(contract.ToolEdit, outputs...))

	outcome := built.ask(t, "make the tests pass")

	rethinks := theRethinkRequests(built)
	if len(rethinks) != 1 {
		t.Fatalf("the model was asked the rethink's question %d times with the tools off, want once", len(rethinks))
	}
	newest := fmt.Sprintf("edited /game/src/engine.js by 1 line (%d)", trip)
	if background := rethinks[0].Messages[0].Text; !strings.Contains(background, newest) {
		t.Errorf("the rethink's background does not carry the newest result in full:\n%s", background)
	}
	held := built.held(t, outcome.TaskID)
	if !aFailureBeginning(held, "stalled: twenty edits of engine.js") {
		t.Errorf("the record's failures read %+v, want the stall written from the Showed line", held.Lessons.Failures)
	}
	if _, written := aDecisionBeginning(held, "Rethink: next run grep -n stale"); !written {
		t.Errorf("the record's decisions read %+v, want the Next line as a decision", held.Lessons.Decisions)
	}
	first, _ := requestsCarrying(built, loop.TheRethinkLine)
	if first != trip+1 {
		t.Fatalf("the rethink line first rode on model call %d, want call %d: the one right after the rethink's own", first, trip+1)
	}
	if last := theLastMessageOf(built.model.Requests()[first]); !strings.HasSuffix(strings.TrimSpace(last), "Do the Next line now.") {
		t.Errorf("the fresh window's last message does not end with the instruction to do the Next line:\n%s", last)
	}
	if outcome.Status != contract.StatusDone {
		t.Errorf("the task ended %q, want done in the model's own words after the rethink", outcome.Status)
	}
}

// TestTheCallTheModelRepeatedIsClosedForTenRounds: the model's first line
// said "time to stop circling" for fifty rounds while it made the same call.
// After the rethink that call is refused before the detector sees it, with a
// line saying the answer is on the record and which line to do instead, for
// ten rounds; on the eleventh it runs again.
func TestTheCallTheModelRepeatedIsClosedForTenRounds(t *testing.T) {
	steps := []testkit.Step{
		sameReadOf("c1", "notes.md"), sameReadOf("c2", "notes.md"), sameReadOf("c3", "notes.md"), sameReadOf("c4", "notes.md"),
		theUsualRethink(),
	}
	for at := 1; at <= loop.ClosedForRounds; at++ {
		steps = append(steps, sameReadOf(fmt.Sprintf("d%d", at), "notes.md"))
	}
	steps = append(steps, sameReadOf("open", "notes.md"), answerStep("The notes are read."))
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the notes", "the notes", "the notes")
	built := newHarness(t, steps, reading)

	built.ask(t, "read the notes")

	if len(reading.Inputs()) != 3 {
		t.Errorf("the tool ran %d times, want 3: two before the rethink and the one after the call reopened", len(reading.Inputs()))
	}
	closedLine := "You already have that; it is on the record as r2. Do the Next line of your rethink instead."
	freshWindow, _ := requestsCarrying(built, loop.TheRethinkLine)
	first, _ := requestsCarrying(built, closedLine)
	if freshWindow < 0 || first != freshWindow+1 {
		t.Fatalf("the closed line first rode on model call %d, want call %d: the one after the first repeat on the fresh window", first, freshWindow+1)
	}
	// The round after the call reopened carries every result since the fresh
	// window, each inside its tool-result boundary: the ten refusals and the
	// run.
	requests := built.model.Requests()
	refused, ran := 0, 0
	for _, message := range requests[freshWindow+loop.ClosedForRounds+1].Messages {
		for _, result := range message.ToolResults {
			switch {
			case strings.Contains(result.Text, closedLine):
				refused++
			case result.CallID == "open" && strings.Contains(result.Text, "the notes"):
				ran++
			}
		}
	}
	if refused != loop.ClosedForRounds {
		t.Errorf("the closed call was refused %d times, want %d: once a round while it was closed", refused, loop.ClosedForRounds)
	}
	if ran != 1 {
		t.Error("the call did not run again once the ten rounds had passed")
	}
	if after := requestsJoined(requests[freshWindow:]); strings.Contains(after, "Do something different") {
		t.Error("the detector saw the closed call, and a closed call is refused before the detector")
	}
}

// TestARethinkTheModelCannotAnswerFallsBackToThePlainCut: a rethink the
// model gives no answer to costs nothing more than the call; the stalled
// rounds are cut the way they were before, the round that made progress
// stays byte for byte, the rewind line follows, and the stall is written into
// the record in the harness's own words naming the call.
func TestARethinkTheModelCannotAnswerFallsBackToThePlainCut(t *testing.T) {
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the notes", "the notes", "the brand file")
	built := newHarness(t, []testkit.Step{
		sameReadOf("c1", "notes.md"), sameReadOf("c2", "notes.md"), sameReadOf("c3", "notes.md"), sameReadOf("c4", "notes.md"),
		answerStep(""),
		callStep("I will read the brand file instead.", callFor("c5", "read", `{"path":"brand.md"}`)),
		answerStep("The notes and the brand file are read."),
	}, reading)

	outcome := built.ask(t, "read the notes")

	if outcome.Status == contract.StatusStopped {
		t.Fatalf("the task stopped on the first stall, and a rethink with no answer falls back to the cut")
	}
	if rethinks := theRethinkRequests(built); len(rethinks) != 1 {
		t.Fatalf("the model was asked the rethink's question %d times, want once before the fallback", len(rethinks))
	}
	first, _ := requestsCarrying(built, loop.TheRewindLine)
	if first != 5 {
		t.Fatalf("the rewind line first rode on model call %d, want call 5: the one right after the rethink that got no answer", first)
	}
	sawTheLine, keptTheFirstRound, keptAStalledRound := whatSurvivedTheCut(built.model.Requests()[first])
	if !sawTheLine || !keptTheFirstRound || keptAStalledRound {
		t.Errorf("the cut after the fallback kept the first round %v, kept a stalled round %v and carried the rewind line %v; want true, false, true",
			keptTheFirstRound, keptAStalledRound, sawTheLine)
	}
	if _, count := requestsCarrying(built, loop.TheRethinkLine); count != 0 {
		t.Errorf("the rethink line was handed to the model %d times, and there was no rethink to hand it", count)
	}
	held := built.held(t, outcome.TaskID)
	if !aFailureBeginning(held, "stalled: asked for read notes.md") {
		t.Errorf("the record's failures read %+v, want the stall in the harness's own words naming the call", held.Lessons.Failures)
	}
	if len(reading.Inputs()) != 3 {
		t.Errorf("the tool ran %d times, want 3: two of the stalled read and the different read after the cut", len(reading.Inputs()))
	}
}

// TestTheStopAfterTheLastRethinkListsWhatWasTried: the bounds stay what they
// were, RewindsAllowed rethinks before a run of the same call ends the task,
// and the stop's report then carries one line listing every rethink's
// decision, so that the person reads the ledger rather than only the last
// stall.
func TestTheStopAfterTheLastRethinkListsWhatWasTried(t *testing.T) {
	nexts := []string{"read brand.md", "run ls /notes", "run grep -n room notes.md"}
	steps := []testkit.Step{}
	answers := []string{}
	for stall := 0; stall <= loop.RewindsAllowed; stall++ {
		path := fmt.Sprintf("notes%d.md", stall)
		for call := 1; call <= 4; call++ {
			steps = append(steps, sameReadOf(fmt.Sprintf("c%d", stall*4+call), path))
		}
		answers = append(answers, "the notes", "the notes")
		if stall < loop.RewindsAllowed {
			steps = append(steps, aRethinkAnswer(path+" read four times over", "the file never changes",
				"the wrong file, or the wrong folder", nexts[stall]))
		}
	}
	steps = append(steps, aReviewReply("Read a file once and move on."))
	built := newHarness(t, steps, scriptedTool("read", answers...))

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusStopped {
		t.Fatalf("the task ended %q, want stopped after the last rethink", outcome.Status)
	}
	if rethinks := theRethinkRequests(built); len(rethinks) != loop.RewindsAllowed {
		t.Errorf("the model was asked the rethink's question %d times, want %d: the bound is what it was", len(rethinks), loop.RewindsAllowed)
	}
	if !strings.Contains(outcome.Report, "\nTried by rethink: ") {
		t.Fatalf("the stop's report does not list what the rethinks tried:\n%s", outcome.Report)
	}
	for at, next := range nexts {
		if !strings.Contains(outcome.Report, fmt.Sprintf("D%d Rethink: next %s", at+1, next)) {
			t.Errorf("the stop's report does not list the decision of rethink %d, %q:\n%s", at+1, next, outcome.Report)
		}
	}
	if !sentSomethingLike(built.channel.Sent(), "Tried by rethink: ") {
		t.Errorf("the person was sent %v, want the stop's report with the ledger on it", built.channel.Sent())
	}
}

// TestARethinkAnswerWithNoLabelsIsTakenWholeAsTheNextLine: a small model may
// answer the five questions in prose. An answer with none of the labels is
// the Next line whole, and it fills the failure and the decision too, so that
// the record always gains both and the window always ends on something to do.
func TestARethinkAnswerWithNoLabelsIsTakenWholeAsTheNextLine(t *testing.T) {
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the notes", "the notes", "the brand file")
	built := newHarness(t, []testkit.Step{
		sameReadOf("c1", "notes.md"), sameReadOf("c2", "notes.md"), sameReadOf("c3", "notes.md"), sameReadOf("c4", "notes.md"),
		answerStep("Just read brand.md and compare the two."),
		callStep("I will read the brand file.", callFor("c5", "read", `{"path":"brand.md"}`)),
		answerStep("The notes and the brand file are read."),
	}, reading)

	outcome := built.ask(t, "read the notes")

	held := built.held(t, outcome.TaskID)
	if !aFailureBeginning(held, "stalled: Just read brand.md and compare the two.") {
		t.Errorf("the record's failures read %+v, want the whole answer as the stall", held.Lessons.Failures)
	}
	if _, written := aDecisionBeginning(held, "Rethink: next Just read brand.md and compare the two."); !written {
		t.Errorf("the record's decisions read %+v, want the whole answer as the Next line", held.Lessons.Decisions)
	}
	if _, count := requestsCarrying(built, loop.TheRethinkLine); count == 0 {
		t.Error("the fresh window never opened on the answer")
	}
}

// TestTheRethinkBackgroundIsCappedAtOneRead: the background holds the newest
// result in full so that the model has once and whole the thing it kept
// asking for, and no more than one read's worth of it, because a result may
// be megabytes.
func TestTheRethinkBackgroundIsCappedAtOneRead(t *testing.T) {
	huge := strings.Repeat("the notes say the meeting is at noon. ", 100*1024/38+1)
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, huge, huge, "the brand file")
	built := newHarness(t, []testkit.Step{
		sameReadOf("c1", "notes.md"), sameReadOf("c2", "notes.md"), sameReadOf("c3", "notes.md"), sameReadOf("c4", "notes.md"),
		theUsualRethink(),
		callStep("I will read the brand file instead.", callFor("c5", "read", `{"path":"brand.md"}`)),
		answerStep("The notes and the brand file are read."),
	}, reading)

	built.ask(t, "read the notes")

	rethinks := theRethinkRequests(built)
	if len(rethinks) != 1 {
		t.Fatalf("the model was asked the rethink's question %d times, want once", len(rethinks))
	}
	background := rethinks[0].Messages[0].Text
	if !strings.Contains(background, huge[:loop.MaxRethinkResultBytes]) {
		t.Errorf("the background does not hold the first %d bytes of the newest result", loop.MaxRethinkResultBytes)
	}
	if strings.Contains(background, huge[:loop.MaxRethinkResultBytes+1]) {
		t.Errorf("the background holds more than %d bytes of the newest result", loop.MaxRethinkResultBytes)
	}
	if len(background) > 2*loop.MaxRethinkResultBytes {
		t.Errorf("the background is %d bytes long against a result cap of %d, and the rest of it is the record and a few lines",
			len(background), loop.MaxRethinkResultBytes)
	}
	if !strings.Contains(background, "cut") {
		t.Error("the background does not say the result was cut")
	}
}

// TestTheRethinkQuestionAndAnswerAreLogged: the rethink is the third question
// the harness asks with the tools off, and the log holds it the way it holds
// the review and the section question: the question, the model's whole
// answer, and what came of it. Run 29's logic task stalled twice and the
// record kept only one line of each answer, so nobody could read what the
// model had thought. A rethink with no answer is logged too, with the cut it
// fell back to.
func TestTheRethinkQuestionAndAnswerAreLogged(t *testing.T) {
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the notes", "the notes", "the brand file")
	built := newHarness(t, []testkit.Step{
		sameReadOf("c1", "notes.md"), sameReadOf("c2", "notes.md"), sameReadOf("c3", "notes.md"), sameReadOf("c4", "notes.md"),
		theUsualRethink(),
		callStep("I will read the brand file.", callFor("c5", "read", `{"path":"brand.md"}`)),
		answerStep("The notes and the brand file are read."),
	}, reading)

	outcome := built.ask(t, "read the notes")

	var rethinks []map[string]string
	for _, asked := range questionEventsOf(t, built) {
		if asked["purpose"] == "rethink" {
			rethinks = append(rethinks, asked)
		}
	}
	if len(rethinks) != 1 {
		t.Fatalf("the log holds %d rethink question events, want one", len(rethinks))
	}
	logged := rethinks[0]
	if logged["question"] != loop.TheRethinkQuestion || logged["task"] != outcome.TaskID {
		t.Errorf("the rethink's event reads %v, want the rethink question under task %s", logged, outcome.TaskID)
	}
	if !strings.Contains(logged["answer"], "Next: read brand.md") {
		t.Errorf("the rethink's event does not carry the model's whole answer: %q", logged["answer"])
	}
	if !strings.Contains(logged["outcome"], "fresh window") {
		t.Errorf("the rethink's event does not say what came of the answer: %q", logged["outcome"])
	}
}

// TestARethinkWithNoAnswerIsLoggedWithTheCut: when the model gives no answer
// the plain cut stands, and the log says so.
func TestARethinkWithNoAnswerIsLoggedWithTheCut(t *testing.T) {
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the notes", "the notes", "the brand file")
	built := newHarness(t, []testkit.Step{
		sameReadOf("c1", "notes.md"), sameReadOf("c2", "notes.md"), sameReadOf("c3", "notes.md"), sameReadOf("c4", "notes.md"),
		answerStep(""),
		callStep("I will read the brand file instead.", callFor("c5", "read", `{"path":"brand.md"}`)),
		answerStep("The notes and the brand file are read."),
	}, reading)

	built.ask(t, "read the notes")

	for _, asked := range questionEventsOf(t, built) {
		if asked["purpose"] == "rethink" {
			if asked["answer"] != "" || !strings.Contains(asked["outcome"], "cut") {
				t.Errorf("the unanswered rethink's event reads %v, want an empty answer and the cut as its outcome", asked)
			}
			return
		}
	}
	t.Fatalf("the log holds no rethink question event for the rethink that got no answer")
}

// twoFailuresAboutTheRoom is the model writing two failures on its first
// round, whose causes are the two the rethinks below name again.
func twoFailuresAboutTheRoom() testkit.Step {
	return callStep("I will write what I already know went wrong.",
		taskCall("f1", `{"operation":"failure","text":"the notes never name the room","cause":"the room is written in the brand file"}`),
		taskCall("f2", `{"operation":"failure","text":"the room was never asked for","cause":"nobody wrote the room down anywhere"}`))
}

// theNameNewCausesLine is the second half of the line put in front of the
// rethink's question when both its causes are already on the record, in the
// exact words the brief fixed.
const theNameNewCausesLine = "name two causes the record does not hold"

// theRethinkSendBacks are the model calls made with the tools off whose last
// message is the rethink's question asked once more, with the line naming
// the failures that already hold its causes in front of it.
func theRethinkSendBacks(built *harness) []contract.Request {
	found := []contract.Request{}
	for _, request := range built.model.Requests() {
		if len(request.Messages) == 0 {
			continue
		}
		last := request.Messages[len(request.Messages)-1]
		if request.ToolsOff && strings.Contains(last.Text, theNameNewCausesLine) {
			found = append(found, request)
		}
	}
	return found
}

// TestARethinkWhoseCausesAreAllOnTheRecordIsAskedOnceMore: the sky task's
// two rethinks both named the cause the record already held, because the
// question asks for causes not on the record and nothing checked the answer.
// Now each of the two causes is held against every failure's cause on the
// record; when both are already there, the question is asked once more with
// one line in front of it naming the failures that say so, and the second
// answer is taken as it stands, whatever it says. One send-back per rethink.
func TestARethinkWhoseCausesAreAllOnTheRecordIsAskedOnceMore(t *testing.T) {
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the notes say the meeting is at noon", "the notes say the meeting is at noon", "the brand file names the room")
	heldCauses := "the room is in the brand file, or nobody wrote the room down"
	built := newHarness(t, []testkit.Step{
		twoFailuresAboutTheRoom(),
		sameReadOf("c1", "notes.md"), sameReadOf("c2", "notes.md"), sameReadOf("c3", "notes.md"), sameReadOf("c4", "notes.md"),
		aRethinkAnswer("the notes say the meeting is at noon, four times over", "the notes do not hold the room", heldCauses, "read brand.md"),
		aRethinkAnswer("the notes say the meeting is at noon, four times over", "the notes do not hold the room", heldCauses, "grep -rn room ."),
		callStep("I will search as the rethink says.", callFor("c5", "read", `{"path":"brand.md"}`)),
		answerStep("The room is found. What changed: nothing. What I checked: the brand file. What is left: nothing."),
	}, reading)

	outcome := built.ask(t, "read the notes and find the room")

	if rethinks := theRethinkRequests(built); len(rethinks) != 1 {
		t.Fatalf("the model was asked the rethink's question plain %d times, want once", len(rethinks))
	}
	sendBacks := theRethinkSendBacks(built)
	if len(sendBacks) != 1 {
		t.Fatalf("the model was asked the rethink's question once more %d times, want once: one send-back per rethink, whatever the second answer says", len(sendBacks))
	}
	last := sendBacks[0].Messages[len(sendBacks[0].Messages)-1].Text
	wanted := "F1 and F2 already say this; " + theNameNewCausesLine + "\n" + loop.TheRethinkQuestion
	if last != wanted {
		t.Errorf("the send-back's question reads:\n%s\nwant the line naming F1 and F2 in front of the question:\n%s", last, wanted)
	}
	held := built.held(t, outcome.TaskID)
	if _, written := aDecisionBeginning(held, "Rethink: next grep -rn room ."); !written {
		t.Errorf("the record's decisions read %+v, want the second answer's Next line as the decision", held.Lessons.Decisions)
	}
	if _, written := aDecisionBeginning(held, "Rethink: next read brand.md"); written {
		t.Errorf("the record's decisions read %+v, and the first answer's Next line was sent back, not taken", held.Lessons.Decisions)
	}
	first, _ := requestsCarrying(built, loop.TheRethinkLine)
	if first < 0 || !strings.Contains(theLastMessageOf(built.model.Requests()[first]), "Next: grep -rn room .") {
		t.Errorf("the fresh window does not open on the second answer; the requests read:\n%s", requestsJoined(built.model.Requests()))
	}
}

// TestARethinkWithOneNewCauseIsTakenAsItStands: a rethink that names one
// cause the record holds and one it does not is taken as it stands, with no
// second ask, because one new cause is what the Next line tells apart.
func TestARethinkWithOneNewCauseIsTakenAsItStands(t *testing.T) {
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the notes say the meeting is at noon", "the notes say the meeting is at noon", "the brand file names the room")
	built := newHarness(t, []testkit.Step{
		twoFailuresAboutTheRoom(),
		sameReadOf("c1", "notes.md"), sameReadOf("c2", "notes.md"), sameReadOf("c3", "notes.md"), sameReadOf("c4", "notes.md"),
		aRethinkAnswer("the notes say the meeting is at noon, four times over", "the notes do not hold the room",
			"the room is in the brand file, or the room is only on the printed invitation", "read brand.md"),
		callStep("I will read the brand file, as the rethink says.", callFor("c5", "read", `{"path":"brand.md"}`)),
		answerStep("The room is found. What changed: nothing. What I checked: the brand file. What is left: nothing."),
	}, reading)

	outcome := built.ask(t, "read the notes and find the room")

	if rethinks := theRethinkRequests(built); len(rethinks) != 1 {
		t.Fatalf("the model was asked the rethink's question %d times, want once", len(rethinks))
	}
	if sendBacks := theRethinkSendBacks(built); len(sendBacks) != 0 {
		t.Errorf("the model was asked the rethink's question once more %d times, and one of its causes is new", len(sendBacks))
	}
	held := built.held(t, outcome.TaskID)
	decision, written := aDecisionBeginning(held, "Rethink: next read brand.md")
	if !written || !strings.Contains(decision.Reason, "the printed invitation") {
		t.Errorf("the record's decisions read %+v, want the answer's Next line as the decision with both causes as its reason", held.Lessons.Decisions)
	}
}

// aTaskThatWritesAFailureAndRunsOut is one round of work that writes a
// failure into the record, and then the report the spent budget buys, so
// that the task ends stopped with a failure on its record.
func aTaskThatWritesAFailureAndRunsOut() []testkit.Step {
	return []testkit.Step{
		callStep("I will read the notes and write what went wrong.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("f1", `{"operation":"failure","text":"the notes never name the room","cause":"the room is written in the brand file"}`)),
		answerStep("The budget is spent. What changed: nothing. What I checked: the notes. What is left: the room."),
	}
}

// TestAPickedUpTaskWithAFailureOpensWithARethink: the sky task was picked up
// three times, and each time it opened on the record and went straight back
// to the failure list that had stopped it. A task picked up after a stop
// whose record holds a failure now opens with a rethink, before its first
// working call: the record and the newest result in front of the model, the
// five questions, the answer into the record, and the fresh window on the
// answer, with the person's word last.
func TestAPickedUpTaskWithAFailureOpensWithARethink(t *testing.T) {
	steps := append(aTaskThatWritesAFailureAndRunsOut(),
		aRethinkAnswer("the notes were read once and never named the room", "the notes do not hold the room",
			"the room is on the printed invitation, or nobody wrote it down", "read brand.md"),
		callStep("I will read the brand file, as the rethink says.", callFor("c2", "read", `{"path":"brand.md"}`)),
		answerStep("The room is in the brand file. What changed: nothing. What I checked: both files. What is left: nothing."))
	built := newHarness(t, steps, scriptedTool("read", "the notes say the meeting is at noon", "the brand file names the room"))
	stopped := runOutOfBudget(t, built)
	before := len(built.model.Requests())

	outcome := continueTheTask(t, built, stopped.TaskID)

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the picked-up task ended %q, want done: %s", outcome.Status, outcome.Report)
	}
	requests := built.model.Requests()
	if len(requests) < before+2 {
		t.Fatalf("the pick-up made %d model calls, want at least two: the rethink and the working call after it", len(requests)-before)
	}
	asked := requests[before]
	if !asked.ToolsOff || asked.Messages[len(asked.Messages)-1].Text != loop.TheRethinkQuestion {
		t.Fatalf("the pick-up's first call is not the rethink with the tools off:\n%s", wholeRequestText(asked))
	}
	background := asked.Messages[0].Text
	for _, words := range []string{"F1", "the notes never name the room", "the notes say the meeting is at noon"} {
		if !strings.Contains(background, words) {
			t.Errorf("the rethink's background does not carry %q:\n%s", words, background)
		}
	}
	window := requests[before+1]
	opening, rethought, there := theMessagesAround(window, loop.TheRethinkLine)
	if !there || !strings.Contains(rethought.Text, "Next: read brand.md") || !strings.Contains(rethought.Text, loop.TheDoTheNextLine) {
		t.Fatalf("the working call after the rethink does not open on the answer with the instruction to do its Next line:\n%s", wholeRequestText(window))
	}
	if !strings.HasPrefix(opening.Text, orientation.TheHeading) || !strings.Contains(opening.Text, "the notes say the meeting is at noon") {
		t.Errorf("the fresh window does not open with the orientation and the newest results in full:\n%s", opening.Text)
	}
	if last := window.Messages[len(window.Messages)-1].Text; last != "carry on" {
		t.Errorf("the fresh window's last message reads %q, want the person's word last, after the rethink", last)
	}
	held := built.held(t, outcome.TaskID)
	if !aFailureBeginning(held, "stalled: the notes were read once") {
		t.Errorf("the record's failures read %+v, want the rethink's Showed line written as a failure", held.Lessons.Failures)
	}
	if _, written := aDecisionBeginning(held, "Rethink: next read brand.md"); !written {
		t.Errorf("the record's decisions read %+v, want the Next line as a decision", held.Lessons.Decisions)
	}
}

// TestAPickedUpTaskWithNoFailureOpensAsBefore: a picked-up task with no
// failure on its record has nothing to rethink, and opens on the orientation
// and the person's word as it did.
func TestAPickedUpTaskWithNoFailureOpensAsBefore(t *testing.T) {
	built := newHarness(t, aTaskThatRunsOutAndThenCarriesOn(), scriptedTool("read", "the notes", "the brand file"))
	stopped := runOutOfBudget(t, built)
	before := len(built.model.Requests())

	outcome := continueTheTask(t, built, stopped.TaskID)

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the picked-up task ended %q, want done: %s", outcome.Status, outcome.Report)
	}
	if rethinks := theRethinkRequests(built); len(rethinks) != 0 {
		t.Errorf("the model was asked the rethink's question %d times, and the record holds no failure", len(rethinks))
	}
	first := built.model.Requests()[before]
	if first.ToolsOff || !strings.Contains(wholeRequestText(first), orientation.TheHeading) {
		t.Errorf("the pick-up's first call does not open on the orientation with the tools on:\n%s", wholeRequestText(first))
	}
	if _, count := requestsCarrying(built, loop.TheRethinkLine); count != 0 {
		t.Errorf("the rethink line rode on %d calls, and there was no rethink", count)
	}
}

// TestATaskAnsweredOnItsQuestionOpensWithoutARethink: a task waiting on its
// own question is not a task picked up after a stop. The person's answer is
// what it needs, so it opens as before, failure on the record or not.
func TestATaskAnsweredOnItsQuestionOpensWithoutARethink(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes and write what went wrong.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("f1", `{"operation":"failure","text":"the notes never name the room","cause":"the room is written in the brand file"}`)),
		answerStep("Which room did you mean?"),
		answerStep("The blue room it is. What changed: nothing. What I checked: the notes. What is left: nothing."),
	}, scriptedTool("read", "the notes say the meeting is at noon"))
	waiting := built.ask(t, "read the notes and find the room")
	if waiting.Status != contract.StatusWaiting {
		t.Fatalf("the first sitting ended %q, want waiting on the question", waiting.Status)
	}

	answered := built.task("the blue room")
	answered.ResumeID = waiting.TaskID
	outcome, err := built.loop.Run(t.Context(), answered)
	if err != nil {
		t.Fatalf("the loop could not pick the task up again: %v", err)
	}

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the answered task ended %q, want done: %s", outcome.Status, outcome.Report)
	}
	if rethinks := theRethinkRequests(built); len(rethinks) != 0 {
		t.Errorf("the model was asked the rethink's question %d times on an answer to its own question", len(rethinks))
	}
}
