package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JaredTate/coeus/internal/contract"
)

// recordingLink is a link that keeps every envelope the screen sends, which is
// how a test reads what the screen said to the running program.
type recordingLink struct {
	sent   []contract.SocketEnvelope
	refuse error
}

// Send keeps the envelope, or refuses it when the test asked it to.
func (link *recordingLink) Send(envelope contract.SocketEnvelope) error {
	if link.refuse != nil {
		return link.refuse
	}
	link.sent = append(link.sent, envelope)
	return nil
}

// screenWithLink builds a screen whose link keeps everything it is told.
func screenWithLink() (*Screen, *recordingLink) {
	screen, _ := newTestScreen(80, 24)
	link := &recordingLink{}
	screen.link = link
	return screen, link
}

// press gives the screen one ordinary key press.
func press(screen *Screen, letter rune) {
	screen.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{letter}})
}

// pressKey gives the screen one named key, such as Enter or Escape.
func pressKey(screen *Screen, which tea.KeyType) {
	screen.Update(tea.KeyMsg{Type: which})
}

// typeWord types a whole phrase into the input box, one key press at a time.
func typeWord(screen *Screen, text string) {
	for _, letter := range text {
		if letter == ' ' {
			pressKey(screen, tea.KeySpace)
			continue
		}
		press(screen, letter)
	}
}

// aPreview is the preview docs/TUI_DESIGN.md draws in its example frame.
func aPreview() contract.SocketEnvelope {
	return contract.SocketEnvelope{
		Type:  contract.SocketPreview,
		ID:    "3",
		Title: previewTitle,
		Text:  "browser_click e7 \"Post\"",
	}
}

func TestAPreviewDrawsTheCardAndWaitsForThePerson(t *testing.T) {
	screen, _ := screenWithLink()
	send(screen, aPreview())

	frame := screen.View()
	for _, wanted := range []string{previewTitle, "browser_click e7", "[a] approve once", "[A] always this session", "[r] reject with a reason"} {
		if !strings.Contains(frame, wanted) {
			t.Errorf("the frame does not hold %q, and the preview card shows the call and its three answers", wanted)
		}
	}
	if !strings.Contains(frame, "waiting for you") {
		t.Error("the status strip does not say the screen is waiting for the person, and a card has the keys")
	}
}

func TestApprovingOnceSendsTheOnceAnswerAndRecordsIt(t *testing.T) {
	screen, link := screenWithLink()
	send(screen, aPreview())
	press(screen, 'a')

	if len(link.sent) != 1 {
		t.Fatalf("pressing a sent %d envelopes, and it sends one", len(link.sent))
	}
	sent := link.sent[0]
	if sent.Type != contract.SocketApprove || sent.ID != "3" || sent.Text != "" {
		t.Errorf("pressing a sent %+v, and an approve carrying no text approves preview 3 for this one call", sent)
	}
	if !strings.Contains(screen.View(), "approved once") {
		t.Error("the transcript does not record the answer, and every answer is written down in one dim line")
	}
}

func TestApprovingAlwaysSendsTheAlwaysAnswer(t *testing.T) {
	screen, link := screenWithLink()
	send(screen, aPreview())
	press(screen, 'A')

	if len(link.sent) != 1 || link.sent[0].Text != contract.ApproveAlwaysText {
		t.Fatalf("pressing A sent %+v, and it approves calls like this one for the rest of the session", link.sent)
	}
	if !strings.Contains(screen.View(), "approved for this session") {
		t.Error("the transcript does not record that the answer was for the whole session")
	}
}

func TestRejectingAsksWhyAndThenSendsTheReason(t *testing.T) {
	screen, link := screenWithLink()
	send(screen, aPreview())
	press(screen, 'r')

	if !strings.Contains(screen.View(), "Why not?") {
		t.Error("pressing r did not ask for a reason, and the answer is to reject with a reason")
	}
	if len(link.sent) != 0 {
		t.Fatalf("pressing r sent %+v before the reason was typed", link.sent)
	}

	typeWord(screen, "not the right account")
	pressKey(screen, tea.KeyEnter)

	if len(link.sent) != 1 {
		t.Fatalf("answering the reason sent %d envelopes, and it sends one", len(link.sent))
	}
	sent := link.sent[0]
	if sent.Type != contract.SocketDeny || sent.ID != "3" || sent.Reason != "not the right account" {
		t.Errorf("rejecting sent %+v, and it denies preview 3 with the reason that was typed", sent)
	}
	if !strings.Contains(screen.View(), "rejected: not the right account") {
		t.Error("the transcript does not record the rejection and its reason")
	}
}

func TestAnAnsweredCardGivesTheKeysBackToTheInputBox(t *testing.T) {
	screen, _ := screenWithLink()
	send(screen, aPreview())
	press(screen, 'a')
	typeWord(screen, "hello")

	if screen.input.text() != "hello" {
		t.Errorf("the input box holds %q after the card was answered, and the keys belong to it again", screen.input.text())
	}
}

func TestAnErrorFromTheLinkBecomesAnErrorCard(t *testing.T) {
	screen, link := screenWithLink()
	link.refuse = errors.New("the link went away while you were typing, so wait for it to come back")
	typeWord(screen, "hello")
	pressKey(screen, tea.KeyEnter)

	if !strings.Contains(screen.View(), errorTitle) {
		t.Error("a message that could not be delivered was lost quietly, and it must become an error card")
	}
}
