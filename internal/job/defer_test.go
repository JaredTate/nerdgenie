package job_test

import (
	"testing"
	"time"
)

// TestASecondGuardStopDefersTheTaskAndNextTaskPassesOverIt: on the night of
// 7 September the sky task stopped on the guard twice and the job waited nine
// hours for a person while twelve tasks that needed nothing from the sky sat
// untouched. After the job's one pick-up is spent, the store sets the task
// aside: the first Defer answers yes, NextTask passes over the deferred task
// while another task ahead of the job's last is unfinished and not deferred,
// the job's last task waits for the deferred one, the mark survives a restart
// because it rides in the job's state snapshot, and the record's own task
// list does not change. A job that is not there and a task not on the list
// are refused with an error naming them.
func TestASecondGuardStopDefersTheTaskAndNextTaskPassesOverIt(t *testing.T) {
	ctx := t.Context()
	holding := newJobs(t)
	jobID := holding.aJob(t, "Build the flight simulator.")
	sky := holding.aTask(t, jobID, "the sky and atmosphere", time.Time{})
	cockpit := holding.aTask(t, jobID, "the cockpit", time.Time{})
	holding.aTask(t, jobID, "the final regression", time.Time{})
	if next, due := holding.nextTask(t, theEpoch()); !due || next.TaskID != sky {
		t.Fatalf("the first task handed out is %+v (due %v), want the sky task %s", next, due, sky)
	}
	if again, err := holding.jobs.PickUpOnce(ctx, jobID, sky); err != nil || !again {
		t.Fatalf("the first guard stop's pick-up answered %v (error %v), want yes", again, err)
	}
	if again, err := holding.jobs.PickUpOnce(ctx, jobID, sky); err != nil || again {
		t.Fatalf("the second guard stop's pick-up answered %v (error %v), want no", again, err)
	}

	deferred, err := holding.jobs.Defer(ctx, jobID, sky)
	if err != nil || !deferred {
		t.Fatalf("the first deferral answered %v (error %v), want yes: the task goes to the back of the line", deferred, err)
	}

	if next, due := holding.nextTask(t, theEpoch()); !due || next.TaskID != cockpit {
		t.Errorf("after the deferral the next task is %+v (due %v), want the cockpit task %s: a deferred task is passed over while another is unfinished", next, due, cockpit)
	}
	holding = holding.restart(t)
	if next, due := holding.nextTask(t, theEpoch()); due {
		t.Errorf("after a restart the task %+v was handed out while the cockpit is still running: the deferred task waits behind it, and the last task waits for the deferred one", next)
	}
	held, err := holding.jobs.Load(ctx, jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if len(held.Work.Tasks) != 3 || held.Work.Tasks[0].TaskID != sky || held.Work.Tasks[0].Done {
		t.Errorf("the record's task list reads %+v, want the sky task still first and unfinished: the job's state holds the mark, not the record", held.Work.Tasks)
	}
	if _, err := holding.jobs.Defer(ctx, jobID, "t99"); err == nil {
		t.Error("a task that is not on the list was deferred without an error naming it")
	}
	if _, err := holding.jobs.Defer(ctx, "99", sky); err == nil {
		t.Error("a job that is not there had a task deferred without an error naming it")
	}
}

// TestADeferredTaskIsTakenAgainWhenOnlyDeferredTasksRemain: two tasks of a
// job are deferred and the one other task ahead of the last finishes; the
// deferred tasks are then handed out again in their order, before the last
// task, and with no failure counted, because the claim a deferred task held
// was let go when it was set aside rather than left to run out and be
// written down as a dead process's.
func TestADeferredTaskIsTakenAgainWhenOnlyDeferredTasksRemain(t *testing.T) {
	ctx := t.Context()
	holding := newJobs(t)
	jobID := holding.aJob(t, "Build the flight simulator.")
	sky := holding.aTask(t, jobID, "the sky and atmosphere", time.Time{})
	cockpit := holding.aTask(t, jobID, "the cockpit", time.Time{})
	terrain := holding.aTask(t, jobID, "the terrain", time.Time{})
	regression := holding.aTask(t, jobID, "the final regression", time.Time{})
	for _, taskID := range []string{sky, cockpit} {
		if next, due := holding.nextTask(t, theEpoch()); !due || next.TaskID != taskID {
			t.Fatalf("the task handed out is %+v (due %v), want %s", next, due, taskID)
		}
		if deferred, err := holding.jobs.Defer(ctx, jobID, taskID); err != nil || !deferred {
			t.Fatalf("deferring task %s answered %v (error %v), want yes", taskID, deferred, err)
		}
	}
	if next, due := holding.nextTask(t, theEpoch()); !due || next.TaskID != terrain {
		t.Fatalf("with two tasks deferred the next task is %+v (due %v), want the terrain task %s", next, due, terrain)
	}
	holding.finish(t, jobID, terrain, "the terrain is drawn", false)

	// Long after the claims the deferred tasks held would have run out.
	later := theEpoch().Add(3 * time.Hour)
	next, due := holding.nextTask(t, later)
	if !due || next.TaskID != sky {
		t.Fatalf("once only deferred tasks remain ahead of the last the next task is %+v (due %v), want the sky task %s, the first deferred in the list's order", next, due, sky)
	}
	if summary := holding.summaryOf(t, jobID); summary.FailuresInARow != 0 {
		t.Errorf("the job counts %d failures, want none: the claim a deferred task held was let go, not left to run out", summary.FailuresInARow)
	}
	holding.finish(t, jobID, sky, "the sky is drawn", false)
	if next, due := holding.nextTask(t, later); !due || next.TaskID != cockpit {
		t.Errorf("after the sky the next task is %+v (due %v), want the cockpit task %s, the second deferred", next, due, cockpit)
	}
	holding.finish(t, jobID, cockpit, "the cockpit is built", false)
	if next, due := holding.nextTask(t, later); !due || next.TaskID != regression {
		t.Errorf("after the deferred tasks the next task is %+v (due %v), want the final regression %s, last", next, due, regression)
	}
}

// TestATaskDeferredTwiceIsNotDeferredAgain: the deferral is once per task,
// the way the pick-up is, so a task that stalls the same way after it has
// come back is put down for a person; the count is per task, survives a
// restart, and a finished task is refused with an error naming it.
func TestATaskDeferredTwiceIsNotDeferredAgain(t *testing.T) {
	ctx := t.Context()
	holding := newJobs(t)
	jobID := holding.aJob(t, "Build the flight simulator.")
	sky := holding.aTask(t, jobID, "the sky and atmosphere", time.Time{})
	cockpit := holding.aTask(t, jobID, "the cockpit", time.Time{})

	if first, err := holding.jobs.Defer(ctx, jobID, sky); err != nil || !first {
		t.Fatalf("the first deferral answered %v (error %v), want yes", first, err)
	}
	if second, err := holding.jobs.Defer(ctx, jobID, sky); err != nil || second {
		t.Errorf("the second deferral answered %v (error %v), want no: a task is set aside once", second, err)
	}
	holding = holding.restart(t)
	if third, err := holding.jobs.Defer(ctx, jobID, sky); err != nil || third {
		t.Errorf("after a restart the deferral answered %v (error %v), want no: the once is written into the job", third, err)
	}
	if other, err := holding.jobs.Defer(ctx, jobID, cockpit); err != nil || !other {
		t.Errorf("the other task's first deferral answered %v (error %v), want yes: the once is per task", other, err)
	}
	holding.finish(t, jobID, cockpit, "the cockpit is built", false)
	if _, err := holding.jobs.Defer(ctx, jobID, cockpit); err == nil {
		t.Error("a finished task was deferred without an error naming it, and there is nothing of it left to set aside")
	}
}
