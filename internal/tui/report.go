package tui

// The name-and-value pairs a status message from the running program may carry.
// internal/contract gives the message one open map of fields rather than a fixed
// shape, so this is the list the screen reads; anything else in the map is
// ignored, and a field that is not sent leaves what is on the screen alone.
const (
	// modelField is the alias of the model the program is using.
	modelField = "model"
	// taskField is the task the work belongs to, such as "task 17".
	taskField = "task"
	// taskStateField is what that task is doing: running, waiting, or done.
	taskStateField = "taskState"
	// tokensInField and tokensOutField are what this session has cost so far.
	tokensInField  = "tokensIn"
	tokensOutField = "tokensOut"
	// costField is the money the provider reported, when it reports any.
	costField = "cost"
	// budgetField is what the task has left, such as "86 rounds, 51 min left".
	budgetField = "budget"
	// stateField is what the program is doing right now, in one of the words
	// below.
	stateField = "state"
	// toolField is the name of the tool that is running, which the status strip
	// says after the word "using".
	toolField = "tool"
	// toolLineField is one finished tool call, already written as the one dim
	// line the transcript keeps.
	toolLineField = "toolLine"
)

// reportedStates are the words the program uses for what it is doing. A word
// this screen does not know leaves the state alone, because a screen must never
// say something that is not true.
var reportedStates = map[string]programState{
	"idle":     stateIdle,
	"thinking": stateThinking,
	"using":    stateUsingTool,
	"waiting":  stateWaitingForYou,
	"paused":   statePaused,
}

// readStatus takes what the program says about itself and puts it in the header,
// the status strip, the transcript, and the command palette. A status message is
// also the program answering for itself, so it fills the health dot in.
func (screen *Screen) readStatus(fields map[string]string) {
	screen.attached = true
	screen.lastHealth = screen.now

	setIfSent(fields, modelField, &screen.modelAlias)
	setIfSent(fields, taskField, &screen.taskID)
	setIfSent(fields, taskStateField, &screen.taskState)
	setIfSent(fields, tokensInField, &screen.tokensIn)
	setIfSent(fields, tokensOutField, &screen.tokensOut)
	setIfSent(fields, costField, &screen.money)
	setIfSent(fields, budgetField, &screen.budget)

	if listed, sent := fields[commandsField]; sent {
		screen.learnCommands(listed)
	}
	if line, sent := fields[toolLineField]; sent && line != "" {
		screen.flushDeltas()
		screen.remember(block{kind: blockTool, text: line})
	}
	screen.readReportedState(fields)
}

// readReportedState takes the one word that says what the program is doing, and
// leaves the screen as it was when the word is one it does not know.
func (screen *Screen) readReportedState(fields map[string]string) {
	word, sent := fields[stateField]
	if !sent {
		return
	}
	doing, known := reportedStates[word]
	if !known {
		return
	}
	screen.setState(doing, fields[toolField])
}

// setIfSent copies one field into the screen when the program sent it, and
// leaves what is on the screen alone when it did not.
func setIfSent(fields map[string]string, name string, into *string) {
	if value, sent := fields[name]; sent {
		*into = value
	}
}
