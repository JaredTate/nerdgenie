package loop_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// deliveringTool hands the loop a message from the user while a tool call is in
// flight, which is how a mid-turn message is proved without a second thread.
type deliveringTool struct {
	name    string
	holder  *harness
	says    string
	stops   bool
	handed  bool
	answers []string
	used    int
}

// Spec is what the model is told about the delivering tool.
func (tool *deliveringTool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name:        tool.name,
		Description: "A tool the test uses to hand the loop a message while a call is in flight.",
		Classes:     []contract.PermissionClass{contract.ClassRead},
	}
}

// Run hands the message over the first time it is called and then answers.
func (tool *deliveringTool) Run(_ context.Context, _ json.RawMessage) (contract.ToolOutput, error) {
	if !tool.handed {
		tool.handed = true
		if tool.stops {
			tool.holder.loop.Stop()
		} else if err := tool.holder.loop.Deliver(contract.Inbound{ID: "m2", Sender: "the user", Text: tool.says}); err != nil {
			return contract.ToolOutput{}, err
		}
	}
	said := "nothing came back"
	if tool.used < len(tool.answers) {
		said = tool.answers[tool.used]
	}
	tool.used++
	return contract.ToolOutput{Text: said}, nil
}

// midTurnHarness builds a loop whose first tool hands the loop a message.
func midTurnHarness(t *testing.T, steps []testkit.Step, says string) (*harness, *deliveringTool) {
	t.Helper()
	tool := &deliveringTool{name: "read", says: says, answers: []string{"the first file", "the second file", "the third file"}}
	built := newHarness(t, steps, tool)
	tool.holder = built
	return built, tool
}

// TestAMidTurnCorrectionLandsInTheRecordWordForWord proves rule 1 of design
// section 3: the harness copies the user's words into the record itself, so the
// correction is there whatever the model does with it next.
func TestAMidTurnCorrectionLandsInTheRecordWordForWord(t *testing.T) {
	said := "no, lead with the date not the features"
	built, _ := midTurnHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		callStep("The user has corrected me.", callFor("c2", "read", `{"path":"brand.md"}`)),
		answerStep("I have taken that in. Shall I go on?"),
	}, said)

	outcome := built.ask(t, "post the anniversary tweet")

	held := built.held(t, outcome.TaskID)
	if len(held.Rules.Corrections) != 1 || held.Rules.Corrections[0].Text != said {
		t.Fatalf("the record holds %v, want the user's words exactly as they were sent", held.Rules.Corrections)
	}
	if !strings.Contains(requestsJoined(built.model.Requests()), said) {
		t.Error("the model was never shown what the user said")
	}
}

// TestAMidTurnNewRequestIsAnsweredAndTheTaskCarriesOn proves the second half of
// rule 1: a message that is not a stop steers the work rather than ending it.
func TestAMidTurnNewRequestIsAnsweredAndTheTaskCarriesOn(t *testing.T) {
	built, _ := midTurnHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		callStep("I will also check the brand file, as asked.", callFor("c2", "read", `{"path":"brand.md"}`)),
		answerStep("Both are read. Shall I draft the post?"),
	}, "also check the brand file while you are there")

	outcome := built.ask(t, "post the anniversary tweet")

	if outcome.Status != contract.StatusWaiting {
		t.Errorf("the task ended %q, and a new request in the middle does not end a task", outcome.Status)
	}
	if built.model.StepsLeft() != 0 {
		t.Error("the task did not carry on after the user's message")
	}
}

// TestAMidTurnStopStopsTheTask proves the third half of rule 1: a short fixed
// list of words means stop, and nothing else does.
func TestAMidTurnStopStopsTheTask(t *testing.T) {
	built, tool := midTurnHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("The review says what to keep."),
	}, "stop")

	outcome := built.ask(t, "post the anniversary tweet")

	if outcome.Status != contract.StatusStopped {
		t.Errorf("the task ended %q, want stopped, because the user said stop", outcome.Status)
	}
	if tool.used != 1 {
		t.Errorf("the tools ran %d times, and nothing runs after the user says stop", tool.used)
	}
	if !sentSomethingLike(built.channel.Sent(), "I stopped this task") {
		t.Errorf("the user was sent %v, want a message saying the task stopped", built.channel.Sent())
	}
}

// TestOnlyTheWordsThatMeanStopStopTheTask proves a message that merely holds the
// word "stop" is a correction and not a stop.
func TestOnlyTheWordsThatMeanStopStopTheTask(t *testing.T) {
	built, _ := midTurnHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("Understood. Shall I go on?"),
	}, "do not stop until the post is up")

	outcome := built.ask(t, "post the anniversary tweet")

	if outcome.Status != contract.StatusWaiting {
		t.Errorf("the task ended %q, and only the words that mean stop stop a task", outcome.Status)
	}
}

// TestTheQueueOfMidTurnMessagesHasACap proves every buffer in the loop is
// bounded.
func TestTheQueueOfMidTurnMessagesHasACap(t *testing.T) {
	built := newHarness(t, nil)
	caps := contract.DefaultConfig().Caps

	for at := range caps.QueuedMessages {
		if err := built.loop.Deliver(contract.Inbound{ID: "m", Text: "one more"}); err != nil {
			t.Fatalf("the queue refused message %d, and it holds %d", at+1, caps.QueuedMessages)
		}
	}
	if err := built.loop.Deliver(contract.Inbound{ID: "m", Text: "one too many"}); err == nil {
		t.Error("the queue took one message past its cap, and every buffer here has a cap")
	}
}

// TestAStopCommandStopsTheRunningTask proves the stop command reaches the task
// that is running.
func TestAStopCommandStopsTheRunningTask(t *testing.T) {
	tool := &deliveringTool{name: "read", stops: true, answers: []string{"the notes"}}
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("The review says what to keep."),
	}, tool)
	tool.holder = built

	outcome := built.ask(t, "post the anniversary tweet")

	if outcome.Status != contract.StatusStopped {
		t.Errorf("the task ended %q, want stopped, because the stop command was used", outcome.Status)
	}
	if tool.used != 1 {
		t.Errorf("the tools ran %d times, and nothing runs after a stop", tool.used)
	}
}

// TestTheStopCommandSaysWhatItStopped proves the command the orchestrator
// registers answers the user.
func TestTheStopCommandSaysWhatItStopped(t *testing.T) {
	built := newHarness(t, nil)
	command := built.loop.StopCommand()

	if command.Name != "stop" {
		t.Errorf("the command is called %q, and the orchestrator registers it as \"stop\"", command.Name)
	}
	said, err := command.Run(t.Context(), "", contract.CommandContext{Channel: built.channel})
	if err != nil {
		t.Fatalf("the stop command failed with nothing running: %v", err)
	}
	if !strings.Contains(said, "Nothing is running") {
		t.Errorf("the stop command said %q with nothing running, and it should say so", said)
	}
}
