package tui

import (
	"strconv"
	"strings"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// maxBudgetDigits is the longest run of digits the screen will read as a budget
// count, so that a line of nonsense cannot become a huge number.
const maxBudgetDigits = 9

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
	screen.everAttached = true

	setIfSent(fields, contract.StatusFieldModel, &screen.modelAlias)
	setIfSent(fields, contract.StatusFieldTask, &screen.taskID)
	setIfSent(fields, contract.StatusFieldTaskState, &screen.taskState)
	setIfSent(fields, contract.StatusFieldTokensIn, &screen.tokensIn)
	setIfSent(fields, contract.StatusFieldTokensOut, &screen.tokensOut)
	setIfSent(fields, contract.StatusFieldCost, &screen.money)
	setIfSent(fields, contract.StatusFieldBudget, &screen.budget)
	setCountIfSent(fields, contract.StatusFieldContextTokens, &screen.contextTokens)
	setCountIfSent(fields, contract.StatusFieldContextWindow, &screen.contextWindow)
	screen.readBudget(fields)

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

// readBudget works out how much of the task's budget is left, so that the status
// strip can draw a bar as well as the words. The count is the first number in
// the budget line, and the fullest report seen since this task started is what
// the bar is measured against; a new task starts the measure again.
func (screen *Screen) readBudget(fields map[string]string) {
	if _, sent := fields[contract.StatusFieldBudget]; !sent {
		return
	}
	if screen.taskID != screen.budgetTask {
		screen.budgetTask = screen.taskID
		screen.budgetMost = 0
	}
	screen.budgetNow = firstNumber(screen.budget)
	screen.budgetMost = max(screen.budgetMost, screen.budgetNow)
}

// firstNumber reads the first run of digits in a line of plain words, and says
// zero when there is none or when the run is too long to be a count of anything.
func firstNumber(text string) int {
	digits := ""
	for _, letter := range text {
		if letter >= '0' && letter <= '9' {
			digits += string(letter)
			continue
		}
		if digits != "" {
			break
		}
	}
	if digits == "" || len(digits) > maxBudgetDigits {
		return 0
	}
	count, err := strconv.Atoi(digits)
	if err != nil {
		return 0
	}
	return count
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

// setCountIfSent copies one field that counts something into the screen. A field
// that is not a plain number counts as nothing at all, because a screen must
// never measure one thing against another it could not read.
func setCountIfSent(fields map[string]string, name string, into *int) {
	value, sent := fields[name]
	if !sent {
		return
	}
	count, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || count < 0 {
		count = 0
	}
	*into = count
}
