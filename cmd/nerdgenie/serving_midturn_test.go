package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/clock"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aModelThatCallsBack runs one function the first time a task calls it, so
// that a test can look at the agent while a job's task is running rather than
// before or after. Everything else is the fake model underneath.
type aModelThatCallsBack struct {
	contract.Model
	once   sync.Once
	during func()
}

// Send runs the function on the first call and then answers from the script.
func (model *aModelThatCallsBack) Send(ctx context.Context, request contract.Request, onDelta func(delta string)) (contract.Reply, error) {
	model.once.Do(model.during)
	return model.Model.Send(ctx, request, onDelta)
}

// theReviewAnswer answers the four review questions, which a task that had a
// correction and a job that has run every task are both asked.
const theReviewAnswer = "1. One task was to run.\n2. It ran.\n3. There was no difference.\n4. Keep one task per piece of work."

// aJobWithOneTaskDue puts one job with one task that may start now on the
// agent's job list, which is what the job driver finds on its next pass.
func aJobWithOneTaskDue(t *testing.T, running *agent, text string) {
	t.Helper()
	jobID, err := running.jobs.Create(context.Background(), contract.NewJob{
		Ask: "run the campaign", Name: "the campaign", Why: "the user wants the campaign run",
	})
	if err != nil {
		t.Fatalf("making the job failed: %v", err)
	}
	if _, err := running.jobs.AddTask(context.Background(), contract.NewTask{JobID: jobID, Text: text}); err != nil {
		t.Fatalf("adding the job's task failed: %v", err)
	}
}

// TestTheJobDriverHoldsTheLoopWhileAJobsTaskRuns pins that a job's task holds
// the loop the way a person's task does, for as long as it runs: a person's
// message arriving meanwhile finds it held and is delivered to the task, rather
// than taking the loop for a task of its own that waits behind the job.
func TestTheJobDriverHoldsTheLoopWhileAJobsTaskRuns(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()
	aScreenAttachedTo(t, running)
	aJobWithOneTaskDue(t, running, "post the tweet")

	takenMeanwhile := false
	running.watched.use(&aModelThatCallsBack{
		Model: testkit.NewFakeModel(testkit.Script{Name: "local-coder", ContextLength: 262144, Steps: []testkit.Step{
			{Text: "The tweet is posted.", Finish: contract.FinishEnd},
			{Text: theReviewAnswer, Finish: contract.FinishEnd},
		}}),
		// A person's message takes the loop this way, and must find it held.
		during: func() {
			if running.takeTheLoop() {
				takenMeanwhile = true
				running.freeTheLoop()
			}
		},
	})

	started, err := running.runWhatIsDue(context.Background())
	if err != nil {
		t.Fatalf("running the due task failed: %v", err)
	}
	if !started {
		t.Fatal("the job's task was not started, and it was due")
	}
	if takenMeanwhile {
		t.Error("the loop could be taken while the job's task was running, so a message typed meanwhile would start a task of its own instead of reaching the task")
	}
	if !running.takeTheLoop() {
		t.Error("the loop is still held after the job's task ended, so nothing could start")
	}
	running.freeTheLoop()
}

// TestAMessageTypedDuringAJobsTaskIsHandedToThatTask pins the other half: the
// message is delivered to the running task, which writes it into its own
// record and puts it in front of the model on the next call, and no task of its
// own is started for it.
func TestAMessageTypedDuringAJobsTaskIsHandedToThatTask(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()
	aScreenAttachedTo(t, running)
	aJobWithOneTaskDue(t, running, "post the tweet")
	note := filepath.Join(running.settings.SandboxRoots[0], "notes.md")
	if err := os.WriteFile(note, []byte("the anniversary is today\n"), contract.DataFileMode); err != nil {
		t.Fatalf("writing the note failed: %v", err)
	}

	// The message lands while the first call is in flight, so the task reads
	// it after the tool round that follows; the call after that must carry it.
	fake := testkit.NewFakeModel(testkit.Script{Name: "local-coder", ContextLength: 262144, Steps: []testkit.Step{
		{
			Text:   "I will read the notes.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{
				{ID: "call-read", Name: contract.ToolRead, Input: json.RawMessage(`{"path":` + quoted(note) + `}`)},
				{ID: "call-task", Name: contract.ToolTask, Input: json.RawMessage(
					`{"why":"the user wants the tweet posted","doneWhen":["the notes are read"]}`)},
			},
		},
		{
			Text:   "I will point the done line at the result.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{{ID: "call-task-proof", Name: contract.ToolTask, Input: json.RawMessage(
				`{"doneWhen":[{"text":"the notes are read","done":true,"resultId":"r1"}]}`)}},
		},
		{Text: "The tweet is posted. What is left: nothing.", Finish: contract.FinishEnd},
		{Text: theReviewAnswer, Finish: contract.FinishEnd},
		{Text: theReviewAnswer, Finish: contract.FinishEnd},
	}})
	message := contract.Inbound{Channel: contract.TerminalChannelName, Sender: contract.TerminalChannelName, Text: "use TypeScript for every file"}
	var handed error
	running.watched.use(&aModelThatCallsBack{Model: fake, during: func() {
		handed = running.startTask(context.Background(), newScreenTasks(), message)
	}})

	if _, err := running.runWhatIsDue(context.Background()); err != nil {
		t.Fatalf("running the due task failed: %v", err)
	}
	// A task wrongly started for the message runs after the job; this waits for
	// it, so that the log below holds everything that was going to be written.
	waitUntilNotBusy(t, running)

	if handed != nil {
		t.Errorf("handing the message to the running task failed: %v", handed)
	}
	requests := fake.Requests()
	if len(requests) < 2 || !strings.Contains(testkit.WholeRequestText(requests[1]), message.Text) {
		t.Error("the call after the tool round did not carry the person's words, so the message was not handed to the running task")
	}
	jobTask := tasksTheMessageWasWrittenUnder(t, running, "post the tweet")
	under := tasksTheMessageWasWrittenUnder(t, running, message.Text)
	if !slices.Equal(under, jobTask) {
		t.Errorf("the message was written into the log under tasks %v, want only the job's task %v: a task of its own was started for it", under, jobTask)
	}
}

// TestTheJobDriverLeavesADueTaskAloneWhileAPersonsTaskHoldsTheLoop pins that
// the driver asks the store for a task only once it holds the loop: a task
// taken from the store cannot be taken again until its hour runs out, so one
// taken beside a person's task would sit there, taken and never run.
func TestTheJobDriverLeavesADueTaskAloneWhileAPersonsTaskHoldsTheLoop(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()
	aJobWithOneTaskDue(t, running, "post the tweet")
	// A driver that wrongly runs the task would call the model; a script with
	// no steps fails that call at once rather than waiting on a server that is
	// not there.
	running.watched.use(testkit.NewFakeModel(testkit.Script{Name: "local-coder", ContextLength: 262144}))

	if !running.takeTheLoop() {
		t.Fatal("could not take the loop to stand in for a person's running task")
	}
	defer running.freeTheLoop()

	started, err := running.runWhatIsDue(context.Background())
	if err != nil {
		t.Fatalf("the driver's pass failed: %v", err)
	}
	if started {
		t.Error("a job's task was started beside a person's task, and the agent works on one task at a time")
	}
	if _, there, err := running.jobs.NextTask(context.Background(), clock.System().Now()); err != nil || !there {
		t.Errorf("the due task is no longer there to take (there=%v, err=%v), so the driver took it while a person's task held the loop", there, err)
	}
}

// tasksTheMessageWasWrittenUnder is the task numbers the log holds a message
// with these words under, in the order they were written.
func tasksTheMessageWasWrittenUnder(t *testing.T, running *agent, text string) []string {
	t.Helper()
	events, err := running.events.ByKind(context.Background(), contract.EventMessage)
	if err != nil {
		t.Fatalf("reading the messages out of the log failed: %v", err)
	}
	under := []string{}
	for _, event := range events {
		message := contract.Inbound{}
		if err := json.Unmarshal(event.Body, &message); err != nil || message.Text != text {
			continue
		}
		under = append(under, event.TaskID)
	}
	return under
}

// quoted writes a string the way JSON carries it.
func quoted(text string) string {
	written, _ := json.Marshal(text)
	return string(written)
}
