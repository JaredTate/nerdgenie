package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/testkit"
)

// TestAnUnattendedTaskOffersNoSkillToNobody is the gate review's tenth finding.
// The after-action review showed the skill offer with no thought for whether
// anybody was there to answer it, so every scheduled job that learned a
// procedure showed a preview to nobody and waited out the whole answer deadline.
func TestAnUnattendedTaskOffersNoSkillToNobody(t *testing.T) {
	built := newHarness(t, aTaskThatStopsAndLearnsAProcedure(), scriptedTool("read", "the account is locked"))
	task := built.task("post the tweet")
	task.Unattended = true

	outcome, err := built.loop.Run(t.Context(), task)
	if err != nil {
		t.Fatalf("the loop could not run the unattended task: %v", err)
	}

	if previews := built.channel.Previews(); len(previews) != 0 {
		t.Errorf("the run showed %d previews although nobody is there: %v", len(previews), previews)
	}
	if len(built.skills.Files("to-post-the-tweet")) != 0 {
		t.Error("a skill nobody approved was saved by an unattended run")
	}
	if facts := factsIn(t, built); len(facts) != 1 {
		t.Errorf("memory holds %v, and an unattended run keeps the lesson as a fact", facts)
	}
	if !strings.Contains(outcome.Report, "kept it as a fact") {
		t.Errorf("the report reads %q, and it does not say the lesson was kept as a fact rather than offered",
			outcome.Report)
	}
}

// TestAnAttendedTaskStillOffersTheSkill proves the offer is only held back when
// there is nobody to answer it.
func TestAnAttendedTaskStillOffersTheSkill(t *testing.T) {
	built := newHarness(t, aTaskThatStopsAndLearnsAProcedure(), scriptedTool("read", "the account is locked"))
	built.channel.AnswerPreviewsWith(contract.AnswerOnce)

	outcome := built.ask(t, "post the tweet")

	previews := built.channel.Previews()
	if len(previews) != 1 || !strings.Contains(previews[0].Title, "Save this as a skill") {
		t.Fatalf("the user was shown %v, want one offer of the skill the review found", previews)
	}
	if strings.Contains(outcome.Report, "kept it as a fact") {
		t.Errorf("the report reads %q, and an attended run says nothing about holding the offer back", outcome.Report)
	}
}

// aTaskThatStopsAndLearnsAProcedure is one task that stops on a line the model
// wrote and whose review answer is a way of doing something, which is what the
// skill offer is made of.
func aTaskThatStopsAndLearnsAProcedure() []testkit.Step {
	return []testkit.Step{
		callStep("Nothing is done yet. I will look at the account.",
			callFor("c1", "read", `{"path":"account.md"}`),
			taskCall("c1t", `{"why":"the user wants the tweet posted","doneWhen":["the tweet is up"],`+
				`"stopWhen":["the account is locked"]}`)),
		callStep("The account is locked, so the line I wrote has come true.",
			taskCall("c2t", `{"operation":"`+loop.OperationStopNow+`","text":"the account is locked"}`)),
		aReviewReply("To post the tweet, open the composer, paste the draft, and press post."),
	}
}
