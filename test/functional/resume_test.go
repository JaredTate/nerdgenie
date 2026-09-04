// The whole-program tests for picking a task up again through the socket: an
// answer to the model's question carries on the task that asked it, the word
// "continue" carries on a task that stopped at its budget, and anything else
// starts a fresh task. Before this, "coeus serve" never filled the loop's
// ResumeID, so every message began a new task and a question could never be
// answered.
package functional

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// waitForRecordLineSaying reads envelopes until a status carries a record line
// with the words in it, and returns that line, which is how these tests ask
// which task number a message was written down under.
func (screen *attachedScreen) waitForRecordLineSaying(t *testing.T, words string, bound time.Duration) string {
	t.Helper()
	found := screen.waitUntil(t, bound, "a record line saying "+words, func(envelope contract.SocketEnvelope) bool {
		return envelope.Type == contract.SocketStatus &&
			strings.Contains(strings.ToLower(envelope.Fields[contract.StatusFieldRecordLine]), strings.ToLower(words))
	})
	return found.Fields[contract.StatusFieldRecordLine]
}

// theAskThatIsAnsweredWithAQuestion is what the person types first, and what the
// resumed task's record still holds when the model is called again.
const theAskThatIsAnsweredWithAQuestion = "what does the note say?"

// theAnswerThePersonGives is the reply to the model's question. It is not one of
// the words for carrying on, because a waiting task is picked up by any message
// at all.
const theAnswerThePersonGives = "the top one"

// aTaskThatAsksAQuestionAndIsAnswered is one task in four calls: read the file
// and open the record, ask the person which cupboard they meant, point the done
// line at the result once they have answered, and report.
func aTaskThatAsksAQuestionAndIsAnswered(work string) testkit.Script {
	path := filepath.Join(work, theFileTheToolReads)
	return testkit.Script{Name: "local", ContextLength: 32768, Steps: []testkit.Step{
		{
			Expect: []string{theAskThatIsAnsweredWithAQuestion},
			Text:   "I will open the file.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{
				{ID: "call-read", Name: contract.ToolRead, Input: json.RawMessage(`{"path":"` + path + `"}`)},
				{ID: "call-task", Name: contract.ToolTask, Input: json.RawMessage(
					`{"why":"the user wants what the file says","doneWhen":["the file has been read"]}`)},
			},
			Usage: contract.Usage{InputTokens: 400, OutputTokens: 20},
		},
		{
			Text:   "The note names two cupboards. Which cupboard did you mean?",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 500, OutputTokens: 12},
		},
		{
			// Both of these prove the task was picked up rather than started
			// again: the person's answer is in front of the model, and so is
			// the record of the task that asked the question.
			Expect: []string{theAnswerThePersonGives, theAskThatIsAnsweredWithAQuestion},
			Text:   "I will point the done line at the result.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{{ID: "call-task-again", Name: contract.ToolTask, Input: json.RawMessage(
				`{"doneWhen":[{"text":"the file has been read","done":true,"resultId":"r1"}]}`)}},
			Usage: contract.Usage{InputTokens: 600, OutputTokens: 20},
		},
		{
			Text:   "What it says: the kettle is on the third shelf. What is left: nothing.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 700, OutputTokens: 20},
		},
	}}
}

func TestAnAnswerToTheModelsQuestionCarriesTheSameTaskOn(t *testing.T) {
	agent := startTheAgentWorkingIn(t, aTaskThatAsksAQuestionAndIsAnswered)
	writeTheNote(t, agent.work)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAskThatIsAnsweredWithAQuestion})
	screen.waitForReplySaying(t, "which cupboard did you mean", 60*time.Second)

	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAnswerThePersonGives})

	ended := screen.waitForRecordLineSaying(t, " done", 60*time.Second)
	if !strings.HasPrefix(ended, "task 1 ") {
		t.Errorf("the answer to the model's question ended %q, and it should carry on task 1 rather than start a task of its own", ended)
	}
}

// theAskThatRunsOutOfBudget is what the person types for the task that is
// stopped by its one-round budget.
const theAskThatRunsOutOfBudget = "read the note and tell me what is in the cupboard"

// aTaskStoppedAtItsBudget is one task that spends its one round on a tool call,
// is told its budget is spent, and is then picked up again by the word
// "continue", which spends the picked-up task's ending call the same way.
func aTaskStoppedAtItsBudget(work string) testkit.Script {
	path := filepath.Join(work, theFileTheToolReads)
	return testkit.Script{Name: "local", ContextLength: 32768, Steps: []testkit.Step{
		{
			Expect: []string{theAskThatRunsOutOfBudget},
			Text:   "I will open the file.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{
				{ID: "call-read", Name: contract.ToolRead, Input: json.RawMessage(`{"path":"` + path + `"}`)},
				{ID: "call-task", Name: contract.ToolTask, Input: json.RawMessage(
					`{"why":"the user wants what the file says","doneWhen":["the file has been read"]}`)},
			},
			Usage: contract.Usage{InputTokens: 400, OutputTokens: 20},
		},
		{
			Text:   "What I did: I read the note. What is left: saying what is in the cupboard.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 500, OutputTokens: 20},
		},
		{
			Expect: []string{theAskThatRunsOutOfBudget},
			Text:   "What I did: I read the note. What is left: the same thing.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 600, OutputTokens: 20},
		},
	}}
}

func TestTheWordContinueCarriesOnTheTaskThatStoppedAtItsBudget(t *testing.T) {
	agent := startTheAgentWorkingIn(t, aTaskStoppedAtItsBudget,
		func(home contract.Home, _ string) {
			addSettingToTheHome(t, home, "[caps]\nrounds_per_task = 1")
		})
	writeTheNote(t, agent.work)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAskThatRunsOutOfBudget})
	screen.waitForReplySaying(t, "i stopped this task", 60*time.Second)

	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "continue"})

	carriedOn := screen.waitForRecordLineSaying(t, "continue", 60*time.Second)
	if !strings.HasPrefix(carriedOn, "task 1 ") {
		t.Errorf("the word continue after a stop was written down as %q, and it should carry on task 1 rather than start a task of its own",
			carriedOn)
	}
}

// theAskAfterTheFinishedTask is about something else, so it is a task of its own
// and not a message that carries the finished one on.
const theAskAfterTheFinishedTask = "and what is on the shelf below?"

// aFinishedTaskAndThenSomethingElse is one whole task followed by a plain answer
// to an unrelated message, which is the task after it.
func aFinishedTaskAndThenSomethingElse(work string) testkit.Script {
	script := aTaskThatReadsAFile(work)
	script.Steps = append(script.Steps, testkit.Step{
		Expect: []string{theAskAfterTheFinishedTask},
		Text:   "Nothing is on the shelf below.",
		Finish: contract.FinishEnd,
		Usage:  contract.Usage{InputTokens: 700, OutputTokens: 8},
	})
	return script
}

func TestAMessageAfterAFinishedTaskStartsATaskOfItsOwn(t *testing.T) {
	agent := startTheAgentWorkingIn(t, aFinishedTaskAndThenSomethingElse)
	writeTheNote(t, agent.work)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "what does the note say?"})
	screen.waitForReplySaying(t, "third shelf", 60*time.Second)

	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAskAfterTheFinishedTask})

	next := screen.waitForRecordLineSaying(t, "shelf below", 60*time.Second)
	if !strings.HasPrefix(next, "task 2 ") {
		t.Errorf("the message after a finished task was written down as %q, and a message about something else is a task of its own",
			next)
	}
}
