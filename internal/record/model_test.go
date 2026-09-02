package record

import (
	"errors"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// TestTheModelWritesItsHalfOfTheRecord walks one good update through the door the
// task tool uses: the why, the done list, the stop list, and the plan.
func TestTheModelWritesItsHalfOfTheRecord(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()

	err := keeper.Apply(ctx, Update{
		Why:      "mark the anniversary publicly today",
		DoneWhen: []contract.DoneLine{{Text: "one post is up on the DigiByte account"}},
		StopWhen: []string{"the account shows a login page or a captcha"},
		Plan:     []string{"read the product notes", "draft the post", "post it"},
	})
	if err != nil {
		t.Fatalf("the model could not write its half of the record: %v", err)
	}

	held := keeper.Record()
	if held.Goal.Why != "mark the anniversary publicly today" {
		t.Errorf("the why reads %q", held.Goal.Why)
	}
	if len(held.Goal.DoneWhen) != 1 || len(held.Rules.StopWhen) != 1 || len(held.Work.Plan) != 3 {
		t.Errorf("the record holds %+v after one update", held)
	}
	if held.Work.Plan[2].Number != 3 || held.Work.Plan[2].Text != "post it" {
		t.Errorf("the plan steps are numbered from one in the order they were written: %+v", held.Work.Plan)
	}
	if keeper.LatestCheckpoint() != 2 {
		t.Errorf("one update left the record at checkpoint %d, and one update is one change", keeper.LatestCheckpoint())
	}
}

// TestTheAskAndTheWhyAndTheCorrectionsAreFixed proves rule one. The ask has no
// field in an update at all, so this test proves it survives one; the why is set
// once; a correction cannot be reached from here.
func TestTheAskAndTheWhyAndTheCorrectionsAreFixed(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()

	if _, err := keeper.AddCorrection(ctx, "no, lead with the date"); err != nil {
		t.Fatalf("cannot add the correction: %v", err)
	}
	if err := keeper.Apply(ctx, Update{Why: "the first why"}); err != nil {
		t.Fatalf("cannot set the why: %v", err)
	}
	if err := keeper.Apply(ctx, Update{Why: "the first why"}); err != nil {
		t.Errorf("writing the same why again was refused, and it changes nothing: %v", err)
	}
	err := keeper.Apply(ctx, Update{Why: "a different why"})
	if !errors.Is(err, ErrWhyIsSet) {
		t.Errorf("the why was rewritten, and it is set once: %v", err)
	}

	held := keeper.Record()
	if held.Goal.Ask != taskStart().Ask {
		t.Errorf("the ask now reads %q, and nothing may edit it", held.Goal.Ask)
	}
	if held.Goal.Why != "the first why" {
		t.Errorf("the why now reads %q", held.Goal.Why)
	}
	if len(held.Rules.Corrections) != 1 || held.Rules.Corrections[0].Text != "no, lead with the date" {
		t.Errorf("the corrections read %+v, and they are the user's own words", held.Rules.Corrections)
	}
}

// TestRefusesADoneLineWithNothingBehindIt proves rule three for the done list.
func TestRefusesADoneLineWithNothingBehindIt(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()
	if _, err := keeper.AddResult(ctx, "the draft", "the whole draft"); err != nil {
		t.Fatalf("cannot add the result: %v", err)
	}

	err := keeper.Apply(ctx, Update{DoneWhen: []contract.DoneLine{{Text: "it is posted", Done: true}}})
	if !errors.Is(err, ErrDoneLineNeedsProof) {
		t.Errorf("a done line was marked done with nothing behind it: %v", err)
	}
	err = keeper.Apply(ctx, Update{DoneWhen: []contract.DoneLine{{Text: "it is posted", Done: true, ResultID: "r9"}}})
	if !errors.Is(err, ErrNoSuchResult) {
		t.Errorf("a done line named a result this record never wrote: %v", err)
	}
	if err := keeper.Apply(ctx, Update{DoneWhen: []contract.DoneLine{{Text: "it is posted", Done: true, ResultID: "r1"}}}); err != nil {
		t.Errorf("a done line naming a real result was refused: %v", err)
	}
	if err := keeper.Apply(ctx, Update{DoneWhen: []contract.DoneLine{{Text: "the user said so", Done: true, UserReply: "yes"}}}); err != nil {
		t.Errorf("a done line proved by the user's own reply was refused: %v", err)
	}
	if err := keeper.Apply(ctx, Update{DoneWhen: []contract.DoneLine{{Text: ""}}}); err == nil {
		t.Error("a done line with no text was accepted")
	}
}

// TestRefusesADecisionWithNoReasonAndAFailureWithNoCause proves rule two.
func TestRefusesADecisionWithNoReasonAndAFailureWithNoCause(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()

	if err := keeper.Apply(ctx, Update{Decision: &NewDecision{Text: "Lead with the date"}}); !errors.Is(err, ErrDecisionNeedsReason) {
		t.Errorf("a decision was written with no reason: %v", err)
	}
	if err := keeper.Apply(ctx, Update{Failure: &NewFailure{Text: "Draft 1 was 312 characters"}}); !errors.Is(err, ErrFailureNeedsCause) {
		t.Errorf("a failure was written with no cause: %v", err)
	}
	if err := keeper.Apply(ctx, Update{Decision: &NewDecision{Reason: "correction C1"}}); err == nil {
		t.Error("a decision was written with no choice in it")
	}
	if err := keeper.Apply(ctx, Update{Failure: &NewFailure{Cause: "three facts in one post"}}); err == nil {
		t.Error("a failure was written with nothing that went wrong in it")
	}

	good := Update{
		Decision: &NewDecision{Text: "Lead with the date", Reason: "correction C1"},
		Failure:  &NewFailure{Text: "Draft 1 was 312 characters", Cause: "three facts in one post"},
	}
	if err := keeper.Apply(ctx, good); err != nil {
		t.Fatalf("a decision and a failure that carry their reason and cause were refused: %v", err)
	}
	if err := keeper.Apply(ctx, good); err != nil {
		t.Fatalf("a second decision and failure were refused: %v", err)
	}
	held := keeper.Record().Lessons
	if len(held.Decisions) != 2 || held.Decisions[1].ID != "D2" || held.Failures[1].ID != "F2" {
		t.Errorf("the lessons read %+v, and their labels count upwards", held)
	}
}

// TestAnUpdateThatBreaksARuleChangesNothing proves an update is all or nothing,
// so that the model never has to guess how far its last one got.
func TestAnUpdateThatBreaksARuleChangesNothing(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()

	before := keeper.Text()
	err := keeper.Apply(ctx, Update{
		Why:      "a good why",
		StopWhen: []string{"a good stop line"},
		Decision: &NewDecision{Text: "a decision with no reason"},
	})
	if err == nil {
		t.Fatal("an update with a decision that carries no reason was accepted")
	}
	if keeper.Text() != before {
		t.Errorf("a refused update changed the record.\n--- before ---\n%s\n--- after ---\n%s", before, keeper.Text())
	}
	if keeper.LatestCheckpoint() != 1 {
		t.Errorf("a refused update saved a checkpoint, leaving the record at %d", keeper.LatestCheckpoint())
	}
}

// TestAnEmptyUpdateSavesNothing proves a round in which the model writes nothing
// into the record costs nothing, which is most rounds.
func TestAnEmptyUpdateSavesNothing(t *testing.T) {
	keeper, store := newKeeper(t, taskStart())
	if err := keeper.Apply(t.Context(), Update{}); err != nil {
		t.Fatalf("an empty update was refused: %v", err)
	}
	if keeper.LatestCheckpoint() != 1 || store.Count() != 1 {
		t.Errorf("an empty update saved checkpoint %d and left %d events in the log", keeper.LatestCheckpoint(), store.Count())
	}
}

// TestKeepsTheMarksOnPlanStepsTheModelDidNotChange proves that editing the plan
// does not throw away the proof of the steps that are already finished.
func TestKeepsTheMarksOnPlanStepsTheModelDidNotChange(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()

	if err := keeper.Apply(ctx, Update{Plan: []string{"read the notes", "draft the post"}}); err != nil {
		t.Fatalf("cannot write the plan: %v", err)
	}
	if _, err := keeper.AddResult(ctx, "read the notes", "the whole file"); err != nil {
		t.Fatalf("cannot add the result: %v", err)
	}
	if err := keeper.MarkPlanStep(ctx, 1, "r1"); err != nil {
		t.Fatalf("cannot mark the first step done: %v", err)
	}
	if err := keeper.Apply(ctx, Update{Plan: []string{"read the notes", "draft the post", "post it"}}); err != nil {
		t.Fatalf("cannot add a step to the plan: %v", err)
	}

	held := keeper.Record().Work.Plan
	if len(held) != 3 || !held[0].Done || held[0].ResultID != "r1" {
		t.Errorf("the finished step lost its mark when the plan grew: %+v", held)
	}
}

// TestRefusesTheOtherKindsWork proves a task has a plan and a job has a task
// list, and neither takes the other's.
func TestRefusesTheOtherKindsWork(t *testing.T) {
	task, _ := newKeeper(t, taskStart())
	job, _ := newKeeper(t, jobStart())
	ctx := t.Context()

	if err := task.Apply(ctx, Update{Tasks: []NewJobTask{{TaskID: "t1", Text: "do it"}}}); !errors.Is(err, ErrWrongKind) {
		t.Errorf("a task took a task list, and a task has a plan: %v", err)
	}
	if err := job.Apply(ctx, Update{Plan: []string{"do it"}}); !errors.Is(err, ErrWrongKind) {
		t.Errorf("a job took a plan, and a job has a task list: %v", err)
	}
}
