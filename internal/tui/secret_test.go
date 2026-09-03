package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JaredTate/coeus/internal/contract"
)

// theSecret is what the tests type at the masked prompt. No part of it may ever
// reach the frame, the history, or the transcript.
const theSecret = "hunter2 is the key"

// askForASecret is the message the program sends when it wants a password: an
// ask that tells the screen to hide what is typed.
func askForASecret() contract.SocketEnvelope {
	return contract.SocketEnvelope{
		Type:      contract.SocketAsk,
		ID:        "7",
		Title:     "API key for anthropic",
		MaskInput: true,
	}
}

func TestTheMaskedPromptNeverEchoesWhatIsTyped(t *testing.T) {
	screen, link := screenWithLink()
	send(screen, askForASecret())
	typeWord(screen, theSecret)

	frame := screen.View()
	if strings.Contains(frame, "hunter2") {
		t.Fatal("the secret is on the frame, and nothing typed at a masked prompt is ever drawn")
	}
	if !strings.Contains(frame, "API key for anthropic") {
		t.Error("the frame does not say what secret is being asked for, and the title sits dim above the box")
	}
	if bullets := strings.Count(frame, string(maskGlyph)); bullets != len([]rune(theSecret)) {
		t.Errorf("the frame holds %d bullets and the secret is %d characters long", bullets, len([]rune(theSecret)))
	}

	pressKey(screen, tea.KeyEnter)
	if len(link.sent) != 1 {
		t.Fatalf("answering the masked prompt sent %d envelopes, and it sends one", len(link.sent))
	}
	sent := link.sent[0]
	if sent.Type != contract.SocketSecret || sent.ID != "7" || sent.Secret != theSecret {
		t.Errorf("the masked prompt sent %+v, and it carries the secret in the field made for it", sent)
	}
	if sent.Text != "" {
		t.Errorf("the envelope's ordinary text is %q, and a secret never travels as ordinary text", sent.Text)
	}
}

func TestNothingTypedAtAMaskedPromptIsKept(t *testing.T) {
	screen, _ := screenWithLink()
	send(screen, askForASecret())
	typeWord(screen, theSecret)
	pressKey(screen, tea.KeyEnter)

	if strings.Contains(screen.View(), "hunter2") {
		t.Error("the secret reached the transcript, and nothing typed at a masked prompt is written down")
	}
	if len(screen.history) != 0 {
		t.Errorf("the history holds %q, and nothing typed at a masked prompt goes into it", screen.history)
	}
	if screen.input.secret {
		t.Error("the box is still in secret mode after the secret was sent")
	}
}

func TestEscapeCancelsAMaskedPromptAndTellsTheProgram(t *testing.T) {
	screen, link := screenWithLink()
	send(screen, askForASecret())
	typeWord(screen, theSecret)
	pressKey(screen, tea.KeyEsc)

	if screen.input.secret {
		t.Error("Escape did not leave secret mode")
	}
	if len(link.sent) != 1 || link.sent[0].Type != contract.SocketCancel || link.sent[0].ID != "7" {
		t.Errorf("cancelling sent %+v, and the program is told so that it is not left waiting", link.sent)
	}
	if link.sent[0].Secret != "" {
		t.Error("cancelling carried a secret, and there is nothing to carry")
	}
}

func TestTheMaskedPromptUsesAStarWhereTheTerminalHasNoPadlock(t *testing.T) {
	screen, _ := screenWithLink()
	send(screen, askForASecret())

	if !strings.Contains(screen.View(), "*") {
		t.Error("the prompt glyph is not a star, and this terminal's settings do not promise an emoji")
	}

	withEmoji := New(Options{
		Clock:       screen.clock,
		Width:       80,
		Height:      24,
		Environment: func(name string) string { return map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}[name] },
	})
	withEmoji.link = &recordingLink{}
	send(withEmoji, askForASecret())
	if !strings.Contains(withEmoji.View(), string(lockGlyph)) {
		t.Error("the prompt glyph is not the padlock, and this terminal's settings promise an emoji")
	}
}

func TestAnOrdinaryQuestionIsNotMasked(t *testing.T) {
	screen, _ := screenWithLink()
	send(screen, contract.SocketEnvelope{Type: contract.SocketAsk, ID: "8", Title: questionTitle, Text: "which account?"})
	typeWord(screen, "the DigiByte one")

	if screen.input.secret {
		t.Error("an ordinary question put the box into secret mode, and only a masked ask does that")
	}
	if !strings.Contains(screen.View(), "the DigiByte one") {
		t.Error("an ordinary answer is not being drawn, and only a secret is hidden")
	}
}
