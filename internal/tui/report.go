package tui

import (
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// reportedStates turns the word a status message carries in its state field into
// what this screen says in its status strip. A word that is not one of the five
// internal/contract names leaves the state alone, because a screen must never
// say something that is not true.
var reportedStates = map[string]programState{
	contract.StateIdle:          stateIdle,
	contract.StateThinking:      stateThinking,
	contract.StateUsingTool:     stateUsingTool,
	contract.StateWaitingForYou: stateWaitingForYou,
	contract.StatePaused:        statePaused,
}

// readStatus takes what the program says about itself and puts it in the header,
// the status strip, the transcript, and the command palette. The field names are
// the ones internal/contract lists; a field that is not sent leaves what is on
// the screen alone, and a field the screen does not know is ignored.
func (screen *Screen) readStatus(fields map[string]string) {
	screen.attached = true

	setIfSent(fields, contract.StatusFieldModel, &screen.modelAlias)
	setIfSent(fields, contract.StatusFieldTask, &screen.taskID)
	setIfSent(fields, contract.StatusFieldTaskState, &screen.taskState)
	setIfSent(fields, contract.StatusFieldTokensIn, &screen.tokensIn)
	setIfSent(fields, contract.StatusFieldTokensOut, &screen.tokensOut)
	setIfSent(fields, contract.StatusFieldCost, &screen.money)
	setIfSent(fields, contract.StatusFieldBudget, &screen.budget)

	if listed, sent := fields[contract.StatusFieldCommands]; sent {
		screen.learnCommands(listed)
	}
	if line, sent := fields[contract.StatusFieldToolLine]; sent && line != "" {
		screen.flushDeltas()
		screen.remember(block{kind: blockTool, text: line})
	}
	screen.readHealth(fields)
	screen.readReportedState(fields)
}

// readHealth fills in the dot on the right of the header. The program says in so
// many words whether its health check answered, and a status message that does
// not say counts as an answer of its own, because the program sent it.
func (screen *Screen) readHealth(fields map[string]string) {
	answered, sent := fields[contract.StatusFieldHealthy]
	if sent && answered != "true" {
		screen.lastHealth = time.Time{}
		return
	}
	screen.lastHealth = screen.now
}

// readReportedState takes the one word that says what the program is doing, and
// leaves the screen as it was when the word is one it does not know.
func (screen *Screen) readReportedState(fields map[string]string) {
	word, sent := fields[contract.StatusFieldState]
	if !sent {
		return
	}
	doing, known := reportedStates[word]
	if !known {
		return
	}
	screen.setState(doing, fields[contract.StatusFieldTool])
}

// setIfSent copies one field into the screen when the program sent it, and
// leaves what is on the screen alone when it did not.
func setIfSent(fields map[string]string, name string, into *string) {
	if value, sent := fields[name]; sent {
		*into = value
	}
}
