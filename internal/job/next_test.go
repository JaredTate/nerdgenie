package job_test

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/job"
)

func TestTheReportLandsInTheJobAndTheNextTaskIsHandedOut(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aJob(t, "Run the campaign this month.")
	first := holding.aTask(t, jobID, "post the anniversary tweet", time.Time{})
	second := holding.aTask(t, jobID, "draft the blog piece", time.Time{})

	next, due := holding.nextTask(t, theEpoch())
	if !due || next.JobID != jobID || next.TaskID != first || next.Unattended {
		t.Fatalf("the first task handed out is %+v (due %v), want task %s of job %s, attended", next, due, first, jobID)
	}
	next, due = holding.nextTask(t, theEpoch())
	if !due || next.TaskID != second {
		t.Fatalf("the second call handed out %+v (due %v), and a task somebody is already running is nobody else's to start", next, due)
	}
	if _, due := holding.nextTask(t, theEpoch()); due {
		t.Error("a third task was handed out, and the job holds only two")
	}

	reportID := holding.finish(t, jobID, first, "posted, 236 characters, link saved", false)
	if reportID != contract.ReportID(jobID, 1) {
		t.Errorf("the report identifier is %q, want %q", reportID, contract.ReportID(jobID, 1))
	}
	held, err := holding.jobs.Load(t.Context(), jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if !held.Work.Tasks[0].Done || held.Work.Tasks[0].ReportID != reportID {
		t.Errorf("the finished task is %+v, want it done with report %s behind it", held.Work.Tasks[0], reportID)
	}
	if _, err := holding.jobs.FinishTask(t.Context(), jobID, first, "posted again", false); err == nil {
		t.Error("a finished task took a second report, and its report is written once")
	}
	if _, err := holding.jobs.FinishTask(t.Context(), jobID, "t99", "nothing", false); err == nil {
		t.Error("a task that is not on the list took a report")
	}
}

func TestATaskWithADateWaitsForIt(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aJob(t, "Post on the anniversary itself.")
	taskID := holding.aTask(t, jobID, "post for day three", theEpoch().Add(24*time.Hour))

	if _, due := holding.nextTask(t, theEpoch()); due {
		t.Fatal("a task dated tomorrow was handed out today")
	}
	next, due := holding.nextTask(t, theEpoch().Add(25*time.Hour))
	if !due || next.TaskID != taskID {
		t.Errorf("a day later the task is %+v (due %v), want task %s", next, due, taskID)
	}
	held, err := holding.jobs.Load(t.Context(), jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if held.Work.Tasks[0].DueAt == "" {
		t.Errorf("the task's date is not written into the record: %+v", held.Work.Tasks[0])
	}
}

func TestATaskDatedNextWeekDoesNotHoldUpTheOneBehindIt(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aJob(t, "Do a long thing.")
	holding.aTask(t, jobID, "the task for next week", theEpoch().Add(7*24*time.Hour))
	soon := holding.aTask(t, jobID, "the task that can start at once", time.Time{})

	next, due := holding.nextTask(t, theEpoch())

	if !due || next.TaskID != soon {
		t.Errorf("the task handed out is %+v (due %v), want %s, because a task dated next week waits on its own", next, due, soon)
	}
}

func TestOnlyARunningJobHandsOutWork(t *testing.T) {
	ctx := t.Context()
	for _, stopping := range []struct {
		name  string
		stop  func(*opened, string) error
		state contract.JobState
	}{
		{"paused", func(holding *opened, jobID string) error { return holding.jobs.Pause(ctx, jobID) }, contract.JobPaused},
		{"switched off", func(holding *opened, jobID string) error { return holding.jobs.SwitchOff(ctx, jobID) }, contract.JobOff},
	} {
		t.Run(stopping.name, func(t *testing.T) {
			holding := newJobs(t)
			jobID := holding.aJob(t, "Do a long thing.")
			holding.aTask(t, jobID, "the one task", time.Time{})

			if err := stopping.stop(holding, jobID); err != nil {
				t.Fatalf("cannot stop the job: %v", err)
			}

			if summary := holding.summaryOf(t, jobID); summary.State != stopping.state {
				t.Errorf("the job is %q, want %q", summary.State, stopping.state)
			}
			if next, due := holding.nextTask(t, theEpoch()); due {
				t.Errorf("a job that is %s handed out %+v", stopping.name, next)
			}
		})
	}
}

func TestRunNowTakesTheDateOffTheNextTask(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Do a long thing.")
	taskID := holding.aTask(t, jobID, "the one task", theEpoch().Add(24*time.Hour))
	if err := holding.jobs.Pause(ctx, jobID); err != nil {
		t.Fatalf("cannot pause the job: %v", err)
	}

	if err := holding.jobs.RunNow(ctx, jobID); err != nil {
		t.Fatalf("cannot run the job now: %v", err)
	}

	next, due := holding.nextTask(t, theEpoch())
	if !due || next.TaskID != taskID {
		t.Errorf("after run now the next task is %+v (due %v), and run now means start it without waiting", next, due)
	}
	if err := holding.jobs.RunNow(ctx, "99"); err == nil {
		t.Error("a job that is not there was run now without an error naming it")
	}
}

func TestATaskPastItsBudgetIsGivenUpAndCountedAsAFailure(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aJob(t, "Do a long thing.")
	taskID := holding.aTask(t, jobID, "the task that never comes back", time.Time{})
	if _, due := holding.nextTask(t, theEpoch()); !due {
		t.Fatal("the task was not handed out to begin with")
	}

	next, due := holding.nextTask(t, theEpoch().Add(job.TaskBudget+time.Minute))

	if !due || next.TaskID != taskID {
		t.Errorf("after the budget ran out the task is %+v (due %v), want %s offered again", next, due, taskID)
	}
	if failures := holding.summaryOf(t, jobID).FailuresInARow; failures != 1 {
		t.Errorf("giving up a task past its budget counted %d failures, want 1", failures)
	}
	held, err := holding.jobs.Load(t.Context(), jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if len(held.Work.Results) != 1 {
		t.Errorf("the job holds %d reports, want the one saying the task was given up", len(held.Work.Results))
	}
}

func TestWaitSleepsUntilThereCouldBeWorkAndNeverLongerThanTheClamp(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aJob(t, "Do a long thing.")
	holding.aTask(t, jobID, "the task for next week", theEpoch().Add(7*24*time.Hour))
	// A task added wakes the store until it looks, so the store looks first and
	// finds nothing due, which is the state a wait sleeps in.
	holding.nextTask(t, theEpoch())

	waiting := make(chan error, 1)
	go func() { waiting <- holding.jobs.Wait(t.Context()) }()
	waitForSleeper(t, holding)
	holding.clock.Advance(job.TimerClamp)

	if err := <-waiting; err != nil {
		t.Fatalf("waiting for work failed: %v", err)
	}

	holding.aTask(t, jobID, "the task for half a minute from now", theEpoch().Add(90*time.Second))
	holding.nextTask(t, theEpoch())
	go func() { waiting <- holding.jobs.Wait(t.Context()) }()
	waitForSleeper(t, holding)
	holding.clock.Advance(30 * time.Second)
	if err := <-waiting; err != nil {
		t.Fatalf("waiting for the nearer task failed: %v", err)
	}
}

// waitForSleeper waits until the store is really asleep, so that the clock is
// never moved before there is anybody to wake.
func waitForSleeper(t *testing.T, holding *opened) {
	t.Helper()
	for range 1000 {
		if holding.clock.Sleepers() > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the job store never went to sleep waiting for work")
}

func TestNoTaskIsHandedOutWhileWorkMayNotStart(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aJob(t, "Post the update every morning.")
	taskID := holding.aTask(t, jobID, "write and post today's message", time.Time{})
	mayStart := false
	holding.jobs.OnlyStartWorkWhen(func() bool { return mayStart })

	if next, due := holding.nextTask(t, theEpoch()); due {
		t.Errorf("task %+v was handed out while an update was draining the agent or the crash-loop breaker was tripped", next)
	}

	mayStart = true

	next, due := holding.nextTask(t, theEpoch())
	if !due || next.TaskID != taskID {
		t.Errorf("once work may start again the store handed out %+v (due %v), want task %s", next, due, taskID)
	}
}

func TestWaitRestsBeforeItComesBackSoADriverCannotSpin(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aJob(t, "Do the thing that is already overdue.")
	holding.aTask(t, jobID, "the task that was due an hour ago", theEpoch().Add(-time.Hour))

	if err := holding.jobs.Wait(t.Context()); err != nil {
		t.Fatalf("the first wait, with work already overdue, failed: %v", err)
	}
	waiting := make(chan error, 1)
	go func() { waiting <- holding.jobs.Wait(t.Context()) }()
	waitForTheRest(t, holding, waiting)

	holding.clock.Advance(time.Second)

	select {
	case err := <-waiting:
		if err != nil {
			t.Fatalf("the second wait failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("one second of the clock did not end the rest between two waits, and one second is the longest that rest may be")
	}
}

// waitForTheRest waits until the store is resting between two waits, and fails
// the test when the wait came straight back instead. A driver that cannot take
// the work yet, because a task of its own is running, asks again as fast as the
// processor allows when the wait comes straight back.
func waitForTheRest(t *testing.T, holding *opened, waiting <-chan error) {
	t.Helper()
	for range 1000 {
		select {
		case err := <-waiting:
			t.Fatalf("the wait came straight back (error %v) although work was already overdue, so a driver that cannot take the work yet spins", err)
		default:
		}
		if holding.clock.Sleepers() > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the job store never rested between two waits")
}

// TestAClaimOnATaskOfAPausedJobIsLeftAloneWhenItsBudgetRunsOut is the rule a
// person's stop needs: the loop pauses a job on the task the person stopped
// and keeps the claim on it, so that nothing of the job runs until they say to
// carry on. A job that is not running has nothing running, so a claim on one
// of its tasks is not a dead process's, and it is not given up, counted as a
// failure, and offered again an hour later. Once the job runs again the claim
// is a claim like any other, and the old rule takes it.
func TestAClaimOnATaskOfAPausedJobIsLeftAloneWhenItsBudgetRunsOut(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Do a long thing.")
	taskID := holding.aTask(t, jobID, "the task the person stopped", time.Time{})
	if _, due := holding.nextTask(t, theEpoch()); !due {
		t.Fatal("the task was not handed out to begin with")
	}
	if err := holding.jobs.Pause(ctx, jobID); err != nil {
		t.Fatalf("cannot pause the job: %v", err)
	}

	if next, due := holding.nextTask(t, theEpoch().Add(job.TaskBudget+time.Minute)); due {
		t.Errorf("the paused job handed out %+v an hour after the stop, and nothing of a paused job runs", next)
	}

	if failures := holding.summaryOf(t, jobID).FailuresInARow; failures != 0 {
		t.Errorf("the claim on the paused job's task was counted as %d failures, want none: the person stopped it, no process died", failures)
	}
	held, err := holding.jobs.Load(ctx, jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if len(held.Work.Results) != 0 || len(held.Lessons.Failures) != 0 {
		t.Errorf("the paused job holds %d reports and %d failures, want none written for a task the person stopped",
			len(held.Work.Results), len(held.Lessons.Failures))
	}
	if err := holding.jobs.Resume(ctx, jobID); err != nil {
		t.Fatalf("cannot resume the job: %v", err)
	}
	next, due := holding.nextTask(t, theEpoch().Add(job.TaskBudget+2*time.Minute))
	if !due || next.TaskID != taskID {
		t.Errorf("once the job runs again the task is %+v (due %v), want %s offered again, because a job set running again lets go of the claims its paused run held", next, due, taskID)
	}
	if failures := holding.summaryOf(t, jobID).FailuresInARow; failures != 0 {
		t.Errorf("once the job ran again the old claim was counted as %d failures, want none: it was let go when the job ran again, not left to run out", failures)
	}
}

// TestAJobSetRunningAgainHandsOutThePutDownTaskFirstWithItsOldClaimReleased
// is the review's first finding. A put-down task kept the claim its run held,
// and a job set running again by any way but the exact carry-on word, such as
// "/cron run" or "/resume", skipped that task as running and handed out the
// one after it; an hour later the old claim ran out and the task was written
// into the job as a failure. Setting a paused job running again lets go of
// the claims its tasks hold, so the put-down task is handed out first and its
// old claim is never counted as a failure.
func TestAJobSetRunningAgainHandsOutThePutDownTaskFirstWithItsOldClaimReleased(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Do two things.")
	first := holding.aTask(t, jobID, "the task the person stopped", time.Time{})
	second := holding.aTask(t, jobID, "the task after it", time.Time{})
	if next, due := holding.nextTask(t, theEpoch()); !due || next.TaskID != first {
		t.Fatalf("the first task was not handed out to begin with: %+v (due %v)", next, due)
	}
	if err := holding.jobs.Pause(ctx, jobID); err != nil {
		t.Fatalf("cannot pause the job: %v", err)
	}

	if err := holding.jobs.RunNow(ctx, jobID); err != nil {
		t.Fatalf("cannot run the job now: %v", err)
	}

	next, due := holding.nextTask(t, theEpoch().Add(time.Minute))
	if !due || next.TaskID != first {
		t.Errorf("a minute after run now the next task is %+v (due %v), want %s, the task the job was paused on, and not %s", next, due, first, second)
	}
	// The claim the paused run held would have run out an hour after it was
	// taken. It was let go instead, so nothing runs out, nothing is counted,
	// and the task handed out again is still the one running.
	if next, due := holding.nextTask(t, theEpoch().Add(job.TaskBudget+30*time.Second)); due {
		t.Errorf("past the old claim's hour the store handed out %+v, and the task handed out after run now is still running", next)
	}
	if failures := holding.summaryOf(t, jobID).FailuresInARow; failures != 0 {
		t.Errorf("the old claim was counted as %d failures, want none", failures)
	}
	held, err := holding.jobs.Load(ctx, jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if len(held.Work.Results) != 0 {
		t.Errorf("the job holds %d reports, want none: no task failed", len(held.Work.Results))
	}
}

// theTimeAWakeIsGiven is how long, in real time, a wait is given to come back
// after the store is woken. A wake is a channel send, so a wait that takes
// longer than this was sleeping on the clock and not listening.
const theTimeAWakeIsGiven = 2 * time.Second

// waitInTheBackground starts one wait and hands back the channel its answer
// arrives on.
func waitInTheBackground(holding *opened) <-chan error {
	waiting := make(chan error, 1)
	go func() { waiting <- holding.jobs.Wait(context.Background()) }()
	return waiting
}

// mustComeBackAtOnce fails the test when the wait does not come back within
// the time a wake is given, in real time and with the clock never moved.
func mustComeBackAtOnce(t *testing.T, waiting <-chan error, what string) {
	t.Helper()
	select {
	case err := <-waiting:
		if err != nil {
			t.Fatalf("%s, and the wait failed: %v", what, err)
		}
	case <-time.After(theTimeAWakeIsGiven):
		t.Fatalf("%s, and the wait was still asleep %s later, so the store was sleeping on the clock rather than listening for a wake", what, theTimeAWakeIsGiven)
	}
}

// mustStayAsleep fails the test when the wait comes back on its own, which is
// what a store that has looked for work and found nothing new must not do.
func mustStayAsleep(t *testing.T, waiting <-chan error, what string) {
	t.Helper()
	select {
	case err := <-waiting:
		t.Fatalf("%s, and the wait came back (error %v) with nothing new to look at", what, err)
	case <-time.After(50 * time.Millisecond):
	}
}

// TestAJobMadeWhileTheStoreWaitsWakesItAtOnce is the acid test's first
// complaint: a person's ask became a job, and its first task started a minute
// later, because the store was asleep for the whole of its clamp and nothing
// told it a job had been made. A job made or a task added while the store
// waits wakes it, so the first task is handed out at once.
func TestAJobMadeWhileTheStoreWaitsWakesItAtOnce(t *testing.T) {
	holding := newJobs(t)
	holding.jobs.OnlyStartWorkWhen(func() bool { return true })
	// The store has looked once and found nothing, so it is asleep for the whole
	// of its clamp when the job is made.
	holding.nextTask(t, theEpoch())
	waiting := waitInTheBackground(holding)
	waitForSleeper(t, holding)

	jobID := holding.aJob(t, "Run the campaign this month.")
	taskID := holding.aTask(t, jobID, "post the anniversary tweet", time.Time{})

	mustComeBackAtOnce(t, waiting, "a job with a task was made while the store waited")
	next, due := holding.nextTask(t, theEpoch())
	if !due || next.TaskID != taskID {
		t.Errorf("after the wake the next task is %+v (due %v), want %s handed out with the clock never moved", next, due, taskID)
	}
}

// TestAWakeLastsUntilTheStoreLooksForWork pins the two ends of a wake. A
// driver that is woken while its own task runs cannot take the work yet and
// waits again, so the wake must still be there when it does: it lasts until
// the store is asked for work. And once the store has looked, the wake is
// spent, so a store with nothing new goes back to sleep rather than spinning.
func TestAWakeLastsUntilTheStoreLooksForWork(t *testing.T) {
	holding := newJobs(t)
	holding.nextTask(t, theEpoch())
	jobID := holding.aJob(t, "Run the campaign this month.")
	holding.aTask(t, jobID, "post the anniversary tweet", time.Time{})

	// Nobody was waiting when the job was made, and nobody has looked since.
	mustComeBackAtOnce(t, waitInTheBackground(holding), "a job was made before the wait began")
	holding.clock.Advance(job.RestBetweenWaits)
	mustComeBackAtOnce(t, waitInTheBackground(holding), "the store was woken and has not looked for work yet")

	holding.nextTask(t, theEpoch())
	holding.clock.Advance(job.RestBetweenWaits)
	waiting := waitInTheBackground(holding)
	waitForSleeper(t, holding)
	mustStayAsleep(t, waiting, "the store has looked since it was woken")
}

// TestSettingAJobRunningAgainWakesTheStore covers the third way work appears
// while the store waits: a paused job told to run now, or picked up again by
// the person, has a task due at once.
func TestSettingAJobRunningAgainWakesTheStore(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Do a long thing.")
	taskID := holding.aTask(t, jobID, "the task the person paused on", theEpoch().Add(24*time.Hour))
	if err := holding.jobs.Pause(ctx, jobID); err != nil {
		t.Fatalf("cannot pause the job: %v", err)
	}
	holding.nextTask(t, theEpoch())
	waiting := waitInTheBackground(holding)
	waitForSleeper(t, holding)

	if err := holding.jobs.RunNow(ctx, jobID); err != nil {
		t.Fatalf("cannot run the job now: %v", err)
	}

	mustComeBackAtOnce(t, waiting, "a paused job was told to run now while the store waited")
	next, due := holding.nextTask(t, theEpoch())
	if !due || next.TaskID != taskID {
		t.Errorf("after run now the next task is %+v (due %v), want %s", next, due, taskID)
	}
}

// TestAStoreThatHasJustOpenedLooksBeforeItSleeps is the restart half of the
// same complaint: a store that has just opened has never looked for work, and
// a job left running by the process before it must not wait out a clamp.
func TestAStoreThatHasJustOpenedLooksBeforeItSleeps(t *testing.T) {
	holding := newJobs(t)

	mustComeBackAtOnce(t, waitInTheBackground(holding), "the store has just opened and has not looked for work")
}
