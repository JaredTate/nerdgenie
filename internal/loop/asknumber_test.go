package loop_test

import (
	"encoding/json"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/testkit"
)

// TestTheAskIsWrittenIntoTheLogUnderTheTasksNumber is what a restart needs: the
// message that started a task is the one event that says which screen the task
// belongs to, and it was written under no task at all, so nothing reading the
// log afterwards could tell whose task was waiting. The task takes its number
// before the ask is written, so the ask can carry it.
func TestTheAskIsWrittenIntoTheLogUnderTheTasksNumber(t *testing.T) {
	built := newHarness(t, scriptThatPins(), scriptedTool("read", theBrandRule))

	outcome := built.ask(t, "write the post")

	messages := built.eventsOfKind(t, contract.EventMessage)
	if len(messages) == 0 {
		t.Fatal("the log holds no message event at all, and the ask is written into it before the first call")
	}
	first := messages[0]
	if first.TaskID != outcome.TaskID {
		t.Errorf("the ask is written under task %q and the task is number %q", first.TaskID, outcome.TaskID)
	}
	written := contract.Inbound{}
	if err := json.Unmarshal(first.Body, &written); err != nil {
		t.Fatalf("the ask's event does not read as a message: %v", err)
	}
	if written.Text != "write the post" || written.Channel != "terminal" || written.Sender != "the user" {
		t.Errorf("the ask's event reads %+v, and it should carry the words, the channel and the sender as they came in", written)
	}
}

// TestANumberAPlainAnswerTookIsNotHandedOutAgainAfterARestart is the other
// half of writing the ask under a number: a task that answered with no tools
// made no record and so no checkpoint, and the numbering counted checkpoints
// alone, so the next start would have handed its number to a task with a
// record, and the log would then hold two tasks' words under one number.
func TestANumberAPlainAnswerTookIsNotHandedOutAgainAfterARestart(t *testing.T) {
	built := newHarness(t, []testkit.Step{answerStep("Hello.")}, scriptedTool("read", theBrandRule))
	plain := built.ask(t, "say hello")
	if plain.TaskID != "" {
		t.Fatalf("a plain answer made the record %q, and it should make none", plain.TaskID)
	}

	// A restart is a new loop over the same log, which counts the numbers the
	// log has handed out before it hands out the next.
	restarted, err := loop.New(built.optionsOver(testkit.NewFakeModel(
		testkit.Script{Name: "test", ContextLength: 24000, Steps: scriptThatPins()})))
	if err != nil {
		t.Fatalf("cannot build the loop again over the same log: %v", err)
	}
	outcome, err := restarted.Run(t.Context(), built.task("write the post"))
	if err != nil {
		t.Fatalf("the restarted loop could not run the task: %v", err)
	}

	if outcome.TaskID != "2" {
		t.Errorf("the task after the restart is number %q, want 2, because the plain answer before it took number 1", outcome.TaskID)
	}
}
