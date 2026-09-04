package loop_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestTheSituationNamesTheFilesTheTaskChanged proves the harness reads the files
// changed out of the file-change events and out of the writes it saw, and says
// how many more there are when there are too many to name.
func TestTheSituationNamesTheFilesTheTaskChanged(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will write the draft.", callFor("c1", "write", `{"path":"drafts/one.md","content":"one"}`)),
		answerStep("Written. Shall I go on?"),
	}, scriptedTool("write", "one file written"))
	for at := range 9 {
		body, err := json.Marshal(contract.FileChangeBody{Path: fmt.Sprintf("notes/file-%d.md", at), Existed: true})
		if err != nil {
			t.Fatalf("cannot write a file-change event: %v", err)
		}
		if _, err := built.store.Append(t.Context(), contract.Event{
			TaskID: "1", Kind: contract.EventFileChange, Body: body,
		}); err != nil {
			t.Fatalf("cannot put a file-change event in the log: %v", err)
		}
	}

	outcome := built.ask(t, "write the draft")

	held := built.held(t, outcome.TaskID)
	if !situationHolds(held, "drafts/one.md") {
		t.Errorf("the situation reads %v, and it must name the file the write tool touched", held.Work.Situation)
	}
	if !situationHolds(held, "and 2 more") {
		t.Errorf("the situation reads %v, and it must say how many files it did not name", held.Work.Situation)
	}
}

// TestTheMemoryHintRidesAtTheEndOfThePrompt proves the three lines from memory
// reach the model, and that a build with no memory at all simply has none.
func TestTheMemoryHintRidesAtTheEndOfThePrompt(t *testing.T) {
	built := newHarness(t, []testkit.Step{answerStep("Two plus two is four.")})
	if err := built.memory.Save(t.Context(), []contract.Fact{{
		ID: "f1", Text: "the user asks what is two plus two every morning", Source: "the user", Recorded: theStartOfTime,
	}}); err != nil {
		t.Fatalf("cannot put a fact in the fake memory: %v", err)
	}

	built.ask(t, "what is two plus two")

	if !strings.Contains(requestsJoined(built.model.Requests()), "every morning") {
		t.Error("the memory hint never reached the model")
	}
}

// TestALoopWithNoMemoryStillRuns proves the parts that are switched off when
// their dependency is missing.
func TestALoopWithNoMemoryStillRuns(t *testing.T) {
	built := newHarness(t, []testkit.Step{answerStep("Two plus two is four.")})
	options := built.options()
	options.Memory, options.Skills = nil, nil
	made, err := loop.New(options)
	if err != nil {
		t.Fatalf("cannot build a loop with no memory and no skills: %v", err)
	}

	outcome, err := made.Run(t.Context(), built.task("what is two plus two"))
	if err != nil {
		t.Fatalf("the loop could not run without a memory: %v", err)
	}
	if outcome.Status != contract.StatusDone {
		t.Errorf("the task ended %q, want done", outcome.Status)
	}
}

// TestAJobTaskWithNowhereToAnswerIsRefused proves the loop will not run work it
// could not report on, even when the job asked for it.
func TestAJobTaskWithNowhereToAnswerIsRefused(t *testing.T) {
	built := newHarness(t, nil)
	aJobOfTwoTasks(t, built)

	if _, err := built.loop.RunNextJobTask(t.Context(), nil); err == nil {
		t.Error("the loop ran a job's task with nowhere to send the report")
	}
}

// TestAFinishedTaskThatCannotBeReportedSaysSo proves a report that will not send
// is an error rather than a task that quietly looks finished.
func TestAFinishedTaskThatCannotBeReportedSaysSo(t *testing.T) {
	built := newHarness(t, closingScript("the notes are read"), scriptedTool("read", "the notes"))
	task := built.task("read the notes")
	task.Channel = &brokenChannel{FakeChannel: built.channel}

	if _, err := built.loop.Run(t.Context(), task); err == nil {
		t.Error("a finished task whose report never reached the user came back as a success")
	}
}

// TestAStoppedTaskThatCannotBeReportedSaysSo proves the same on the stop path.
func TestAStoppedTaskThatCannotBeReportedSaysSo(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the page.", callFor("c1", "browser_read", `{}`)),
		aReviewReply("Keep watching for the login page."),
	}, scriptedTool("browser_read", "This page is a login page and it wants a username."))
	task := built.task("post the tweet")
	task.Channel = &brokenChannel{FakeChannel: built.channel}

	if _, err := built.loop.Run(t.Context(), task); err == nil {
		t.Error("a stopped task whose report never reached the user came back as a success")
	}
}

// TestAJobReportThatCannotBeSentSaysSo proves the same on the job path.
func TestAJobReportThatCannotBeSentSaysSo(t *testing.T) {
	built := newHarness(t, twoTasksAndTheirReview(), scriptedTool("read", "the notes", "the notes"))
	aJobOfTwoTasks(t, built)

	if _, err := built.loop.RunNextJobTask(t.Context(), &brokenChannel{FakeChannel: built.channel}); err == nil {
		t.Error("a job report that never reached the user came back as a success")
	}
}

// TestAJobTaskThatEndsWaitingLeavesTheJobWhereItIs proves a task waiting on the
// user is not a finished task.
func TestAJobTaskThatEndsWaitingLeavesTheJobWhereItIs(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("Which account should I post from?"),
	}, scriptedTool("read", "the notes"))
	jobID := aJobOfTwoTasks(t, built)

	if _, err := built.loop.RunNextJobTask(t.Context(), built.channel); err != nil {
		t.Fatalf("the loop could not run the job's task: %v", err)
	}

	for _, task := range built.jobs.Tasks(jobID) {
		if task.Done {
			t.Errorf("the task %s is marked done, and it is waiting on the user", task.TaskID)
		}
	}
}

// TestAJobsOwnCheckpointsAreNotListedAsTasks proves the tasks command tells a
// task's events from a job's, which share one log.
func TestAJobsOwnCheckpointsAreNotListedAsTasks(t *testing.T) {
	built := newHarness(t, closingScript("the notes are read"), scriptedTool("read", "the notes"))
	if _, err := built.store.Append(t.Context(), contract.Event{
		TaskID: contract.RecordLogKey(contract.RecordJob, "4"), Kind: contract.EventCheckpoint,
		Body: json.RawMessage(`{"number":1,"text":""}`),
	}); err != nil {
		t.Fatalf("cannot put a job checkpoint in the log: %v", err)
	}
	built.ask(t, "read the notes that are long enough to need cutting on the listing line, and then some")

	listed, err := runTasksCommand(t, built, "")
	if err != nil {
		t.Fatalf("the tasks command failed: %v", err)
	}
	if strings.Contains(listed, "task j4") {
		t.Errorf("the listing reads %q, and a job is not a task", listed)
	}
	if !strings.Contains(listed, "...") {
		t.Errorf("the listing reads %q, and a long ask is cut to fit the line", listed)
	}
}

// TestADoneLineWithAWebAddressIsNotReadAsAFile proves the path check does not
// mistake an address for a file on this machine.
func TestADoneLineWithAWebAddressIsNotReadAsAFile(t *testing.T) {
	built := newHarness(t, closingScript("the post is up at https://x.com/digibyte/1"),
		scriptedTool("read", "the notes"))

	outcome := built.ask(t, "post the tweet")

	if outcome.Status != contract.StatusDone {
		t.Errorf("the task ended %q, and a web address in a done line is not a file to look for", outcome.Status)
	}
}

// TestAReviewWithFewerThanFourLinesKeepsTheLastOne proves the review reads what
// the model actually wrote rather than insisting on four lines.
func TestAReviewWithFewerThanFourLinesKeepsTheLastOne(t *testing.T) {
	steps := append(closingScript("the notes are read"), answerStep("It went fine.\nKeep the notes short."))
	built, _ := midTurnHarness(t, steps, "no, keep it short")

	built.ask(t, "read the notes")

	facts := factsIn(t, built)
	if len(facts) != 1 || facts[0].Text != "Keep the notes short." {
		t.Errorf("memory holds %v, want the last line of a review that wrote fewer than four", facts)
	}
}
