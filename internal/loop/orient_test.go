package loop_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/orientation"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestATaskStartsOrientedWithTheFolderAndThePorts: at the start of tasks the
// model listed the work folder and asked which ports were listening, a round
// each. The first request of a task carries both.
func TestATaskStartsOrientedWithTheFolderAndThePorts(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		answerStep("The folder is seen. What changed: nothing. What I checked: the folder. What is left: nothing."),
	})
	if err := os.MkdirAll(filepath.Join(built.workFolder, "src"), 0o755); err != nil {
		t.Fatalf("cannot make src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(built.workFolder, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("cannot write the readme: %v", err)
	}

	built.ask(t, "look around")

	first, count := requestsCarrying(built, orientation.TheHeading)
	if first != 0 || count != 1 {
		t.Fatalf("the heading rides in request %d, %d times; want the first request, once", first, count)
	}
	text := wholeRequestText(built.model.Requests()[0])
	for _, wanted := range []string{"in the working folder " + built.workFolder, "README.md  src/", "listening ports:"} {
		if !strings.Contains(text, wanted) {
			t.Errorf("the first request does not carry %q:\n%s", wanted, text)
		}
	}
	if strings.Contains(text, orientation.TheResultsHeading) {
		t.Errorf("a new task has no results to show, and the first request shows some:\n%s", text)
	}
	// The ask is the last thing the model reads before its first reply. With
	// the block after the ask, the fresh run of 6 September wrote no plan in
	// any task but the first, where the run before it had written one in every
	// task: the model answered the folder listing instead of the ask.
	if strings.Index(text, orientation.TheHeading) > strings.Index(text, "look around") {
		t.Errorf("the orientation comes after the ask, and the ask must come last:\n%s", text)
	}
}

// TestARewindOpensWithTheNewestResultsInFull: after the conversation is
// cleared the model used to re-read the results it had just seen by their
// ids, because the fresh window carried only their one-line summaries.
func TestARewindOpensWithTheNewestResultsInFull(t *testing.T) {
	reading := testkit.NewScriptedTool(contract.ToolSpec{
		Name: "read", Description: "A tool the test scripted, which answers with what the test gave it.",
	}, "the notes say the meeting is at noon", "the notes say the meeting is at noon", "the brand file says plain words")
	built := newHarness(t, []testkit.Step{
		sameReadAgain("c1"), sameReadAgain("c2"), sameReadAgain("c3"), sameReadAgain("c4"),
		callStep("I will read the brand file instead.", callFor("c5", "read", `{"path":"brand.md"}`)),
		answerStep("The notes and the brand file are read."),
	}, reading)

	built.ask(t, "read the notes")

	requests := built.model.Requests()
	if len(requests) < 5 {
		t.Fatalf("the model was called %d times, want at least 5", len(requests))
	}
	after := wholeRequestText(requests[4])
	if !strings.Contains(after, loop.TheRewindLine) {
		t.Fatalf("the fifth request is not the one after the clearing:\n%s", after)
	}
	if !strings.Contains(after, orientation.TheResultsHeading) || !strings.Contains(after, "the notes say the meeting is at noon") {
		t.Errorf("the request after the clearing does not carry the newest result in full:\n%s", after)
	}
}

// TestAPickedUpTaskOpensWithTheNewestResultsInFull: a task picked up after a
// stop, a question, or a restart starts a fresh window, and the results it
// made before are the first thing it needs.
func TestAPickedUpTaskOpensWithTheNewestResultsInFull(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("Which colour do you want?"),
		answerStep("Blue it is. What changed: nothing. What I checked: the notes. What is left: nothing."),
	}, scriptedTool("read", "the notes say blue or green"))

	first := built.ask(t, "read the notes and ask me")
	if first.Status == contract.StatusDone {
		t.Fatalf("the first sitting ended done, and it was meant to wait on a question")
	}
	before := len(built.model.Requests())

	outcome, err := built.loop.Run(t.Context(), loop.Task{
		Message:  contract.Inbound{ID: "m2", Sender: "the user", Text: "blue", Channel: "terminal"},
		Channel:  built.channel,
		ResumeID: first.TaskID,
	})
	if err != nil {
		t.Fatalf("the task could not be picked up: %v", err)
	}
	if outcome.Status != contract.StatusDone {
		t.Fatalf("the picked-up task ended %q, want done: %s", outcome.Status, outcome.Report)
	}
	text := wholeRequestText(built.model.Requests()[before])
	if !strings.Contains(text, orientation.TheResultsHeading) || !strings.Contains(text, "the notes say blue or green") {
		t.Errorf("the first request of the picked-up task does not carry the newest result in full:\n%s", text)
	}
}
