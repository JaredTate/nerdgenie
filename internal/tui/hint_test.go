package tui

import (
	"strings"
	"testing"
)

func TestTheBannerTellsThePersonHowToSeeTheCommands(t *testing.T) {
	screen, _ := newTestScreen(80, 24)

	if !strings.Contains(screen.View(), commandHintText) {
		t.Errorf("the first frame does not say %q, and it is the only thing on it that says where the commands are:\n%s", commandHintText, screen.View())
	}
}

func TestTheHintGoesAwayWithTheBannerOnceThereIsAConversation(t *testing.T) {
	screen, _ := newTestScreen(80, 24)
	screen.remember(block{kind: blockPerson, text: "the first thing anyone typed"})

	if strings.Contains(screen.View(), commandHintText) {
		t.Error("the banner's hint is still on the frame once there is a conversation to read")
	}
}
