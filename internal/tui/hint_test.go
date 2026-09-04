package tui

import (
	"strings"
	"testing"
)

// TestTheWelcomeTellsThePersonHowToStart holds finding 5 of the first trial:
// the first frame says where the commands are, and now also what to type,
// because the first person to use the screen asked how to see what jobs and
// tasks it had and nothing on the frame answered.
func TestTheWelcomeTellsThePersonHowToStart(t *testing.T) {
	screen, _ := newTestScreen(80, 24)

	if !strings.Contains(screen.frame(), askHintText) {
		t.Errorf("the first frame does not say %q, and it is the only thing on it that says how to start:\n%s", askHintText, screen.frame())
	}
}

func TestTheHintGoesAwayWithTheWelcomeOnceThereIsAConversation(t *testing.T) {
	screen, _ := newTestScreen(80, 24)
	screen.remember(block{kind: blockPerson, text: "the first thing anyone typed"})

	if strings.Contains(screen.frame(), askHintText) {
		t.Error("the welcome's hint is still on the frame once there is a conversation to read")
	}
}
