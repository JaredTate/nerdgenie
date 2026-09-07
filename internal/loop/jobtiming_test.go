package loop_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theTimeATimedTaskTakes is how far the reading tool moves the clock in
// each task of the timed job, an odd span so that the words on the report
// are the store's moments and not a rounded guess.
const theTimeATimedTaskTakes = 3*time.Minute + 12*time.Second

// aToolThatTakesTime is a read tool that moves the fake clock when it runs,
// which is how a task is made to take a known time without a real clock.
type aToolThatTakesTime struct {
	clock *testkit.FakeClock
	by    time.Duration
}

func (tool aToolThatTakesTime) Spec() contract.ToolSpec {
	return contract.ToolSpec{Name: "read", Description: "A read that takes a while, for the tests.", Classes: []contract.PermissionClass{contract.ClassRead}}
}

func (tool aToolThatTakesTime) Run(context.Context, json.RawMessage) (contract.ToolOutput, error) {
	tool.clock.Advance(tool.by)
	return contract.ToolOutput{Text: "the notes"}, nil
}

// twoTimedTasksAndTheirReview is the script for a job of two tasks that each
// read the notes and close, with the architecture question every job task
// is asked at its end answered "none" so that it does not eat the next
// task's steps, and the job's own review at the end.
func twoTimedTasksAndTheirReview() []testkit.Step {
	steps := []testkit.Step{}
	for range 2 {
		steps = append(steps, closingScript("the post is up")...)
		steps = append(steps, answerStep("none"))
	}
	return append(steps, aReviewReply("Keep posting at the same hour every day."))
}

// TestAFinishedTaskAndAFinishedJobSayWhatTheyTook: the person asked to see
// how long a task and a job ran. Each task's progress line ends with what
// the task took, read off the store's own moments, and the job's closing
// line says what the whole job took from the moment it was made.
func TestAFinishedTaskAndAFinishedJobSayWhatTheyTook(t *testing.T) {
	clock := testkit.NewFakeClock(theStartOfTime)
	built := newHarness(t, twoTimedTasksAndTheirReview(), aToolThatTakesTime{clock: clock, by: theTimeATimedTaskTakes})
	built.clock = clock
	built.jobs = &fakeJobThatResumes{FakeJob: testkit.NewFakeJob(clock)}
	built.loop = mustBuild(t, built)
	jobID := aJobOfTwoTasks(t, built)

	runTheJobToTheEnd(t, built.loop, built.channel)

	sent := built.channel.Sent()
	if !sentSomethingLike(sent, "Job "+jobID+", report j"+jobID+".1: 1 of 2 tasks done. Task t1 took 3m 12s.") {
		t.Errorf("the first task's report does not say what the task took; the user was sent %v", sent)
	}
	if !sentSomethingLike(sent, "Task t2 took 3m 12s.") {
		t.Errorf("the second task's report does not say what the task took; the user was sent %v", sent)
	}
	if !sentSomethingLike(sent, "Job "+jobID+" is finished: every one of its 2 tasks is done, in 6m 24s.") {
		t.Errorf("the job's closing line does not say what the job took; the user was sent %v", sent)
	}
}
