package main

import (
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// TestTheStatusCarriesTheJobOfTheRunningTask is what the side panel draws the
// job from: the job's number, its ask on one line, which of its tasks is
// running, and its task list one task per line with a mark on every task that
// is done.
func TestTheStatusCarriesTheJobOfTheRunningTask(t *testing.T) {
	held := contract.Record{
		Goal: contract.Goal{Ask: "run the campaign\nfor the month"},
		Work: contract.Work{Tasks: []contract.JobTask{
			{TaskID: "t1", Text: "post the tweet", Done: true, ReportID: "j4.1"},
			{TaskID: "t2", Text: "write the summary"},
		}},
	}
	fields := map[string]string{}

	fillTheJobFields(fields, contract.TaskToRun{JobID: "4", TaskID: "t2"}, held)

	for field, want := range map[string]string{
		contract.StatusFieldJob:      "4",
		contract.StatusFieldJobAsk:   "run the campaign for the month",
		contract.StatusFieldJobTask:  "t2",
		contract.StatusFieldJobTasks: "[x] t1 post the tweet\n[ ] t2 write the summary",
	} {
		if fields[field] != want {
			t.Errorf("the status carries %s = %q, want %q", field, fields[field], want)
		}
	}
}

// TestTheStatusSaysThereIsNoJobWhenNoJobsTaskIsRunning holds that the job
// field is sent empty rather than left out, so that a screen which drew a job
// a moment ago goes back to its count of jobs rather than keeping the old one.
func TestTheStatusSaysThereIsNoJobWhenNoJobsTaskIsRunning(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	fields := running.statusForAScreen()

	for _, field := range []string{
		contract.StatusFieldJob, contract.StatusFieldJobAsk, contract.StatusFieldJobTask, contract.StatusFieldJobTasks,
	} {
		value, sent := fields[field]
		if !sent {
			t.Errorf("the status does not carry %s at all while nothing runs, so a screen keeps whatever job it drew last", field)
		}
		if value != "" {
			t.Errorf("the status carries %s = %q while no task runs", field, value)
		}
	}
}

// TestTheStatusCarriesWhenTheJobAndItsTasksBegan is what the side panel
// draws the running times from: the moment the job was made, and one line
// per task that has started with its start and its finish, in the order of
// the job's own list. A job whose store kept no moments sends both empty.
func TestTheStatusCarriesWhenTheJobAndItsTasksBegan(t *testing.T) {
	t0 := time.Date(2026, 9, 7, 23, 26, 10, 0, time.UTC)
	held := contract.Record{Work: contract.Work{Tasks: []contract.JobTask{
		{TaskID: "t1", Text: "post the tweet", Done: true},
		{TaskID: "t2", Text: "write the summary"},
		{TaskID: "t3", Text: "post the summary"},
	}}}
	timing := contract.JobTiming{Started: t0, Tasks: map[string]contract.TaskTiming{
		"t2": {Started: t0.Add(4 * time.Minute)},
		"t1": {Started: t0.Add(30 * time.Second), Finished: t0.Add(3*time.Minute + 42*time.Second)},
	}}
	fields := map[string]string{}

	fillTheJobTiming(fields, held, timing)

	if fields[contract.StatusFieldJobStarted] != "2026-09-07T23:26:10Z" {
		t.Errorf("the job's start reads %q", fields[contract.StatusFieldJobStarted])
	}
	want := "t1 2026-09-07T23:26:40Z 2026-09-07T23:29:52Z\nt2 2026-09-07T23:30:10Z -"
	if fields[contract.StatusFieldJobTaskTimes] != want {
		t.Errorf("the task times read %q, want %q", fields[contract.StatusFieldJobTaskTimes], want)
	}

	empty := map[string]string{}
	fillTheJobTiming(empty, held, contract.JobTiming{})
	if empty[contract.StatusFieldJobStarted] != "" || empty[contract.StatusFieldJobTaskTimes] != "" {
		t.Errorf("a job with no moments sends %q and %q, want both empty", empty[contract.StatusFieldJobStarted], empty[contract.StatusFieldJobTaskTimes])
	}
}
