package loop_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// brokenStore is an event log that refuses one of the three things the loop asks
// of it, so that every failure path out of the log is walked.
type brokenStore struct {
	*testkit.FakeStore
	refuseAppend bool
	refuseByKind bool
}

// Append refuses when the test said it should.
func (store *brokenStore) Append(ctx context.Context, event contract.Event) (int64, error) {
	if store.refuseAppend {
		return 0, errors.New("this log is full, so free some space and try again")
	}
	return store.FakeStore.Append(ctx, event)
}

// ByKind refuses when the test said it should.
func (store *brokenStore) ByKind(ctx context.Context, kind contract.EventKind) ([]contract.Event, error) {
	if store.refuseByKind {
		return nil, errors.New("this log cannot be read right now, so check the file")
	}
	return store.FakeStore.ByKind(ctx, kind)
}

// brokenChannel is a channel that will not send, which is what a screen that has
// gone away looks like.
type brokenChannel struct {
	*testkit.FakeChannel
}

// Send always refuses.
func (channel *brokenChannel) Send(_ context.Context, _ string) error {
	return errors.New("this channel has gone away, so nothing can be sent through it")
}

// brokenBuilder is a working-context builder that cannot build.
type brokenBuilder struct{}

// Build always refuses.
func (builder brokenBuilder) Build(_ context.Context, _ loop.BuildInput) (contract.Request, error) {
	return contract.Request{}, errors.New("the working context is longer than the model can hold, so pin fewer results")
}

// registryWithAGhost tells the model about a tool that is not really there,
// which is what a registry looks like when a tool has been taken away.
type registryWithAGhost struct {
	contract.ToolRegistry
}

// Specs adds the ghost to what the model is told.
func (registry *registryWithAGhost) Specs() []contract.ToolSpec {
	return append(registry.ToolRegistry.Specs(), contract.ToolSpec{
		Name: "ghost", Description: "A tool the model is told about that is not really there at all.",
	})
}

// spillingTool returns more than the cap allows and names the file holding the
// rest.
type spillingTool struct{}

// Spec is what the model is told about the spilling tool.
func (tool spillingTool) Spec() contract.ToolSpec {
	return contract.ToolSpec{Name: "read", Description: "A tool whose result was too long, so the rest went to a file."}
}

// Run returns a short result and the file holding the rest.
func (tool spillingTool) Run(_ context.Context, _ json.RawMessage) (contract.ToolOutput, error) {
	return contract.ToolOutput{Text: "the first part of the notes", SpillPath: "/tmp/spill/r1.txt"}, nil
}

// TestALogThatWillNotTakeAnEventStopsTheTask proves the loop says so rather than
// carrying on with a task nothing is written down about.
func TestALogThatWillNotTakeAnEventStopsTheTask(t *testing.T) {
	built := newHarness(t, []testkit.Step{answerStep("Two plus two is four.")})
	options := built.options()
	options.Store = &brokenStore{FakeStore: built.store, refuseAppend: true}
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop over a broken log: %v", err)
	}

	if _, err := made.Run(t.Context(), built.task("what is two plus two")); err == nil {
		t.Error("the loop ran a task it could write nothing about")
	}
}

// TestALogThatCannotBeReadStopsTheTasksCommand proves the same for the listing.
func TestALogThatCannotBeReadStopsTheTasksCommand(t *testing.T) {
	built := newHarness(t, nil)
	options := built.options()
	options.Store = &brokenStore{FakeStore: built.store, refuseByKind: true}
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop over a broken log: %v", err)
	}

	if _, err := made.TasksCommand().Run(t.Context(), "", contract.CommandContext{}); err == nil {
		t.Error("the tasks command listed tasks out of a log it could not read")
	}
}

// TestAChannelThatWillNotSendStopsTheTask proves a reply that cannot be
// delivered is an error and not a silence.
func TestAChannelThatWillNotSendStopsTheTask(t *testing.T) {
	built := newHarness(t, []testkit.Step{answerStep("Two plus two is four.")})
	task := built.task("what is two plus two")
	task.Channel = &brokenChannel{FakeChannel: built.channel}

	if _, err := built.loop.Run(t.Context(), task); err == nil {
		t.Error("the loop finished a task whose answer never reached the user")
	}
}

// TestAWorkingContextThatCannotBeBuiltFailsTheTask proves the loop reports a
// prompt it could not build rather than calling the model with nothing.
func TestAWorkingContextThatCannotBeBuiltFailsTheTask(t *testing.T) {
	built := newHarness(t, nil)
	options := built.options()
	options.Context = brokenBuilder{}
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop over a broken context builder: %v", err)
	}

	outcome, err := made.Run(t.Context(), built.task("post the tweet"))
	if err != nil {
		t.Fatalf("a prompt that will not build is a failed task, not an error out of the loop: %v", err)
	}
	if outcome.Status != contract.StatusFailed {
		t.Errorf("the task ended %q, want failed", outcome.Status)
	}
	if len(built.model.Requests()) != 0 {
		t.Error("the model was called with a working context that could not be built")
	}
}

// TestAToolTheModelWasToldAboutButIsNotThere proves the loop hands the model an
// error it can act on rather than crashing.
func TestAToolTheModelWasToldAboutButIsNotThere(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will use the ghost.", callFor("c1", "ghost", `{}`)),
		answerStep("That tool is not there. What now?"),
	})
	options := built.options()
	options.Tools = &registryWithAGhost{ToolRegistry: built.tools}
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop over a registry with a ghost in it: %v", err)
	}

	if _, err := made.Run(t.Context(), built.task("use the ghost")); err != nil {
		t.Fatalf("the loop could not run the task: %v", err)
	}
	if !strings.Contains(requestsJoined(built.model.Requests()), "there is no tool called") {
		t.Error("the model was never told that the tool it asked for is not there")
	}
}

// TestAResultThatSpilledToAFileNamesTheFile proves a result over the cap tells
// the model where the rest of it went.
func TestAResultThatSpilledToAFileNamesTheFile(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("Shall I read the rest?"),
	}, spillingTool{})

	built.ask(t, "read the notes")

	if !strings.Contains(requestsJoined(built.model.Requests()), "/tmp/spill/r1.txt") {
		t.Error("the model was never told which file holds the rest of the result")
	}
}

// TestARecordWriteTheRulesRefuseComesBackAsAnErrorTheModelCanFix proves the
// record's rules reach the model in words it can act on.
func TestARecordWriteTheRulesRefuseComesBackAsAnErrorTheModelCanFix(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes and write the record.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"doneWhen":[{"text":"the notes are read","done":true,"resultId":"r99"}]}`),
			taskCall("c2t", `{"decision":{"text":"go on"}}`),
			taskCall("c4t", `{"nothing":"known"}`)),
		answerStep("The record would not take that. What should I write?"),
	}, scriptedTool("read", "the notes"))

	built.ask(t, "read the notes")

	whole := requestsJoined(built.model.Requests())
	for _, wanted := range []string{"refused that change", "carries no reason", "says nothing this record can hold"} {
		if !strings.Contains(whole, wanted) {
			t.Errorf("the model was never told %q", wanted)
		}
	}
}

// TestAReplyCutOffAtTheOutputCapIsToldSoAndTriedAgain is the tenth nightly
// run's frontend task: the model wrote the whole game file in one write call,
// the reply hit the output cap of 8192 tokens three rounds running, the harness
// read each cut-off call as unreadable arguments and offered the three options,
// and on the fourth round the model took the first, "answer the user", with
// "Now the main game logic. Let me write it carefully", and the task closed
// done with no game file written. A reply cut off at the cap is neither an
// answer nor a call: the model is told it was cut off, that none of it was
// kept, and how to write a long file in parts.
func TestAReplyCutOffAtTheOutputCapIsToldSoAndTriedAgain(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		{
			Text:      "I will write the whole file now.",
			ToolCalls: []contract.ToolCall{callFor("c2", "write", `{"path":"main.js","content":"const a = 1;"}`)},
			Finish:    contract.FinishLength,
			Usage:     contract.Usage{OutputTokens: 8192},
		},
		answerStep("The notes are read."),
	}, scriptedTool("read", "the notes"))

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusDone {
		t.Errorf("the task ended %q, want done once the model was sent back and answered", outcome.Status)
	}
	whole := requestsJoined(built.model.Requests())
	if !strings.Contains(whole, "cut off at the output cap after 8192 tokens") || !strings.Contains(whole, "in parts") {
		t.Error("the model was never told its reply was cut off at the output cap, or how to write a long file in parts")
	}
	if strings.Contains(whole, "const a = 1;") {
		t.Error("the cut-off reply was kept in the working context, and none of it is worth keeping")
	}
}

// TestAnAnswerWithPlanStepsStillOpenAndNoDoneListIsSentBackToWork is the same
// task's last round: its done list had been refused, its plan of five steps had
// two marked done, and the reply "Let me write it carefully" closed the task as
// done, its answer taken as the whole done list. A plan with steps still open
// says the work is not over, and the model is sent back to it.
func TestAnAnswerWithPlanStepsStillOpenAndNoDoneListIsSentBackToWork(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"plan":["read the notes","write the summary"]}`)),
		answerStep("Now the summary, the largest part. Let me write it carefully."),
		answerStep("step 1 done: r1, step 2 done: r1\nThe notes are read and the summary is written."),
	}, scriptedTool("read", "the notes"))

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusDone {
		t.Errorf("the task ended %q, want done once every plan step was marked", outcome.Status)
	}
	whole := requestsJoined(built.model.Requests())
	if !strings.Contains(whole, "steps 1 and 2 are not marked done") {
		t.Error("the model was never told that its plan still had steps not marked done")
	}
	for _, step := range built.held(t, outcome.TaskID).Work.Plan {
		if !step.Done {
			t.Errorf("plan step %d is still open on a task that ended done", step.Number)
		}
	}
}

// TestATaskWithNoChannelNameStillNamesItsOrigin proves the record's header says
// where the ask came in when the message itself does not.
func TestATaskWithNoChannelNameStillNamesItsOrigin(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("Shall I go on?"),
	}, scriptedTool("read", "the notes"))

	outcome, err := built.loop.Run(t.Context(), loop.Task{
		Message: contract.Inbound{ID: "m1", Text: "read the notes"},
		Channel: built.channel,
	})
	if err != nil {
		t.Fatalf("the loop could not run the task: %v", err)
	}
	if held := built.held(t, outcome.TaskID); held.Header.Origin != "terminal" {
		t.Errorf("the record says the ask came in on %q, want the channel's own name", held.Header.Origin)
	}
}

// TestADoneLineWhoseCommandWillNotRunSendsTheModelBack proves a sandbox that
// cannot run the command is a line that is not proven.
func TestADoneLineWhoseCommandWillNotRunSendsTheModelBack(t *testing.T) {
	built := newHarness(t, closingScript("the tests pass: `nothing scripted here`"), scriptedTool("read", "the notes"))

	outcome := built.ask(t, "make the tests pass")

	if outcome.Status == contract.StatusDone {
		t.Error("the task closed on a command the sandbox could not run at all")
	}
	if !strings.Contains(requestsJoined(built.model.Requests()), "it could not be run") {
		t.Error("the model was never told the command could not be run")
	}
}

// TestADoneLineIsNotCheckedWhenThereIsNoSandbox proves a machine with no fence
// leans on the result behind the line instead of refusing to close.
func TestADoneLineIsNotCheckedWhenThereIsNoSandbox(t *testing.T) {
	built := newHarness(t, closingScript("the tests pass: `go test ./...`"), scriptedTool("read", "the notes"))
	options := built.options()
	options.Sandbox = nil
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop with no sandbox: %v", err)
	}

	outcome, err := made.Run(t.Context(), built.task("make the tests pass"))
	if err != nil {
		t.Fatalf("the loop could not run the task: %v", err)
	}
	if outcome.Status != contract.StatusDone {
		t.Errorf("the task ended %q, and with no sandbox a done line stands on the result behind it", outcome.Status)
	}
}

// TestAJobStoreThatCannotBeAskedSaysSo proves the loop reports rather than
// guessing when the jobs cannot be read.
func TestAJobStoreThatCannotBeAskedSaysSo(t *testing.T) {
	built := newHarness(t, nil)
	options := built.options()
	options.Jobs = brokenJobs{Job: built.jobs}
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop over a broken job store: %v", err)
	}

	if _, err := made.RunNextJobTask(t.Context(), built.channel); err == nil {
		t.Error("the loop asked a broken job store and carried on as though nothing was wrong")
	}
}

// TestWithNoJobsAtAllNothingIsDue proves a build with no job store simply has no
// jobs rather than failing.
func TestWithNoJobsAtAllNothingIsDue(t *testing.T) {
	built := newHarness(t, nil)
	options := built.options()
	options.Jobs = nil
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop with no job store: %v", err)
	}

	ran, err := made.RunNextJobTask(t.Context(), built.channel)
	if err != nil || ran {
		t.Errorf("asking for work with no job store returned %v and %v, want no work and no error", ran, err)
	}
}

// brokenJobs is a job store that cannot say what is due.
type brokenJobs struct {
	contract.Job
}

// NextTask always refuses.
func (jobs brokenJobs) NextTask(_ context.Context, _ time.Time) (contract.TaskToRun, bool, error) {
	return contract.TaskToRun{}, false, errors.New("this job store cannot be read right now, so check the database file")
}

// TestAThirdReplyCutOffInARowIsKeptAsItStands bounds the retry: a reply that
// is a long answer to the user, cut at the cap, is not a file to write in
// parts, and a model told twice to write less and cut off a third time is
// heard as it stands, the way every cut-off reply was before 6 September,
// rather than sent round again until the stall meter stops it.
func TestAThirdReplyCutOffInARowIsKeptAsItStands(t *testing.T) {
	cut := testkit.Step{Text: "The notes say, at great length, that", Finish: contract.FinishLength, Usage: contract.Usage{OutputTokens: 8192}}
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		cut, cut, cut,
	}, scriptedTool("read", "the notes"))

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusDone {
		t.Errorf("the task ended %q, want done on the third cut-off reply, kept as it stands", outcome.Status)
	}
	if requests := built.model.Requests(); len(requests) != 4 {
		t.Errorf("the model was called %d times, want 4: the read, two cut-offs sent back, and the third kept", len(requests))
	}
}

// TestAllMarkedAndStillWorkingIsToldToEndTheTask is the fresh Tetris build's
// yeti task: every done line and plan step marked by round 174, and the model
// went on into the next task's work for twenty more rounds inside this one.
// Once everything is marked, a reply that still asks for tools gets one line
// after a few rounds: answer to end the task; more work is the next task's.
func TestAllMarkedAndStillWorkingIsToldToEndTheTask(t *testing.T) {
	steps := []testkit.Step{
		callStep("I will read the notes.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"doneWhen":[{"text":"the notes are read","done":true,"resultId":"r1"}],"plan":["read the notes"]}`)),
		callStep("step 1 done: r1. Now I will look further.", callFor("c2", "read", `{"path":"more.md"}`)),
	}
	for at := 1; at <= loop.RoundsAfterAllMarked+1; at++ {
		steps = append(steps, callStep("I will look at yet another file.", callFor(fmt.Sprintf("x%d", at), "read", fmt.Sprintf(`{"path":"extra%d.md"}`, at))))
	}
	steps = append(steps, answerStep("It is done. What changed: nothing. What I checked: the notes. What is left: nothing."))
	answers := []string{"the notes", "more"}
	for at := 1; at <= loop.RoundsAfterAllMarked+1; at++ {
		answers = append(answers, fmt.Sprintf("extra %d", at))
	}
	built := newHarness(t, steps, scriptedTool("read", answers...))

	outcome := built.ask(t, "read the notes")

	if outcome.Status != contract.StatusDone {
		t.Errorf("the task ended %q, want done", outcome.Status)
	}
	if _, count := requestsCarrying(built, loop.TheAllMarkedLine); count == 0 {
		t.Error("the model was never told that everything was marked and to answer to end the task")
	}
}
