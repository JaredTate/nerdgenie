package main

import (
	"context"
	"strconv"
	"testing"
	"time"

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
	running.fillTheTask(fields, "7")

	if got, want := fields[contract.StatusFieldPlan], "[x] first step\n[ ] second step"; got != want {
		t.Errorf("the status carries the plan %q, want %q", got, want)
	}
}

// TestTheStatusCarriesTheRunningTasksAsk holds the other half of what the
// panel draws for a plain task: while a task runs, the status carries that
// task's ask from its record, folded onto one line the way the job's ask is,
// so the panel can head the task with the person's own words rather than a
// bare number.
func TestTheStatusCarriesTheRunningTasksAsk(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	ctx := context.Background()
	if _, err := record.New(ctx, running.events, record.Start{
		Kind: contract.RecordTask, ID: "7", Origin: contract.TerminalChannelName,
		Ask: "post the tweet\nabout the anniversary", RoundsLeft: 10, MinutesLeft: 10,
	}); err != nil {
		t.Fatalf("cannot start the record of task 7: %v", err)
	}

	fields := map[string]string{}
	running.fillTheTask(fields, "7")

	if got, want := fields[contract.StatusFieldTaskAsk], "post the tweet about the anniversary"; got != want {
		t.Errorf("the status carries the task's ask %q, want %q", got, want)
	}
}

// TestTheStatusOmitsThePlanAndTheAskWhenNoTaskRuns holds that the task's two
// panel fields are sent empty rather than left out while nothing runs, so a
// screen that drew a task a moment ago clears it rather than keeping the last
// one.
func TestTheStatusOmitsThePlanAndTheAskWhenNoTaskRuns(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	fields := running.statusForAScreen()

	for _, field := range []string{contract.StatusFieldPlan, contract.StatusFieldTaskAsk} {
		value, sent := fields[field]
		if !sent {
			t.Errorf("the status does not carry %s at all while nothing runs, so a screen keeps whatever task it drew last", field)
		}
		if value != "" {
			t.Errorf("the status carries %s = %q while no task runs", field, value)
		}
	}
}

// TestTheStatusCarriesNoPlanOrAskWhenTheRecordIsGone holds that a running
// number with no record behind it leaves both fields empty rather than failing,
// which is what the moment before a task's first checkpoint looks like.
func TestTheStatusCarriesNoPlanOrAskWhenTheRecordIsGone(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	fields := map[string]string{}
	running.fillTheTask(fields, "404")

	for _, field := range []string{contract.StatusFieldPlan, contract.StatusFieldTaskAsk} {
		if got := fields[field]; got != "" {
			t.Errorf("the status carries %s = %q for a task with no record, want it empty", field, got)
		}
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

// theSevenTaskFields are the fields fillTheTask writes for the running task,
// every one of which is sent empty when nothing runs.
var theSevenTaskFields = []string{
	contract.StatusFieldPlan, contract.StatusFieldTaskAsk,
	contract.StatusFieldSituation, contract.StatusFieldFailures, contract.StatusFieldCachedTokens,
	contract.StatusFieldRound, contract.StatusFieldTaskStarted,
}

// TestTheStatusCarriesTheRunningTasksSituationFailuresCacheRoundAndStart
// holds the five fields the second screen draws for a running task: the
// situation one fact per line, the failures one per line with the cause, the
// cached tokens of the last call, the round, and when the task began.
func TestTheStatusCarriesTheRunningTasksSituationFailuresCacheRoundAndStart(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	ctx := context.Background()
	keeper, err := record.New(ctx, running.events, record.Start{
		Kind: contract.RecordTask, ID: "7", Origin: contract.TerminalChannelName, Ask: "do the work", RoundsLeft: 10, MinutesLeft: 10,
	})
	if err != nil {
		t.Fatalf("cannot start the record of task 7: %v", err)
	}
	if err := keeper.SetSituation(ctx, []string{"tests: all 51 passing", "last command: npm test, exit 0"}); err != nil {
		t.Fatalf("cannot write the situation of task 7: %v", err)
	}
	for _, failure := range []record.NewFailure{
		{Text: "the build broke", Cause: "a missing import"},
		{Text: "the test\nhung", Cause: "a lock never let go"},
	} {
		if err := keeper.Apply(ctx, record.Update{Failure: &failure}); err != nil {
			t.Fatalf("cannot write a failure of task 7: %v", err)
		}
	}
	if err := keeper.SetCost(ctx, contract.CostLine{InputTokens: 900, CachedInputTokens: 600, OutputTokens: 100}); err != nil {
		t.Fatalf("cannot write the cost of task 7: %v", err)
	}
	logged, err := running.events.ByTask(ctx, "7")
	if err != nil || len(logged) == 0 {
		t.Fatalf("task 7 has no events in the log to read its start from: %v", err)
	}

	fields := map[string]string{}
	running.fillTheTask(fields, "7")

	for field, want := range map[string]string{
		contract.StatusFieldSituation:    "tests: all 51 passing\nlast command: npm test, exit 0",
		contract.StatusFieldFailures:     "F1 the build broke Cause: a missing import\nF2 the test hung Cause: a lock never let go",
		contract.StatusFieldCachedTokens: "600",
		contract.StatusFieldRound:        strconv.Itoa(keeper.LatestCheckpoint()),
		contract.StatusFieldTaskStarted:  logged[0].Occurred.UTC().Format(time.RFC3339),
	} {
		if fields[field] != want {
			t.Errorf("the status carries %s = %q, want %q", field, fields[field], want)
		}
	}
}

// TestTheStatusSendsTheFiveNewFieldsEmptyWhenNoTaskRuns holds that every one
// of the task's fields is sent empty rather than left out while nothing runs,
// and again when the running number has no record behind it, so a screen
// clears what it drew rather than keeping the last task's facts.
func TestTheStatusSendsTheFiveNewFieldsEmptyWhenNoTaskRuns(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	idle := running.statusForAScreen()
	for _, field := range theSevenTaskFields {
		value, sent := idle[field]
		if !sent {
			t.Errorf("the status does not carry %s at all while nothing runs, so a screen keeps whatever task it drew last", field)
		}
		if value != "" {
			t.Errorf("the status carries %s = %q while no task runs", field, value)
		}
	}

	gone := map[string]string{}
	running.fillTheTask(gone, "404")
	for _, field := range theSevenTaskFields {
		if got := gone[field]; got != "" {
			t.Errorf("the status carries %s = %q for a task with no record, want it empty", field, got)
		}
	}
}

// TestFailureLinesNameEachFailureWithItsCause holds the wire shape of the
// failures field: one failure per line, its label, what went wrong folded onto
// the line, and the cause after the word Cause.
func TestFailureLinesNameEachFailureWithItsCause(t *testing.T) {
	failures := []contract.Failure{
		{ID: "F1", Text: "the build broke", Cause: "a missing import"},
		{ID: "F2", Text: "the test\nhung", Cause: "a lock\nnever let go"},
	}

	if got, want := failureLines(failures), "F1 the build broke Cause: a missing import\nF2 the test hung Cause: a lock never let go"; got != want {
		t.Errorf("the failure lines are %q, want %q", got, want)
	}
	if got := failureLines(nil); got != "" {
		t.Errorf("no failures wrote %q, want nothing", got)
	}
}

// TestAScheduledJobIsNotCountedAsWaiting holds the count to what a person
// means by waiting work: a job made by the clock, such as the nightly
// self-check, is always in the running state and always has a next run, and
// the live screen said "1 job waiting" through a whole day's work because of
// it.
func TestAScheduledJobIsNotCountedAsWaiting(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	ctx := context.Background()
	if _, err := running.jobs.Create(ctx, contract.NewJob{Ask: "check yourself every night", TaskTemplate: "run the nightly self-check", Schedule: &contract.Schedule{Kind: contract.ScheduleCron, Cron: "0 4 * * *"}}); err != nil {
		t.Fatalf("cannot create a scheduled job: %v", err)
	}
	if _, err := running.jobs.Create(ctx, contract.NewJob{Ask: "water the plants"}); err != nil {
		t.Fatalf("cannot create a job to wait: %v", err)
	}

	fields := running.statusForAScreen()

	if got := fields[contract.StatusFieldJobs]; got != "1" {
		t.Errorf("the status counts %q waiting jobs, want 1: the scheduled job is the clock's, not waiting work", got)
	}
}
