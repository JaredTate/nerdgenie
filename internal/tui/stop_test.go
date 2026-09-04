package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// stopsOnEscape says whether Escape sent the stop command to the program.
func stopsOnEscape(screen *Screen, link *recordingLink) bool {
	pressKey(screen, tea.KeyEsc)
	for _, sent := range link.sent {
		if sent.Type == contract.SocketCommand && sent.Text == "stop" {
			return true
		}
	}
	return false
}

func TestEscapeStopsWhateverTheModelIsDoing(t *testing.T) {
	for name, one := range map[string]struct {
		state     programState
		taskState string
		stops     bool
	}{
		"while the model is thinking":        {state: stateThinking, stops: true},
		"while a tool is running":            {state: stateUsingTool, stops: true},
		"while a task runs and nothing else": {state: stateIdle, taskState: "running", stops: true},
		"while a task runs and the model thinks": {
			state: stateThinking, taskState: "running", stops: true,
		},
		"with nothing running at all":      {state: stateIdle, stops: false},
		"while the screen waits on a card": {state: stateWaitingForYou, stops: false},
	} {
		screen, link := screenWithLink()
		screen.taskState = one.taskState
		screen.setState(one.state, "")

		if stopped := stopsOnEscape(screen, link); stopped != one.stops {
			if one.stops {
				t.Errorf("Escape %s sent %+v, and it stops what is running", name, link.sent)
				continue
			}
			t.Errorf("Escape %s sent the stop command, and there is nothing to stop", name)
		}
	}
}
