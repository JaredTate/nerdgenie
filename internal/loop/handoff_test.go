package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theJobToolsAnswerForAJobWithTasks is what the job tool answers when a create
// names a first task and more to follow, which is the answer the loop reads
// the hand-off off.
const theJobToolsAnswerForAJobWithTasks = "created job 1 and started task t1, with t2 to follow\n"

// TestATaskThatMadeAJobWithTasksEndsAtOnceSoTheJobsFirstTaskCanStart is the
// fix for what every game build so far did. The model read the ask, made a job
// of ten tasks, and then kept working inside the task that made it: the job's
// tasks could not start until that task ended, so the side panel showed nought
// of ten done for an hour while the work went on unmarked, and the task's own
// record, not the job's, held everything. A job with a first task carries the
// work from the moment it is made, so the task that made it ends there, with
// one done line naming the job, and the driver takes the job's first task.
func TestATaskThatMadeAJobWithTasksEndsAtOnceSoTheJobsFirstTaskCanStart(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("This is a job, so I will make it.",
			callFor("c1", contract.ToolJob, `{"operation":"create","text":"post the anniversary tweet","tasks":["write the summary"]}`)),
		callStep("Now I will start on the first task myself.", callFor("c2", "read", `{"path":"notes.md"}`)),
		answerStep("Done."),
	}, scriptedTool(contract.ToolJob, theJobToolsAnswerForAJobWithTasks), scriptedTool("read", "the notes"))
	jobID := aJobOfTwoTasks(t, built)

	outcome := built.ask(t, "run the anniversary campaign")

	if outcome.Status != contract.StatusDone {
		t.Errorf("the task that made the job ended %q, want done, because the work is the job's now", outcome.Status)
	}
	if left := built.model.StepsLeft(); left != 2 {
		t.Errorf("the model has %d steps left, want 2: it was called again after making the job, and the job's tasks are where the work goes on", left)
	}
	held := built.held(t, outcome.TaskID)
	if len(held.Goal.DoneWhen) != 1 || !held.Goal.DoneWhen[0].Done || !strings.Contains(held.Goal.DoneWhen[0].Text, "job "+jobID) {
		t.Errorf("the done list reads %+v, want one line naming job %s, proved by the job tool's result", held.Goal.DoneWhen, jobID)
	}
	for _, words := range []string{"job " + jobID, "t1", "side"} {
		if !sentSomethingLike(built.channel.Sent(), words) {
			t.Errorf("the person was sent %v, want the report to say %q: which job, that its first task starts now, and where to watch it", built.channel.Sent(), words)
		}
	}
	if next := theSummaryOf(t, built, jobID).NextTaskID; next != "t1" {
		t.Errorf("the job's next task is %q, want t1, the first, due the moment the task that made the job ended", next)
	}
}

// TestAScheduledJobMadeMidTaskLeavesTheTaskToGoOn keeps the other case: a job
// made with a schedule and no first task carries no work of the task's, so the
// task goes on and answers in its own words.
func TestAScheduledJobMadeMidTaskLeavesTheTaskToGoOn(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will schedule the nightly check.",
			callFor("c1", contract.ToolJob, `{"operation":"create","schedule":"0 4 * * *","template":"run the nightly check"}`)),
		answerStep("The nightly check is scheduled for four every morning."),
	}, scriptedTool(contract.ToolJob, "created job 1\n"))

	outcome := built.ask(t, "check yourself every night")

	if outcome.Status != contract.StatusDone || built.model.StepsLeft() != 0 {
		t.Errorf("the task ended %q with %d steps left, want it done in the model's own words, because a scheduled job carries none of the task's work",
			outcome.Status, built.model.StepsLeft())
	}
	if !sentSomethingLike(built.channel.Sent(), "four every morning") {
		t.Errorf("the person was sent %v, want the model's own answer", built.channel.Sent())
	}
}
