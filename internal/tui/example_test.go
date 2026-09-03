package tui

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theExampleTask fills a screen with the conversation docs/TUI_DESIGN.md draws
// in its example frame, so that the whole frame can be read at once and put
// beside the drawing in that document.
func theExampleTask(screen *Screen) {
	screen.Update(linkMessage{up: true})
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldModel:     "opus",
		contract.StatusFieldTask:      "task 17",
		contract.StatusFieldTaskState: "running",
		contract.StatusFieldTokensIn:  "6.1k",
		contract.StatusFieldTokensOut: "0.4k",
		contract.StatusFieldCost:      "$0.04",
		contract.StatusFieldBudget:    "86 rounds, 51 min left",
	}})
	screen.remember(block{kind: blockPerson, text: "Post a tweet about the DigiByte anniversary. Use the product notes and keep it under 280 characters."})
	screen.remember(block{kind: blockReply, text: "Where I stand: the notes are read, drafting next."})
	screen.remember(block{kind: blockTool, text: "read memory/product.md · 2,100 characters · r3"})
	screen.remember(block{kind: blockTool, text: "task · plan set, done list set"})
	screen.remember(block{kind: blockReply, text: "Draft: \"Nine years ago today DigiByte …\"  (236 characters)"})
	screen.remember(block{kind: blockTool, text: "browser_open x.com/compose/post · Compose post"})
	send(screen, contract.SocketEnvelope{
		Type: contract.SocketPreview,
		ID:   "3",
		Text: "browser_click e7 \"Post\"\nPosts to the DigiByte account:\n\"Nine years ago today DigiByte …\"",
	})
}

func TestTheExampleFrameIsDrawnAsTheDesignDrawsIt(t *testing.T) {
	screen, _ := newTestScreen(80, 30)
	screen.link = &recordingLink{}
	theExampleTask(screen)

	frame := screen.View()
	testkit.Golden(t, "example-frame.txt", []byte(frame))

	for number, line := range strings.Split(frame, "\n") {
		if width := len([]rune(line)); width > 80 {
			t.Errorf("row %d is %d columns wide: %q", number+1, width, line)
		}
	}
}
