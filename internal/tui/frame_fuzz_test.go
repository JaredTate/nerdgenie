package tui

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// drawsInsideTheTerminal fails the test when any row of the frame is wider than
// the terminal, which is the one promise the drawing must keep whatever the
// program sends.
func drawsInsideTheTerminal(t *testing.T, screen *Screen, width int) {
	t.Helper()
	for number, line := range strings.Split(screen.frame(), "\n") {
		if drawn := displayWidth(plainText(line)); drawn > width {
			t.Fatalf("row %d is %d columns wide and the terminal is %d: %q", number+1, drawn, width, line)
		}
	}
}

func FuzzTheFrameStaysInsideTheTerminal(f *testing.F) {
	f.Add("a plain reply", "a plain message", 80)
	f.Add("**bold** and `code`\n```\nfenced\n```", "/status", 60)
	f.Add(strings.Repeat("verylongwordwithnobreaks", 20), "汉字とひらがな", 40)
	f.Fuzz(func(t *testing.T, fromProgram string, fromPerson string, width int) {
		width = 24 + (width%200+200)%200
		screen, clock := newTestScreen(width, 24)
		screen.link = &recordingLink{}

		screen.remember(block{kind: blockPerson, text: fromPerson})
		send(screen, contract.SocketEnvelope{Type: contract.SocketDelta, Text: fromProgram})
		send(screen, contract.SocketEnvelope{Type: contract.SocketPreview, ID: "1", Text: fromProgram})
		send(screen, contract.SocketEnvelope{Type: contract.SocketError, Reason: fromProgram})
		advance(screen, clock, heartbeatInterval)
		screen.input.setText(fromPerson)

		drawsInsideTheTerminal(t, screen, width)
	})
}

func FuzzASocketLineIsDrawnOrIgnoredAndNeverCrashes(f *testing.F) {
	f.Add("{\"type\":\"reply\",\"text\":\"hello\"}")
	f.Add("{\"type\":\"preview\",\"id\":\"3\",\"text\":\"rm -rf /\"}")
	f.Add("{\"type\":\"shout\"}")
	f.Add("not json at all")
	f.Fuzz(func(t *testing.T, line string) {
		envelope, err := contract.DecodeSocketEnvelope([]byte(line))
		if err != nil {
			return
		}
		screen, clock := newTestScreen(80, 24)
		screen.link = &recordingLink{}
		send(screen, envelope)
		advance(screen, clock, heartbeatInterval)
		drawsInsideTheTerminal(t, screen, 80)
	})
}

func FuzzWrappingKeepsEveryWordAndFitsTheWidth(f *testing.F) {
	f.Add("a short line", 20)
	f.Add("one-very-long-word-with-no-spaces-at-all", 10)
	f.Add("汉字 と ひらがな", 5)
	f.Add("\xe3", 51)
	f.Fuzz(func(t *testing.T, text string, width int) {
		width = 1 + (width%120+120)%120
		lines := wrapText(text, width)
		for _, line := range lines {
			if displayWidth(line) > width && len([]rune(line)) > 1 {
				t.Fatalf("the line %q is %d columns wide and the width is %d", line, displayWidth(line), width)
			}
		}
		// A byte that is not a character cannot be drawn, and Go turns it into the
		// replacement character, so both sides are compared as characters.
		wanted := strings.Join(strings.Fields(string([]rune(text))), "")
		got := strings.Join(strings.Fields(strings.Join(lines, " ")), "")
		if wanted != got {
			t.Fatalf("wrapping %q at %d gave back %q, and no character may be lost", text, width, got)
		}
	})
}
