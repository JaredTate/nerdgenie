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

// theLastMessageOf is the text of the last message of a request.
func theLastMessageOf(request contract.Request) string {
	if len(request.Messages) == 0 {
		return ""
	}
	return request.Messages[len(request.Messages)-1].Text
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
	last := theLastMessageOf(after)
	if !strings.HasSuffix(strings.TrimSpace(last), "Do the Next line now.") {
		t.Errorf("the fresh window's last message does not end with the instruction to do the Next line:\n%s", last)
	}
	if !strings.Contains(last, loop.TheRethinkLine) || !strings.Contains(last, "Next: read brand.md") {
		t.Errorf("the fresh window's last message does not hold the rethink line and the model's whole answer:\n%s", last)
	}
	if _, keptTheFirstRound, keptAStalledRound := whatSurvivedTheCut(after); keptTheFirstRound || keptAStalledRound {
		t.Errorf("a round from before the rethink survived into the fresh window: %+v", after.Messages)
	}
	if opening := after.Messages[0].Text; !strings.HasPrefix(opening, orientation.TheHeading) || !strings.Contains(opening, "the notes say the meeting is at noon") {
		t.Errorf("the fresh window does not open with the orientation and the newest results in full:\n%s", opening)
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
	requests := built.model.Requests()
	refused, ran := 0, 0
	for _, message := range requests[len(requests)-1].Messages {
		for _, result := range message.ToolResults {
			switch {
			case strings.HasPrefix(result.Text, closedLine):
				refused++
			case result.CallID == "open" && result.Text == "the notes":
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
