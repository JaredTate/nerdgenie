package main

import (
	"context"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// TestPlanLinesMarkEachStepDoneOrNot holds the wire shape the side panel reads
// back: one plan step per line, each beginning "[x] " when the step is done and
// "[ ] " when it is not, the same marks a job's task list carries.
func TestPlanLinesMarkEachStepDoneOrNot(t *testing.T) {
	plan := []contract.PlanStep{
		{Number: 1, Text: "read the file", Done: true},
		{Number: 2, Text: "write the\nchange"},
	}

	if got, want := planLines(plan), "[x] read the file\n[ ] write the change"; got != want {
		t.Errorf("the plan lines are %q, want %q", got, want)
	}
	if got := planLines(nil); got != "" {
		t.Errorf("an empty plan wrote %q, want nothing", got)
	}
}

// TestTheStatusCarriesTheRunningTasksPlan holds job 2: while a task runs, the
// status a screen reads carries that task's plan from its record, one step per
// line with a mark on every step that is done.
func TestTheStatusCarriesTheRunningTasksPlan(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	ctx := context.Background()
	keeper, err := record.New(ctx, running.events, record.Start{
		Kind: contract.RecordTask, ID: "7", Origin: contract.TerminalChannelName, Ask: "do the work", RoundsLeft: 10, MinutesLeft: 10,
	})
	if err != nil {
		t.Fatalf("cannot start the record of task 7: %v", err)
	}
	if err := keeper.Apply(ctx, record.Update{Plan: []string{"first step", "second step"}}); err != nil {
		t.Fatalf("cannot write the plan of task 7: %v", err)
	}
	resultID, err := keeper.AddResult(ctx, "did the first step", "the whole of it")
	if err != nil {
		t.Fatalf("cannot add a result to task 7: %v", err)
	}
	if err := keeper.MarkPlanStep(ctx, 1, resultID); err != nil {
		t.Fatalf("cannot mark the first step of task 7 done: %v", err)
	}

	fields := map[string]string{}
	running.fillThePlan(fields, "7")

	if got, want := fields[contract.StatusFieldPlan], "[x] first step\n[ ] second step"; got != want {
		t.Errorf("the status carries the plan %q, want %q", got, want)
	}
}

// TestTheStatusOmitsThePlanWhenNoTaskRuns holds that the plan field is sent
// empty rather than left out while nothing runs, so a screen that drew a plan a
// moment ago clears it rather than keeping the last one.
func TestTheStatusOmitsThePlanWhenNoTaskRuns(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	fields := running.statusForAScreen()

	value, sent := fields[contract.StatusFieldPlan]
	if !sent {
		t.Error("the status does not carry the plan field at all while nothing runs, so a screen keeps whatever plan it drew last")
	}
	if value != "" {
		t.Errorf("the status carries the plan %q while no task runs", value)
	}
}

// TestTheStatusCarriesNoPlanWhenTheRecordIsGone holds that a running number
// with no record behind it leaves the plan empty rather than failing, which is
// what the moment before a task's first checkpoint looks like.
func TestTheStatusCarriesNoPlanWhenTheRecordIsGone(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	fields := map[string]string{}
	running.fillThePlan(fields, "404")

	if got := fields[contract.StatusFieldPlan]; got != "" {
		t.Errorf("the status carries the plan %q for a task with no record, want it empty", got)
	}
}

// TestTheStatusCountsTheWaitingJobs holds job 2: the status a screen reads
// carries how many jobs are waiting, as a number, which the side panel draws
// when no job's task is running.
func TestTheStatusCountsTheWaitingJobs(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	ctx := context.Background()
	for _, ask := range []string{"water the plants weekly", "back up the disk nightly"} {
		if _, err := running.jobs.Create(ctx, contract.NewJob{Ask: ask}); err != nil {
			t.Fatalf("cannot create a job to wait: %v", err)
		}
	}

	fields := running.statusForAScreen()

	if got := fields[contract.StatusFieldJobs]; got != "2" {
		t.Errorf("the status counts %q waiting jobs, want 2", got)
	}
}

// TestTheStatusOmitsTheJobCountWhenNoneWait holds that the count is sent empty
// rather than as a zero when no job is waiting, so the panel draws nothing.
func TestTheStatusOmitsTheJobCountWhenNoneWait(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	fields := running.statusForAScreen()

	value, sent := fields[contract.StatusFieldJobs]
	if !sent {
		t.Error("the status does not carry the job count at all, so a screen keeps whatever count it drew last")
	}
	if value != "" {
		t.Errorf("the status carries the job count %q while no job waits, want it empty", value)
	}
}
