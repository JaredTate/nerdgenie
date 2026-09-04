// The whole-program test for "/think": a person types it in the terminal to see
// how hard the model is thinking, types it again with a level, and the very next
// call the agent makes carries that level on the wire. It travels the same way
// every other slash command does, in on the socket and out through the router.
package functional

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theAskThatIsAnsweredWithoutTools is a question the scripted model answers in
// one call and with no tool, so that the test is about the level on the call
// and nothing else.
const theAskThatIsAnsweredWithoutTools = "what is two plus two"

// aScriptThatAnswersOneQuestion is the one step the agent needs to make one
// model call.
func aScriptThatAnswersOneQuestion() testkit.Script {
	return testkit.Script{Name: "local", ContextLength: 32768, Steps: []testkit.Step{{
		Expect: []string{theAskThatIsAnsweredWithoutTools},
		Text:   "Two plus two is four.",
		Finish: contract.FinishEnd,
		Usage:  contract.Usage{InputTokens: 400, OutputTokens: 12},
	}}}
}

func TestTheThinkCommandShowsTheLevelAndPutsTheOneItIsGivenOnTheNextCall(t *testing.T) {
	agent := startTheAgent(t, aScriptThatAnswersOneQuestion())
	screen := agent.attach(t)

	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "think"})
	listed := screen.waitForReplySaying(t, "local", 15*time.Second)
	for _, level := range contract.ThinkLevels() {
		if !strings.Contains(listed.Text, string(level)) {
			t.Errorf("/think leaves out the level %q:\n%s", level, listed.Text)
		}
	}

	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "think medium"})
	set := screen.waitForReplySaying(t, "medium", 15*time.Second)
	if !strings.Contains(set.Text, "local") {
		t.Errorf("/think medium does not name the model it set the level for:\n%s", set.Text)
	}

	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAskThatIsAnsweredWithoutTools + "?"})
	screen.waitForReplySaying(t, "four", 60*time.Second)

	asked := agent.model.Requests()
	if len(asked) == 0 {
		t.Fatal("the model was never called, so nothing carried the think level")
	}
	last := string(asked[len(asked)-1].Body)
	if !strings.Contains(last, `"reasoning_effort":"medium"`) {
		t.Errorf("the call the agent made after /think medium does not ask the server for that effort:\n%s", last)
	}
}

func TestAThinkLevelNobodyOffersIsRefusedInTheTerminal(t *testing.T) {
	agent := startTheAgent(t, aScriptThatAnswersOneQuestion())
	screen := agent.attach(t)

	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "think hardest"})

	refused := screen.waitForReplySaying(t, "hardest", 15*time.Second)
	for _, level := range contract.ThinkLevels() {
		if !strings.Contains(refused.Text, string(level)) {
			t.Errorf("the refusal leaves out the level %q a person could type instead:\n%s", level, refused.Text)
		}
	}
}
