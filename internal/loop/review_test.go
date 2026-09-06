package loop_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aReviewReply is what a model says when it is asked the four questions.
func aReviewReply(fourth string) testkit.Step {
	return answerStep("1. The notes were to be read.\n2. They were read.\n3. There was no difference.\n4. " + fourth)
}

// factsIn is everything the fake memory holds.
func factsIn(t *testing.T, built *harness) []contract.Fact {
	t.Helper()
	found, err := built.memory.Search(t.Context(), "", 0)
	if err != nil {
		t.Fatalf("cannot read the fake memory: %v", err)
	}
	return found
}

// TestTheReviewRunsWhenTheTaskHadACorrection proves the first of the four
// reasons a task is worth reviewing, and that only the fourth answer is kept.
func TestTheReviewRunsWhenTheTaskHadACorrection(t *testing.T) {
	steps := append(closingScript("the notes are read"), aReviewReply("Keep the brand file check on every draft."))
	built, _ := midTurnHarness(t, steps, "no, check the brand file first")

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done: %s", outcome.Status, outcome.Report)
	}
	facts := factsIn(t, built)
	if len(facts) != 1 || facts[0].Text != "Keep the brand file check on every draft." {
		t.Fatalf("memory holds %v, want only the fourth answer of the review", facts)
	}
	if facts[0].Source != "task "+outcome.TaskID {
		t.Errorf("the fact's source reads %q, want the task it was learned in", facts[0].Source)
	}
}

// TestTheReviewRunsWhenTheTaskWasStopped proves the second reason.
func TestTheReviewRunsWhenTheTaskWasStopped(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the page.", callFor("c1", "browser_read", `{}`)),
		aReviewReply("Keep an eye out for the login page."),
	}, scriptedTool("browser_read", "The page is a login page and it wants a username."))

	built.ask(t, "post the tweet")

	if len(factsIn(t, built)) != 1 {
		t.Error("a stopped task was not reviewed, and a stop is one of the four reasons to review")
	}
}

// TestTheReviewRunsWhenTheTaskHadAFailure proves the third reason: something
// went wrong along the way, even though the task finished.
func TestTheReviewRunsWhenTheTaskHadAFailure(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"doneWhen":["the notes are read"],"failure":{"text":"the first read came back empty","cause":"the path was wrong"}}`)),
		callStep("I will point the done line at the result.",
			taskCall("c2t", `{"doneWhen":[{"text":"the notes are read","done":true,"resultId":"r1"}]}`)),
		answerStep("Read. What changed: nothing. What I checked: the notes. What is left: nothing."),
		aReviewReply("Keep the path in the plan before reading."),
	}, scriptedTool("read", "the notes"))

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done: %s", outcome.Status, outcome.Report)
	}
	if len(factsIn(t, built)) != 1 {
		t.Error("a task with a failure in its record was not reviewed, and a failure is one of the four reasons")
	}
}

// TestATaskThatIsGivenUpOnIsStillReviewed proves a task the harness had to give
// up on is reviewed like any other failure.
func TestATaskThatIsGivenUpOnIsStillReviewed(t *testing.T) {
	steps := []testkit.Step{
		callStep("I will read the notes.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"doneWhen":["the notes are read"]}`)),
	}
	for range 4 {
		steps = append(steps, answerStep("It is done, honestly."))
	}
	steps = append(steps, aReviewReply("Keep the done list short enough to prove."))
	built := newHarness(t, steps, scriptedTool("read", "the notes"))

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusFailed {
		t.Fatalf("the task ended %q, want failed", outcome.Status)
	}
	if len(factsIn(t, built)) != 1 {
		t.Error("a task that was given up on was not reviewed")
	}
}

// TestTheReviewRunsAfterMoreThanFiveRounds proves the fourth reason: a long task
// is worth a look even when nothing went wrong.
func TestTheReviewRunsAfterMoreThanFiveRounds(t *testing.T) {
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "one", "two", "three", "four", "five", "six")
	steps := []testkit.Step{
		callStep("I will start.",
			callFor("c1", "read", `{"path":"one.md"}`),
			taskCall("c1t", `{"doneWhen":["every file is read"]}`)),
	}
	for at, path := range []string{"two", "three", "four", "five"} {
		steps = append(steps, callStep("Next one.", callFor("c"+path, "read", `{"path":"`+path+`.md"}`)))
		_ = at
	}
	steps = append(steps,
		callStep("I will point the line at the result.",
			taskCall("ct", `{"doneWhen":[{"text":"every file is read","done":true,"resultId":"r1"}]}`)),
		answerStep("All read. What changed: nothing. What I checked: five files. What is left: nothing."),
		aReviewReply("Keep reading the brand file first."),
	)
	built := newHarness(t, steps, reading)

	outcome := built.ask(t, "read the five files")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done: %s", outcome.Status, outcome.Report)
	}
	if len(factsIn(t, built)) != 1 {
		t.Error("a task of more than five rounds was not reviewed")
	}
}

// TestTheReviewDoesNotRunForAShortCleanTask proves the review runs only when it
// should, which is what keeps it from costing a call on every task.
func TestTheReviewDoesNotRunForAShortCleanTask(t *testing.T) {
	built := newHarness(t, closingScript("the notes are read"), scriptedTool("read", "the notes"))

	built.ask(t, "read the notes")

	if built.model.StepsLeft() != 0 {
		t.Errorf("%d scripted replies were left over, so a call was skipped", built.model.StepsLeft())
	}
	if facts := factsIn(t, built); len(facts) != 0 {
		t.Errorf("memory holds %v after a short clean task, and such a task is not reviewed", facts)
	}
}

// TestAReviewedProcedureIsOfferedAsASkill proves the last step of the review: a
// fourth answer that describes a way of doing something is offered to the user,
// and saved when they say yes.
func TestAReviewedProcedureIsOfferedAsASkill(t *testing.T) {
	steps := append(closingScript("the notes are read"),
		aReviewReply("To post on X, first open the composer and then attach the logo."))
	built, _ := midTurnHarness(t, steps, "no, check the brand file first")
	built.channel.AnswerPreviewsWith(contract.AnswerOnce)

	built.ask(t, "read the notes")

	previews := built.channel.Previews()
	if len(previews) != 1 || !strings.Contains(previews[0].Title, "Save this as a skill") {
		t.Fatalf("the user was shown %v, want one offer of the skill the review found", previews)
	}
	saved := built.skills.Files("to-post-on-x")
	if len(saved) == 0 || !strings.Contains(string(saved["SKILL.md"]), "open the composer") {
		t.Errorf("the skill folder holds %v, want the procedure the review wrote", saved)
	}
}

// TestASkillTheUserRefusesIsNotSaved proves the offer is an offer.
func TestASkillTheUserRefusesIsNotSaved(t *testing.T) {
	steps := append(closingScript("the notes are read"),
		aReviewReply("To post on X, first open the composer and then attach the logo."))
	built, _ := midTurnHarness(t, steps, "no, check the brand file first")
	built.channel.AnswerPreviewsWith(contract.AnswerReject)

	built.ask(t, "read the notes")

	if len(built.skills.Files("to-post-on-x")) != 0 {
		t.Error("a skill the user refused was saved anyway")
	}
	if len(factsIn(t, built)) != 1 {
		t.Error("the lesson itself was not kept as a fact, and every fourth answer is")
	}
}

// TestAReviewWhoseFactCannotBeSavedStillEndsTheTaskDone is the nightly
// game build of 6 September at 03:48: the review of the job's first task
// tried to save what it taught us under the id "review-task-6", memory
// already held a fact under that id from an earlier run whose persona files
// the run had copied, the save was refused, and the refusal ended the task
// with an error, so the job never got its report and never marked the task.
// A lesson that cannot be kept costs the lesson and nothing more: the task
// ends the way it was ending, and the review's id never collides, because it
// carries the time the task was reviewed.
func TestAReviewWhoseFactCannotBeSavedStillEndsTheTaskDone(t *testing.T) {
	steps := append(closingScript("the notes are read"), aReviewReply("Keep the brand file check on every draft."))
	built, _ := midTurnHarness(t, steps, "no, check the brand file first")
	built.memory.Refuse(errors.New("the memory file cannot be written"))

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done: a lesson that cannot be kept costs the lesson and nothing more. %s", outcome.Status, outcome.Report)
	}
}

// TestTheReviewsOfTwoTasksNeverShareAnId holds the second half: a run whose
// memory already holds a review of task 1, copied from another home, keeps
// the new review beside it under an id of its own.
func TestTheReviewsOfTwoTasksNeverShareAnId(t *testing.T) {
	steps := append(closingScript("the notes are read"), aReviewReply("Keep the brand file check on every draft."))
	built, _ := midTurnHarness(t, steps, "no, check the brand file first")
	if err := built.memory.Save(t.Context(), []contract.Fact{{ID: "review-task-1", Text: "an older run's lesson", Source: "task 1"}}); err != nil {
		t.Fatalf("cannot seed the older lesson: %v", err)
	}

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done: %s", outcome.Status, outcome.Report)
	}
	facts := factsIn(t, built)
	ids := map[string]string{}
	for _, fact := range facts {
		ids[fact.Text] = fact.ID
	}
	if len(facts) != 2 || ids["Keep the brand file check on every draft."] == "" || ids["Keep the brand file check on every draft."] == ids["an older run's lesson"] {
		t.Errorf("memory holds %v, want the older lesson and the new one under an id of its own", facts)
	}
}
