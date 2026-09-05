// The test for the store waking when a task finishes. The loop runs one task
// of a job per call now, and the driver takes the next on its next look; a
// task with no date makes no moment for the wait to sleep until, so without a
// wake the second task of a job started a minute after the first ended.
package job_test

import (
	"testing"
	"time"
)

func TestAFinishedTaskWakesTheStoreSoTheNextTaskStartsAtOnce(t *testing.T) {
	holding := newJobs(t)
	holding.jobs.OnlyStartWorkWhen(func() bool { return true })
	jobID := holding.aJob(t, "Run the campaign this month.")
	first := holding.aTask(t, jobID, "post the anniversary tweet", time.Time{})
	second := holding.aTask(t, jobID, "draft the blog piece", time.Time{})
	// The first task is handed out, which spends the wake the job's making
	// left, and the store is then asleep for the whole of its clamp with the
	// undated second task waiting behind the first.
	if handed, due := holding.nextTask(t, theEpoch()); !due || handed.TaskID != first {
		t.Fatalf("the first task was not handed out to begin with: %+v (due %v)", handed, due)
	}
	waiting := waitInTheBackground(holding)
	waitForSleeper(t, holding)

	holding.finish(t, jobID, first, "posted, 236 characters, link saved", false)

	mustComeBackAtOnce(t, waiting, "a task finished while the store waited")
	next, due := holding.nextTask(t, theEpoch())
	if !due || next.TaskID != second {
		t.Errorf("after the wake the next task is %+v (due %v), want %s handed out with the clock never moved", next, due, second)
	}
}
