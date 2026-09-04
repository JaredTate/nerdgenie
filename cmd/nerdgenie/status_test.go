package main

import (
	"testing"

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
