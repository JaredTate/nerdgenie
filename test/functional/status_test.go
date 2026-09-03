// The whole-program test for what the terminal header draws: after a model call
// the status carries how much context the call held, how much the model can
// hold, and what the session has spent, and while a call is in flight it carries
// when the call began. The first human trial found a header with no numbers in
// it at all.
package functional

import (
	"strconv"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// The token counts the scripted model reports, which are the numbers the status
// has to carry back to the header.
const (
	theCallHeld  = 1234
	theCallWrote = 56
)

func TestTheStatusCarriesTheContextMeasureAfterAModelCall(t *testing.T) {
	agent := startTheAgent(t, testkit.Script{
		Name:          "local",
		ContextLength: 32768,
		Steps: []testkit.Step{{
			Expect: []string{"how big is your window"},
			Text:   "My window is thirty-two thousand tokens.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: theCallHeld, OutputTokens: theCallWrote},
		}},
	})

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "how big is your window?"})

	status := screen.waitForStatusCarrying(t, contract.StatusFieldContextTokens, 60*time.Second)
	wanted := map[string]string{
		contract.StatusFieldContextTokens: strconv.Itoa(theCallHeld),
		contract.StatusFieldContextWindow: "32768",
		contract.StatusFieldTokensIn:      strconv.Itoa(theCallHeld),
		contract.StatusFieldTokensOut:     strconv.Itoa(theCallWrote),
	}
	for field, number := range wanted {
		if status.Fields[field] != number {
			t.Errorf("the status carries %s = %q, want %q; the whole status is %v",
				field, status.Fields[field], number, status.Fields)
		}
	}
	// The home this test writes sets no budget, and the shipped caps set none,
	// so the line says so rather than naming a limit nobody set.
	if budget := status.Fields[contract.StatusFieldBudget]; budget != "no budget" {
		t.Errorf("the status carries the budget line %q, want \"no budget\", because none is set: %v", budget, status.Fields)
	}
}

func TestTheStatusSaysWhenTheCallInFlightBegan(t *testing.T) {
	agent := startTheAgent(t, testkit.Script{
		Name:          "local",
		ContextLength: 32768,
		Steps: []testkit.Step{{
			Text:   "I am here.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 100, OutputTokens: 5},
		}},
	})
	agent.model.MisbehaveNext(testkit.StallTheStream, 20*time.Second)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "are you there?"})

	status := screen.waitForStatusCarrying(t, contract.StatusFieldCallStarted, 30*time.Second)
	began, err := time.Parse(time.RFC3339, status.Fields[contract.StatusFieldCallStarted])
	if err != nil {
		t.Fatalf("the moment the call began is %q, which is not a time written the way the contract says: %v",
			status.Fields[contract.StatusFieldCallStarted], err)
	}
	if time.Since(began) > time.Minute {
		t.Errorf("the call is said to have begun at %s, which is not this call", began)
	}
	if status.Fields[contract.StatusFieldState] != contract.StateThinking {
		t.Errorf("the state while a call is in flight is %q, want %q",
			status.Fields[contract.StatusFieldState], contract.StateThinking)
	}
}
