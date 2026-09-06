package loop_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	workingcontext "github.com/JaredTate/nerdgenie/internal/context"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// seedTask writes one task's first checkpoint straight into the log, so a test
// can start a fresh task over a log that already holds earlier work. The
// checkpoint carries its own ask, the way a first checkpoint does, and one
// result whose summary is where the task stood.
func seedTask(t *testing.T, store *testkit.FakeStore, number int, status contract.RecordStatus, ask string, standing string) {
	t.Helper()
	seedTaskWithSituation(t, store, number, status, ask, standing, nil)
}

// seedTaskWithSituation is seedTask with the situation lines the harness writes
// as a task runs, which is where the files it changed are named.
func seedTaskWithSituation(t *testing.T, store *testkit.FakeStore, number int, status contract.RecordStatus, ask string, standing string, situation []string) {
	t.Helper()
	held := contract.Record{
		Header: contract.Header{
			Kind:          contract.RecordTask,
			ID:            strconv.Itoa(number),
			Status:        status,
			Origin:        "terminal",
			NoRoundBudget: true,
			NoTimeBudget:  true,
		},
		Goal: contract.Goal{Ask: ask},
		Work: contract.Work{Situation: situation, Results: []contract.ResultLine{{ID: "r1", Summary: standing}}},
	}
	body, err := json.Marshal(record.Checkpoint{Number: 1, Text: string(record.Print(held))})
	if err != nil {
		t.Fatalf("cannot write the seed checkpoint of task %d: %v", number, err)
	}
	if _, err := store.Append(t.Context(), contract.Event{
		TaskID: strconv.Itoa(number),
		Kind:   contract.EventCheckpoint,
		Body:   body,
	}); err != nil {
		t.Fatalf("cannot append the seed checkpoint of task %d to the log: %v", number, err)
	}
}

// firstRequestText is the whole of the first request the model was given,
// which is where a test reads what rode in front of the model on a task's very
// first call.
func firstRequestText(t *testing.T, built *harness) string {
	t.Helper()
	requests := built.model.Requests()
	if len(requests) == 0 {
		t.Fatal("the model was never called, so there is no request to read the recent-work block out of")
	}
	return testkit.WholeRequestText(requests[0])
}

// TestRecentWorkNamesTheFinishedTasksNewestFirstAndNotTheRunningOne is the
// promise that the model can answer "where are we" itself: a task that starts
// over a log of finished work carries a recent-work block naming the last three
// finished tasks, newest first, and leaves the task that is still running out.
func TestRecentWorkNamesTheFinishedTasksNewestFirstAndNotTheRunningOne(t *testing.T) {
	built := newHarness(t, []testkit.Step{answerStep("Everything so far is done; ask me anything.")})
	seedTask(t, built.store, 1, contract.StatusDone, "Draft the anniversary blog piece.", "saved to blog/anniversary.md")
	seedTask(t, built.store, 2, contract.StatusDone, "Post the anniversary tweet.", "posted, 236 characters")
	seedTask(t, built.store, 3, contract.StatusDone, "Update the changelog for the release.", "changelog updated")
	seedTask(t, built.store, 4, contract.StatusRunning, "Refactor the parser package.", "half done")

	built.ask(t, "where are we?")

	whole := firstRequestText(t, built)
	if !strings.Contains(whole, "Recent work") {
		t.Fatalf("the request carries no recent-work block:\n%s", whole)
	}
	for _, want := range []string{
		"task 3: Update the changelog for the release. — changelog updated",
		"task 2: Post the anniversary tweet. — posted, 236 characters",
		"task 1: Draft the anniversary blog piece. — saved to blog/anniversary.md",
	} {
		if !strings.Contains(whole, want) {
			t.Errorf("the recent-work block is missing %q:\n%s", want, whole)
		}
	}

	// Newest first: task 3 above task 2 above task 1.
	three, two, one := strings.Index(whole, "task 3:"), strings.Index(whole, "task 2:"), strings.Index(whole, "task 1:")
	if !(three < two && two < one) {
		t.Errorf("the finished tasks are not newest first: task 3 at %d, task 2 at %d, task 1 at %d", three, two, one)
	}

	// The task that is still running is not finished work, so it is left out.
	if strings.Contains(whole, "Refactor the parser package") || strings.Contains(whole, "task 4:") {
		t.Errorf("the running task was named in the recent-work block:\n%s", whole)
	}
}

// TestRecentWorkShowsAtMostThreeFinishedTasks proves the cap: a long history of
// finished work cannot push the record down the prompt, so only the three most
// recent are named.
func TestRecentWorkShowsAtMostThreeFinishedTasks(t *testing.T) {
	built := newHarness(t, []testkit.Step{answerStep("Ready.")})
	for number := 1; number <= 5; number++ {
		seedTask(t, built.store, number, contract.StatusDone,
			"Finished task "+strconv.Itoa(number)+".", "done "+strconv.Itoa(number))
	}

	built.ask(t, "status")

	whole := firstRequestText(t, built)
	for _, want := range []string{"task 5:", "task 4:", "task 3:"} {
		if !strings.Contains(whole, want) {
			t.Errorf("the three most recent finished tasks were not all named, missing %q:\n%s", want, whole)
		}
	}
	for _, gone := range []string{"task 2:", "task 1:"} {
		if strings.Contains(whole, gone) {
			t.Errorf("a task past the three most recent was named: %q:\n%s", gone, whole)
		}
	}
}

// TestNoFinishedTasksAddsNoRecentWork proves a log with nothing finished pays
// nothing for the block: a task still running is not finished work, and a task
// starting over it carries no recent-work block at all.
func TestNoFinishedTasksAddsNoRecentWork(t *testing.T) {
	built := newHarness(t, []testkit.Step{answerStep("Hello.")})
	seedTask(t, built.store, 1, contract.StatusRunning, "A task that is still running.", "in progress")
	seedTask(t, built.store, 2, contract.StatusWaiting, "A task waiting on an answer.", "asked a question")

	built.ask(t, "hi")

	whole := firstRequestText(t, built)
	if strings.Contains(whole, "Recent work") {
		t.Errorf("a log with no finished tasks still wrote a recent-work block:\n%s", whole)
	}
}

// TestRecentWorkSkipsCheckpointsItCannotRead proves a checkpoint the log cannot
// make sense of never stops the block being built: a checkpoint whose body is
// not JSON, one whose text does not read as a record, and one written under a
// job's key are all passed over, and the one finished task that reads back is
// still named.
func TestRecentWorkSkipsCheckpointsItCannotRead(t *testing.T) {
	built := newHarness(t, []testkit.Step{answerStep("Ready.")})
	seedTask(t, built.store, 1, contract.StatusDone, "A task that finished cleanly.", "all done")

	// A checkpoint whose body is not JSON, under a task's number.
	if _, err := built.store.Append(t.Context(), contract.Event{
		TaskID: "2", Kind: contract.EventCheckpoint, Body: []byte("this is not json"),
	}); err != nil {
		t.Fatalf("cannot append the unreadable-body checkpoint: %v", err)
	}
	// A checkpoint whose text does not read as a record.
	notARecord, err := json.Marshal(record.Checkpoint{Number: 1, Text: "this is not a record"})
	if err != nil {
		t.Fatalf("cannot write the not-a-record checkpoint: %v", err)
	}
	if _, err := built.store.Append(t.Context(), contract.Event{
		TaskID: "3", Kind: contract.EventCheckpoint, Body: notARecord,
	}); err != nil {
		t.Fatalf("cannot append the not-a-record checkpoint: %v", err)
	}
	// A checkpoint under a job's key, which begins with a letter and so is not a
	// task at all.
	if _, err := built.store.Append(t.Context(), contract.Event{
		TaskID: "j9", Kind: contract.EventCheckpoint, Body: []byte("whatever"),
	}); err != nil {
		t.Fatalf("cannot append the job-key checkpoint: %v", err)
	}

	built.ask(t, "where are we?")

	whole := firstRequestText(t, built)
	if !strings.Contains(whole, "task 1: A task that finished cleanly. — all done") {
		t.Errorf("the one readable finished task was not named:\n%s", whole)
	}
}

// TestRecentWorkStandingIsDoneWhenAFinishedTaskHasNoResult proves the standing
// falls back to the plain word "done" when a task closed on the user's own word
// rather than a tool result, so a line is still written and it never runs off the
// end of an empty result list.
func TestRecentWorkStandingIsDoneWhenAFinishedTaskHasNoResult(t *testing.T) {
	built := newHarness(t, []testkit.Step{answerStep("Ready.")})
	held := contract.Record{
		Header: contract.Header{Kind: contract.RecordTask, ID: "1", Status: contract.StatusDone, Origin: "terminal", NoRoundBudget: true, NoTimeBudget: true},
		Goal:   contract.Goal{Ask: "Confirm the release went out."},
	}
	body, err := json.Marshal(record.Checkpoint{Number: 1, Text: string(record.Print(held))})
	if err != nil {
		t.Fatalf("cannot write the no-result checkpoint: %v", err)
	}
	if _, err := built.store.Append(t.Context(), contract.Event{TaskID: "1", Kind: contract.EventCheckpoint, Body: body}); err != nil {
		t.Fatalf("cannot append the no-result checkpoint: %v", err)
	}

	built.ask(t, "where are we?")

	whole := firstRequestText(t, built)
	if !strings.Contains(whole, "task 1: Confirm the release went out. — done") {
		t.Errorf("a finished task with no result did not fall back to the standing \"done\":\n%s", whole)
	}
}

// TestRecentWorkRidesThroughTheRealLoop proves the wiring end to end: a task run
// to done leaves a real, many-checkpoint record in the log, and the next task
// starting after it carries that finished task in its recent-work block, ask and
// all, read back through the record package.
func TestRecentWorkRidesThroughTheRealLoop(t *testing.T) {
	firstAsk := "post the anniversary tweet"
	built := newHarness(t, []testkit.Step{
		callStep("I will draft the post.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"doneWhen":[{"text":"the post is drafted","done":true,"resultId":"r1"}]}`)),
		answerStep("Drafted. What changed: one draft. What I checked: the notes. What is left: nothing."),
		// The second task's one call, whose request must carry the first task.
		answerStep("The first task is done."),
	}, scriptedTool("read", "the notes name the account"))

	first := built.ask(t, firstAsk)
	if first.Status != contract.StatusDone {
		t.Fatalf("the first task ended %q, want done: %s", first.Status, first.Report)
	}

	built.ask(t, "what did you just do?")

	whole := firstRequestText2(t, built)
	if !strings.Contains(whole, "Recent work") {
		t.Fatalf("the second task carries no recent-work block:\n%s", whole)
	}
	if !strings.Contains(whole, "task "+first.TaskID+":") || !strings.Contains(whole, firstAsk) {
		t.Errorf("the second task's recent-work block does not name the finished first task %q:\n%s", first.TaskID, whole)
	}
}

// firstRequestText2 is the whole of the request the model was given on the
// second task's first call, which is the request after the first task's own
// calls in the shared model's record of them.
func firstRequestText2(t *testing.T, built *harness) string {
	t.Helper()
	requests := built.model.Requests()
	// The first task made two calls, so the second task's first call is the
	// third request the shared model saw.
	if len(requests) < 3 {
		t.Fatalf("the model saw %d requests, want at least three so the second task's call is among them", len(requests))
	}
	return testkit.WholeRequestText(requests[2])
}

// TestBuildInputCarriesRecentWork proves the loop's BuildInput has the field the
// wiring fills, so the working-context builder is handed the recent work.
func TestBuildInputCarriesRecentWork(t *testing.T) {
	var input loop.BuildInput
	input.RecentWork = []workingcontext.RecentTask{{Number: 4, Ask: "a", Standing: "b"}}
	if len(input.RecentWork) != 1 || input.RecentWork[0].Number != 4 {
		t.Fatal("BuildInput.RecentWork does not hold the recent tasks handed to it")
	}
}

// TestRecentWorkCarriesTheFilesATaskChangedAndWhereItStood is the fix for the
// live game build's second failure. The task that built the game ended, the
// person typed a follow-up, and the next task was told "task 1: # Tater Tots
// Tetris Build a complete, polished..." and nothing else, so it went looking
// for the game under ~/Code. The line carries the files the task changed and
// where the model said the work stood, so the next task knows where it is.
func TestRecentWorkCarriesTheFilesATaskChangedAndWhereItStood(t *testing.T) {
	built := newHarness(t, []testkit.Step{answerStep("The game is in the Desktop folder.")})
	seedTaskWithSituation(t, built.store, 1, contract.StatusDone, "Build Tater Tots Tetris in ~/Desktop/Tater Tots Tetrisv1.", "shell: tests: all 62 passing", []string{
		"files changed in this task: package.json, engine.js, index.html, src/main.js and 4 more",
		"last command: node --test test/*.test.js, it worked",
		"tests: all 62 passing",
		"where the work stands: the UI is written and the tests pass",
	})

	built.ask(t, "the start game button wont start game")

	shown := firstRequestText(t, built)
	for _, words := range []string{"task 1: Build Tater Tots Tetris", "files changed in this task: package.json, engine.js, index.html, src/main.js", "where the work stands: the UI is written"} {
		if !strings.Contains(shown, words) {
			t.Errorf("the recent-work block never told the model %q, and the request reads:\n%s", words, shown)
		}
	}
	if strings.Contains(shown, "last command:") {
		t.Error("the recent-work block carries the last command, which is not worth a line once the task has ended")
	}
}

// TestRecentWorkNamesATaskThePersonSetAside holds that a stopped task the
// person cleared away is still named, with the word stopped, because its work
// is on the disk whatever the record says.
func TestRecentWorkNamesATaskThePersonSetAside(t *testing.T) {
	built := newHarness(t, []testkit.Step{answerStep("Noted.")})
	seedTask(t, built.store, 1, contract.StatusStopped, "build the game", "shell: 62 tests passing")

	built.ask(t, "what were we doing?")

	if shown := firstRequestText(t, built); !strings.Contains(shown, "task 1 (stopped): build the game") {
		t.Errorf("the stopped task is not named with its status, and the request reads:\n%s", shown)
	}
}

// TestATaskPickedUpAgainIsNotListedAsRecentWork holds that a task the message
// carries on is the running task and not a recent one, so the model does not
// read its own task as work that ended.
func TestATaskPickedUpAgainIsNotListedAsRecentWork(t *testing.T) {
	built := newHarness(t, []testkit.Step{answerStep("Carrying on.")})
	seedTask(t, built.store, 1, contract.StatusStopped, "build the game", "shell: 62 tests passing")

	task := built.task("the start game button wont start game")
	task.ResumeID = "1"
	if _, err := built.loop.Run(t.Context(), task); err != nil {
		t.Fatalf("picking task 1 up again failed: %v", err)
	}

	if shown := firstRequestText(t, built); strings.Contains(shown, "task 1 (stopped)") {
		t.Errorf("the task being picked up is listed as recent work, and the request reads:\n%s", shown)
	}
}
