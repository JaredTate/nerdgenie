package job_test

import (
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
)

func TestEveryCallSaysSoWhenTheDatabaseHasGoneRatherThanPretending(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Do a long thing.")
	taskID := holding.aTask(t, jobID, "the one task", time.Time{})
	if _, due := holding.nextTask(t, theEpoch()); !due {
		t.Fatal("the one task was not handed out to begin with")
	}
	if err := holding.jobs.Close(); err != nil {
		t.Fatalf("cannot close the job store: %v", err)
	}
	if err := holding.jobs.Close(); err != nil {
		t.Fatalf("closing the job store twice failed: %v", err)
	}
	if err := holding.eventLog.Close(); err != nil {
		t.Fatalf("cannot close the event log: %v", err)
	}

	for _, call := range []struct {
		name string
		make func() error
	}{
		{"listing the jobs", func() error { _, err := holding.jobs.List(ctx); return err }},
		{"asking for the next task", func() error { _, _, err := holding.jobs.NextTask(ctx, theEpoch()); return err }},
		{"finishing a task", func() error {
			_, err := holding.jobs.FinishTask(ctx, jobID, taskID, "the task finished", false)
			return err
		}},
		{"creating a job", func() error {
			_, err := holding.jobs.Create(ctx, contract.NewJob{Ask: "Another long thing."})
			return err
		}},
		{"creating a scheduled job", func() error {
			_, err := holding.jobs.Create(ctx, contract.NewJob{
				Ask:          "Post every hour.",
				Schedule:     &contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour},
				TaskTemplate: "post this hour's message",
			})
			return err
		}},
		{"adding a task", func() error {
			_, err := holding.jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "one more task"})
			return err
		}},
		{"writing the model's half", func() error {
			return holding.jobs.Update(ctx, jobID, record.Update{Why: "because it must be written"})
		}},
		{"pausing the job", func() error { return holding.jobs.Pause(ctx, jobID) }},
		{"resuming the job", func() error { return holding.jobs.Resume(ctx, jobID) }},
		{"running the job now", func() error { return holding.jobs.RunNow(ctx, jobID) }},
		{"writing a note", func() error { return holding.jobs.AppendNote(ctx, jobID, "a note") }},
		{"printing the jobs listing", func() error {
			_, err := holding.jobs.JobsCommand().Run(ctx, "", contract.CommandContext{})
			return err
		}},
		{"printing the cron listing", func() error {
			_, err := holding.jobs.CronCommand().Run(ctx, "", contract.CommandContext{})
			return err
		}},
	} {
		t.Run(call.name, func(t *testing.T) {
			if err := call.make(); err == nil {
				t.Errorf("%s worked with no database behind it, and it must say what is wrong", call.name)
			}
		})
	}
}

func TestASchedulesTickIsNotLostWhenTheJobCannotBeWritten(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aScheduledJob(t, "Post every hour.",
		contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour})
	if err := holding.eventLog.Close(); err != nil {
		t.Fatalf("cannot close the event log: %v", err)
	}

	_, _, err := holding.jobs.NextTask(t.Context(), theEpoch().Add(time.Hour))

	if err == nil {
		t.Fatal("a tick was made with no log to write it to, and the log is what makes a tick real")
	}
	if summary := holding.summaryOf(t, jobID); summary.NextRun != theEpoch().Add(time.Hour) {
		t.Errorf("the tick moved the next run to %s even though it could not be written down", summary.NextRun)
	}
}
