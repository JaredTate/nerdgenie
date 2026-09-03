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
	setIfSent(fields, statusFieldPlan, &screen.plan)
	setIfSent(fields, statusFieldJobs, &screen.jobs)
	setCountIfSent(fields, contract.StatusFieldContextTokens, &screen.contextTokens)
	setCountIfSent(fields, contract.StatusFieldContextWindow, &screen.contextWindow)
	setCountIfSent(fields, contract.StatusFieldStreamed, &screen.streamed)
	setMomentIfSent(fields, contract.StatusFieldCallStarted, &screen.callStarted)
	screen.readBudget(fields)

	if listed, sent := fields[contract.StatusFieldCommands]; sent {
		screen.learnCommands(listed)
	}
	screen.readToolLine(fields)
	screen.readRecordLine(fields)
	screen.readHealth(fields)
	screen.readReportedState(fields)
}

// readToolLine puts one pill in the transcript for the call in flight. The
// program sends the same line on every heartbeat until the call changes, so a
// line that is what is already on the screen is not drawn again, and a line that
// is the same call with what came back added to it takes the place of the pill
// that call already has rather than making a second one. The same call made
// again, straight after the last one came back, is counted on the pill it
// already has rather than given another.
func (screen *Screen) readToolLine(fields map[string]string) {
	line, sent := fields[contract.StatusFieldToolLine]
	if !sent || line == "" || line == screen.lastTool {
		return
	}
	screen.flushDeltas()
	if !screen.replacePill(screen.lastTool, line) && !screen.countTheCallAgain(line) {
		screen.remember(block{kind: blockTool, text: line})
	}
	screen.lastTool = line
}

// countTheCallAgain writes a line for the same call as the newest pill, the same
// tool with the same argument made again after the last one came back, into
// that pill with a count, and says whether it did. The trial saw a model that
// had got stuck run one shell command thirteen times over and fill the screen
// with it; one row that says how many times tells the person more. Only the
// newest pill is looked at, because a count is for a run of the same call and
// not for a call that came round again later.
func (screen *Screen) countTheCallAgain(line string) bool {
	if len(screen.blocks) == 0 {
		return false
	}
	newest := &screen.blocks[len(screen.blocks)-1]
	if newest.kind != blockTool || callOf(newest.text) != callOf(line) {
		return false
	}
	newest.repeats = max(newest.repeats, 1) + 1
	screen.setText(newest, line)
	return true
}

// toolLineSeparator is what the program writes between a call and what came back
// of it, which is internal/loop's ToolLineSeparator spelled here because the
// screen imports nothing of Coeus but the contract and the clock.
const toolLineSeparator = " · "

// callOf is the part of a tool line that names the call: the tool and its main
// argument, which is everything before the separator the program writes between
// the call and what came back of it.
func callOf(line string) string {
	call, _, _ := strings.Cut(line, toolLineSeparator)
	return strings.TrimSpace(call)
}

// replacePill writes a newer line into the pill an older one is already drawn
// in, when the newer line is the older one with more added to the end of it,
// which is what a call gaining its result looks like. It says whether it found
// that pill.
func (screen *Screen) replacePill(older string, newer string) bool {
	if older == "" || !strings.HasPrefix(newer, older) {
		return false
	}
	for at := len(screen.blocks) - 1; at >= 0; at-- {
		if screen.blocks[at].kind == blockTool && screen.blocks[at].text == older {
			screen.setText(&screen.blocks[at], newer)
			return true
		}
	}
	return false
}

// readRecordLine puts one pill in the transcript for the latest change to the
// record, drawn exactly as a tool call is, so that a person can watch tasks and
// jobs start and finish without asking. The program sends the same line on every
// heartbeat until something else changes, so only a line that differs from the
// last one shown is worth a pill of its own.
func (screen *Screen) readRecordLine(fields map[string]string) {
	line, sent := fields[contract.StatusFieldRecordLine]
	if !sent || line == "" || line == screen.lastRecord {
		return
	}
	screen.lastRecord = line
	screen.flushDeltas()
	screen.remember(block{kind: blockTool, text: line})
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

// setMomentIfSent copies one field that names a moment into the screen. The
// program writes it the RFC 3339 way, and a field written any other way counts
// as no moment at all, because a screen that counted from a time it could not
// read would draw a number that means nothing.
func setMomentIfSent(fields map[string]string, name string, into *time.Time) {
	value, sent := fields[name]
	if !sent {
		return
	}
	moment, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		*into = time.Time{}
		return
	}
	*into = moment
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
