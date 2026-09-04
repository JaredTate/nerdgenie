package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
)

// aLoopThatNotesTheRunningJobTask builds a loop over the harness whose record
// line callback asks, the moment a task starts, which job's task is running,
// which is the moment the program builds the status a screen draws the job from.
func aLoopThatNotesTheRunningJobTask(t *testing.T, built *harness) (*loop.Loop, *[]contract.TaskToRun, *int) {
	t.Helper()
	seen := &[]contract.TaskToRun{}
	starts := new(int)
	var made *loop.Loop
	options := built.options()
	options.RecordLine = func(line string) {
		if !strings.Contains(line, " started") {
			return
		}
		*starts++
		if fromJob, there := made.RunningJobTask(); there {
			*seen = append(*seen, fromJob)
		}
	}
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build the loop: %v", err)
	}
	return made, seen, starts
}

// TestTheLoopSaysWhichJobsTaskIsRunning is what lets the side panel show the
// job a person is watching: while a task of a job runs, the loop names the job
// and the task, and once the job's tasks are over it names nothing.
func TestTheLoopSaysWhichJobsTaskIsRunning(t *testing.T) {
	built := newHarness(t, twoTasksAndTheirReview(), scriptedTool("read", "the notes", "the notes"))
	jobID := aJobOfTwoTasks(t, built)
	made, seen, _ := aLoopThatNotesTheRunningJobTask(t, built)

	if _, err := made.RunNextJobTask(t.Context(), built.channel); err != nil {
		t.Fatalf("the loop could not run the job's tasks: %v", err)
	}

	if len(*seen) != 2 {
		t.Fatalf("the loop named a job's task at %d starts, want both tasks of the job: %+v", len(*seen), *seen)
	}
	for at, want := range []string{"t1", "t2"} {
		if (*seen)[at].JobID != jobID || (*seen)[at].TaskID != want {
			t.Errorf("start %d named job %s task %s, want job %s task %s",
				at+1, (*seen)[at].JobID, (*seen)[at].TaskID, jobID, want)
		}
	}
	if fromJob, there := made.RunningJobTask(); there {
		t.Errorf("the loop still names job %s task %s after the job's tasks are over", fromJob.JobID, fromJob.TaskID)
	}
}

// TestAPersonsTaskIsNoJobsTask holds the other half: a task that came from a
// message names no job, so the panel keeps its count of jobs.
func TestAPersonsTaskIsNoJobsTask(t *testing.T) {
	built := newHarness(t, closingScript("the notes are read"), scriptedTool("read", "the notes"))
	made, seen, starts := aLoopThatNotesTheRunningJobTask(t, built)

	if _, err := made.Run(t.Context(), built.task("read the notes")); err != nil {
		t.Fatalf("the loop could not run the task: %v", err)
	}

	if *starts != 1 {
		t.Fatalf("the task started %d times, want once", *starts)
	}
	if len(*seen) != 0 {
		t.Errorf("a person's task was named as a job's: %+v", *seen)
	}
}

// TestALoopRunsTheJobTaskItIsHanded is for the job driver in cmd/nerdgenie, which
// asks the job store for the due task itself so that it can hand the nightly
// self-check to the checker rather than the model. A task the driver has
// already claimed cannot be claimed again, so the loop has to take the task it
// is handed rather than ask for the next one, or the first task of every job
// the model makes would sit claimed and unrun until its budget ran out.
func TestALoopRunsTheJobTaskItIsHanded(t *testing.T) {
	built := newHarness(t, twoTasksAndTheirReview(), scriptedTool("read", "the notes", "the notes"))
	jobID := aJobOfTwoTasks(t, built)
	due, there, err := built.jobs.NextTask(t.Context(), built.clock.Now())
	if err != nil || !there {
		t.Fatalf("the job store handed out nothing: %v", err)
	}

	if err := built.loop.RunJobTask(t.Context(), due, built.channel); err != nil {
		t.Fatalf("the loop could not run the task it was handed: %v", err)
	}

	if !sentSomethingLike(built.channel.Sent(), "Job "+jobID+", report j"+jobID+".1: 1 of 2 tasks done.") {
		t.Errorf("the user was sent %v, want the first task's report, so the task the driver claimed is the one that ran",
			built.channel.Sent())
	}
	for _, task := range built.jobs.Tasks(jobID) {
		if !task.Done {
			t.Errorf("the task %s is not done after the loop ran the job through: %+v", task.TaskID, task)
		}
	}
}
