// The tests of the rethink holding its causes against the record. The sky
// task of the flight-simulator work order was asked twice for two causes not
// on the record and named the one it already held both times, because the
// question asked and nothing checked the answer.
package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

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
