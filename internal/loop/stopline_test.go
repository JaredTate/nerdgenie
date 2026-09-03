package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/testkit"
)

// TestAStopLineAboutTheTaskDoesNotFireOnAResultThatRepeatsTwoOfItsWords is the
// gate review's fourth finding. The model is told to write a stop list about
// the work it is doing, so its stop lines name the subject of the task, and the
// first tool result that mentions that subject used to end the task on round
// one. A stop condition is a statement about the world, and two words in a row
// are not one.
func TestAStopLineAboutTheTaskDoesNotFireOnAResultThatRepeatsTwoOfItsWords(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("Nothing is read yet. I will read the notes.",
			taskCall("c1t", `{"why":"the user wants a post","doneWhen":["the post is written"],`+
				`"stopWhen":["the product notes file cannot be found"]}`),
			callFor("c1", "read", `{"path":"product-notes.md"}`)),
		callStep("The notes are read. I will write the post down.",
			taskCall("c2t", `{"doneWhen":[{"text":"the post is written","done":true,"resultId":"r2"}]}`)),
		answerStep("The post is written and the done line points at the result that proves it."),
	}, scriptedTool("read", "product notes: DigiByte launched in 2014 and the notes run to four pages."))

	outcome := built.ask(t, "write a post from the product notes")

	if outcome.Status == contract.StatusStopped {
		t.Errorf("the task ended stopped on the line %q, and a result that repeats two words of a stop line is not that line coming true",
			outcome.StopLine)
	}
	if outcome.Status != contract.StatusDone {
		t.Errorf("the task ended %q, want done, because nothing on the stop list actually came true", outcome.Status)
	}
}

// TestAStopLineAboutALimitDoesNotFireOnAResultAboutTheSameLimit is the second
// probe of the same finding: "the post is longer than the limit after two
// tries" shares the words "longer than" with a result that says the draft is
// longer than the limit allows, and sharing words is not the same as the line
// having come true.
func TestAStopLineAboutALimitDoesNotFireOnAResultAboutTheSameLimit(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("Nothing is measured yet. I will measure the draft.",
			taskCall("c1t", `{"why":"the user wants a post","doneWhen":["the post fits"],`+
				`"stopWhen":["the post is longer than the limit after two tries"]}`),
			callFor("c1", "read", `{"path":"draft.md"}`)),
		callStep("The draft is too long, so I will shorten it and write that down.",
			taskCall("c2t", `{"doneWhen":[{"text":"the post fits","done":true,"resultId":"r2"}]}`)),
		answerStep("The post fits inside the limit now."),
	}, scriptedTool("read", "the draft is longer than the limit allows, by eleven characters"))

	outcome := built.ask(t, "shorten the post until it fits")

	if outcome.Status == contract.StatusStopped {
		t.Errorf("the task ended stopped on the line %q after one try, and the line says two tries", outcome.StopLine)
	}
}

// TestTheModelSaysWhenItsOwnStopLineHasComeTrue proves the other half of the
// same finding: the harness cannot read a stop line about the world, so the
// model says when one of its own lines has come true, in the same reply as its
// other calls and at no extra model call.
func TestTheModelSaysWhenItsOwnStopLineHasComeTrue(t *testing.T) {
	stopLine := "the product notes file cannot be found"
	built := newHarness(t, []testkit.Step{
		callStep("Nothing is read yet. I will read the notes.",
			taskCall("c1t", `{"why":"the user wants a post","doneWhen":["the post is written"],`+
				`"stopWhen":["`+stopLine+`"]}`),
			callFor("c1", "read", `{"path":"product-notes.md"}`)),
		callStep("The notes are not there, so the stop line I wrote has come true.",
			taskCall("c2t", `{"operation":"stop_now","text":"`+stopLine+`"}`)),
		aReviewReply("Keep the stop list about the world."),
		answerStep("This reply is never played, because the task stopped before it."),
	}, scriptedTool("read", "there is no file called product-notes.md"))

	outcome := built.ask(t, "write a post from the product notes")

	if outcome.Status != contract.StatusStopped {
		t.Errorf("the task ended %q, want stopped, because the model said its own stop line had come true", outcome.Status)
	}
	if outcome.StopLine != stopLine {
		t.Errorf("the line that fired reads %q, want the line the model named", outcome.StopLine)
	}
	if !sentSomethingLike(built.channel.Sent(), stopLine) {
		t.Errorf("the user was sent %v, and a stopped task says which line fired", built.channel.Sent())
	}
	if built.model.StepsLeft() != 1 {
		t.Error("the model was called again after the stop, and a stop the model asked for costs no extra call")
	}
}

// TestAStopNowWithNoLineIsRefusedAndTheTaskGoesOn proves the harness reads a
// stop the model asked for by more than the operation's name: a stop with no
// line to name is refused with what to write instead, and the task carries on.
func TestAStopNowWithNoLineIsRefusedAndTheTaskGoesOn(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will try to stop with no line.", taskCall("c1t", `{"operation":"stop_now"}`)),
		answerStep("I did not name a line, so what should I do?"),
	})

	outcome := built.ask(t, "write a post")

	if outcome.Status == contract.StatusStopped {
		t.Error("the task stopped on a stop the model never named a line for")
	}
	if !strings.Contains(requestsJoined(built.model.Requests()), "which line of the stop list") {
		t.Error("the model was never told that a stop names the line of the stop list that came true")
	}
	_ = loop.HarnessStopLineWall
}
