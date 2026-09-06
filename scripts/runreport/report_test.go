package main

import (
	"encoding/json"
	"github.com/JaredTate/nerdgenie/internal/log"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aRecordedRun writes a small run into a fake log, the way the loop does: a
// checkpoint after every model call, the calls of each round between them, a
// cost line on every checkpoint, and a stall the rewind wrote as a failure.
func aRecordedRun(t *testing.T) *testkit.FakeStore {
	t.Helper()
	store := testkit.NewFakeStore()
	at := time.Date(2026, 9, 5, 22, 0, 0, 0, time.UTC)
	append := func(kind contract.EventKind, body any) {
		t.Helper()
		written, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("cannot write the %s: %v", kind, err)
		}
		if _, err := store.Append(t.Context(), contract.Event{TaskID: "7", Kind: kind, Body: written, Occurred: at}); err != nil {
			t.Fatalf("cannot append the %s: %v", kind, err)
		}
		at = at.Add(16 * time.Second)
	}
	checkpoint := func(number int, status contract.RecordStatus, cost contract.CostLine, failures []contract.Failure) {
		held := contract.Record{
			Header:  contract.Header{Kind: contract.RecordTask, ID: "7", Status: status, Origin: "terminal", NoRoundBudget: true, NoTimeBudget: true, Cost: cost},
			Goal:    contract.Goal{Ask: "make the tests pass"},
			Lessons: contract.Lessons{Failures: failures},
		}
		append(contract.EventCheckpoint, record.Checkpoint{Number: number, Text: string(record.Print(held))})
	}
	call := func(name string, input string) {
		append(contract.EventToolCall, contract.ToolCall{ID: name + input[:4], Name: name, Input: json.RawMessage(input)})
	}
	append(contract.EventMessage, contract.Inbound{Text: "make the tests pass"})
	checkpoint(1, contract.StatusRunning, contract.CostLine{InputTokens: 10000, CachedInputTokens: 0, OutputTokens: 300}, nil)
	call("task", `{"operation":"plan","plan":["fix it"]}`)
	checkpoint(2, contract.StatusRunning, contract.CostLine{InputTokens: 12000, CachedInputTokens: 10000, OutputTokens: 100}, nil)
	call("write", `{"path":"engine.js","content":"x"}`)
	call("shell", `{"command":"node --test test/"}`)
	checkpoint(3, contract.StatusRunning, contract.CostLine{InputTokens: 14000, CachedInputTokens: 12000, OutputTokens: 500}, nil)
	call("shell", `{"command":"cd game && node --test test/ 2>&1 | tail"}`)
	checkpoint(4, contract.StatusRunning, contract.CostLine{InputTokens: 15000, CachedInputTokens: 14000, OutputTokens: 100},
		[]contract.Failure{{ID: "F1", Text: "stalled: asked for read engine.js over and over, so the conversation was cleared", Cause: "nothing changed"}})
	call("read", `{"path":"engine.js"}`)
	checkpoint(5, contract.StatusDone, contract.CostLine{InputTokens: 16000, CachedInputTokens: 15000, OutputTokens: 200},
		[]contract.Failure{{ID: "F1", Text: "stalled: asked for read engine.js over and over, so the conversation was cleared", Cause: "nothing changed"},
			{ID: "F2", Text: "2 failing of 10 after changing engine.js", Cause: "the change"}})
	return store
}

// TestTheNumbersOfARunAreReadOffTheLog holds every number the report prints,
// against a run written the way the loop writes one.
func TestTheNumbersOfARunAreReadOffTheLog(t *testing.T) {
	numbers, err := measure(t.Context(), aRecordedRun(t), "7")
	if err != nil {
		t.Fatalf("cannot measure the run: %v", err)
	}
	if numbers.rounds != 5 || numbers.status != contract.StatusDone {
		t.Errorf("the run reads %d rounds ending %q, want 5 rounds ending done", numbers.rounds, numbers.status)
	}
	if numbers.repliesBatched != 1 {
		t.Errorf("replies with more than one call read %d, want 1: the write and the test run in one reply", numbers.repliesBatched)
	}
	if numbers.recordOnlyRounds != 1 || numbers.testOnlyRounds != 1 {
		t.Errorf("record-only rounds read %d and test-only rounds %d, want 1 and 1", numbers.recordOnlyRounds, numbers.testOnlyRounds)
	}
	if numbers.tokensIn != 67000 || numbers.cachedIn != 51000 || numbers.tokensOut != 1200 {
		t.Errorf("the tokens read %d in, %d cached, %d out, want 67000, 51000 and 1200", numbers.tokensIn, numbers.cachedIn, numbers.tokensOut)
	}
	if numbers.rewinds != 1 || numbers.failures != 2 {
		t.Errorf("rewinds read %d and failures %d, want 1 and 2, as the newest checkpoint holds them", numbers.rewinds, numbers.failures)
	}
	if numbers.callsByTool["shell"] != 2 || numbers.callsByTool["task"] != 1 || numbers.callsByTool["write"] != 1 || numbers.callsByTool["read"] != 1 {
		t.Errorf("the calls by tool read %v", numbers.callsByTool)
	}
	if numbers.minutes <= 0 || numbers.minutes > 5 {
		t.Errorf("the run reads %.1f minutes, want the span from the ask to the last checkpoint, a few minutes", numbers.minutes)
	}
	text := numbers.String()
	for _, words := range []string{"task 7: done after 5 rounds", "replies with more than one call: 1", "only wrote the record: 1", "only ran the tests: 1", "67.0k in, 51.0k of them cached (76%), 1.2k out", "rewinds: 1; failures on the record: 2"} {
		if !strings.Contains(text, words) {
			t.Errorf("the report does not say %q:\n%s", words, text)
		}
	}
}

// TestTheNewestTaskIsMeasuredWhenNoneIsNamed keeps the command usable without
// a number: the newest task is the one a person just watched.
func TestTheNewestTaskIsMeasuredWhenNoneIsNamed(t *testing.T) {
	numbers, err := measure(t.Context(), aRecordedRun(t), "")
	if err != nil || numbers.taskID != "7" {
		t.Errorf("measuring the newest task gave %+v (%v), want task 7", numbers.taskID, err)
	}
	if _, err := measure(t.Context(), aRecordedRun(t), "42"); err == nil {
		t.Error("a task the log does not hold was measured")
	}
}

// TestTheCommandLineIsReadAndRefused holds the usage line.
func TestTheCommandLineIsReadAndRefused(t *testing.T) {
	problems := &strings.Builder{}
	if code := run([]string{"--wrong"}, &strings.Builder{}, problems); code != 2 || !strings.Contains(problems.String(), "usage") {
		t.Errorf("a wrong flag gave %d with %q, want 2 and the usage line", code, problems.String())
	}
	if code := run(nil, &strings.Builder{}, &strings.Builder{}); code != 2 {
		t.Errorf("no log gave %d, want 2", code)
	}
	if code := run([]string{"--log", t.TempDir() + "/missing/nerdgenie.db"}, &strings.Builder{}, &strings.Builder{}); code == 0 {
		t.Error("a log that cannot be opened gave 0")
	}
}

// TestTheCommandReadsARealLogAndPrintsTheNumbers runs the command the way a
// person does, over a real log file: the numbers come out, and the exit code
// is zero.
func TestTheCommandReadsARealLogAndPrintsTheNumbers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nerdgenie.db")
	real, err := log.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("cannot open a log at %s: %v", path, err)
	}
	held := contract.Record{
		Header: contract.Header{Kind: contract.RecordTask, ID: "3", Status: contract.StatusDone, Origin: "terminal", NoRoundBudget: true, NoTimeBudget: true,
			Cost: contract.CostLine{InputTokens: 5000, CachedInputTokens: 4000, OutputTokens: 100}},
		Goal: contract.Goal{Ask: "say hello"},
	}
	written, err := json.Marshal(record.Checkpoint{Number: 1, Text: string(record.Print(held))})
	if err != nil {
		t.Fatalf("cannot write the checkpoint: %v", err)
	}
	if _, err := real.Append(t.Context(), contract.Event{TaskID: "3", Kind: contract.EventCheckpoint, Body: written, Occurred: time.Now()}); err != nil {
		t.Fatalf("cannot append the checkpoint: %v", err)
	}
	if err := real.Close(); err != nil {
		t.Fatalf("cannot close the log: %v", err)
	}

	output, problems := &strings.Builder{}, &strings.Builder{}
	if code := run([]string{"--log", path}, output, problems); code != 0 {
		t.Fatalf("the command gave %d with %q, want 0", code, problems.String())
	}
	if !strings.Contains(output.String(), "task 3: done after 1 rounds") || !strings.Contains(output.String(), "5.0k in, 4.0k of them cached (80%)") {
		t.Errorf("the command printed:\n%s", output.String())
	}
	if code := run([]string{"--log", path, "--task", "9"}, &strings.Builder{}, problems); code == 0 {
		t.Error("a task the log does not hold gave 0")
	}
}

// TestMarksMadeFromTheFirstLineAreCounted is the number the nightly set needs
// for idea one's second half: whether the model marks a step or a line on
// its first line, which the harness keeps as the situation's "where the work
// stands" line on every checkpoint, rather than through the task tool.
func TestMarksMadeFromTheFirstLineAreCounted(t *testing.T) {
	store := testkit.NewFakeStore()
	at := time.Date(2026, 9, 6, 1, 0, 0, 0, time.UTC)
	for number, orient := range []string{"Nothing is read yet.", "Step 1 done: r1. Next: the brand file.", "Line 1 done: r1 and step 2 done: r2.", "The post is written."} {
		held := contract.Record{
			Header: contract.Header{Kind: contract.RecordTask, ID: "9", Status: contract.StatusRunning, Origin: "terminal", NoRoundBudget: true, NoTimeBudget: true},
			Goal:   contract.Goal{Ask: "write the post"},
			Work:   contract.Work{Situation: []string{"where the work stands: " + orient}},
		}
		written, err := json.Marshal(record.Checkpoint{Number: number + 1, Text: string(record.Print(held))})
		if err != nil {
			t.Fatalf("cannot write the checkpoint: %v", err)
		}
		if _, err := store.Append(t.Context(), contract.Event{TaskID: "9", Kind: contract.EventCheckpoint, Body: written, Occurred: at}); err != nil {
			t.Fatalf("cannot append the checkpoint: %v", err)
		}
		at = at.Add(10 * time.Second)
	}

	numbers, err := measure(t.Context(), store, "9")
	if err != nil {
		t.Fatalf("cannot measure the run: %v", err)
	}
	if numbers.firstLineMarks != 3 {
		t.Errorf("marks made from the first line read %d, want 3: one on the second checkpoint and two on the third", numbers.firstLineMarks)
	}
	if !strings.Contains(numbers.String(), "marks made from the first line: 3") {
		t.Errorf("the report does not say the marks from the first line:\n%s", numbers.String())
	}
}

// TestARangeOfTasksIsMeasuredTogether is the nightly set's job-shaped ask:
// a long ask becomes a job whose tasks each get a record of their own, so the
// number for the ask is the sum over every task from the ask's own task on.
func TestARangeOfTasksIsMeasuredTogether(t *testing.T) {
	store := aRecordedRun(t)
	at := time.Date(2026, 9, 6, 2, 0, 0, 0, time.UTC)
	for _, id := range []string{"8", "9"} {
		held := contract.Record{
			Header: contract.Header{Kind: contract.RecordTask, ID: id, Status: contract.StatusDone, Origin: "terminal", NoRoundBudget: true, NoTimeBudget: true,
				Cost: contract.CostLine{InputTokens: 1000, CachedInputTokens: 500, OutputTokens: 500}},
			Goal: contract.Goal{Ask: "one task of the job"},
		}
		written, err := json.Marshal(record.Checkpoint{Number: 1, Text: string(record.Print(held))})
		if err != nil {
			t.Fatalf("cannot write the checkpoint: %v", err)
		}
		if _, err := store.Append(t.Context(), contract.Event{TaskID: id, Kind: contract.EventCheckpoint, Body: written, Occurred: at}); err != nil {
			t.Fatalf("cannot append the checkpoint: %v", err)
		}
		at = at.Add(time.Minute)
	}

	numbers, err := measureFrom(t.Context(), store, "7")
	if err != nil {
		t.Fatalf("cannot measure the tasks from 7: %v", err)
	}
	if numbers.rounds != 7 || numbers.tokensIn != 69000 || numbers.tokensOut != 2200 || numbers.tasks != 3 {
		t.Errorf("the range from task 7 reads %d rounds, %d in, %d out over %d tasks, want 7, 69000, 2200 and 3", numbers.rounds, numbers.tokensIn, numbers.tokensOut, numbers.tasks)
	}
	if numbers.status != contract.StatusDone || numbers.taskID != "7 to 9" {
		t.Errorf("the range reads as %q ending %q, want \"7 to 9\" ending done, the newest task's", numbers.taskID, numbers.status)
	}
	if code := run([]string{"--log", "nowhere.db", "--from", "x"}, &strings.Builder{}, &strings.Builder{}); code != 2 {
		t.Errorf("a from that is not a number gave %d, want 2", code)
	}
}
