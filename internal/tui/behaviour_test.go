package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// quitsOnControlC says whether one Ctrl+C told Bubble Tea to stop.
func quitsOnControlC(screen *Screen) bool {
	_, command := pressWithControl(screen, 'c')
	if command == nil {
		return false
	}
	_, isQuit := command().(tea.QuitMsg)
	return isQuit
}

func TestControlCOnceArmsTheQuitAndTwiceDoesIt(t *testing.T) {
	screen, _ := screenWithLink()
	typeWord(screen, "half a message")

	if quitsOnControlC(screen) {
		t.Fatal("one Ctrl+C quit, and one stray press must never throw away what was typed")
	}
	if !quitsOnControlC(screen) {
		t.Fatal("two Ctrl+C presses did not quit")
	}
}

func TestAKeyBetweenTheTwoControlCsDisarmsTheQuit(t *testing.T) {
	screen, _ := screenWithLink()
	pressWithControl(screen, 'c')
	press(screen, 'x')
	if quitsOnControlC(screen) {
		t.Error("a Ctrl+C after other typing quit, and the two presses must be next to each other")
	}
}

func TestEscapeStopsTheTurnOnlyWhileTheProgramIsBusy(t *testing.T) {
	screen, link := screenWithLink()
	pressKey(screen, tea.KeyEsc)
	if len(link.sent) != 0 {
		t.Fatalf("Escape sent %+v with nothing running", link.sent)
	}

	screen.setState(stateThinking, "")
	pressKey(screen, tea.KeyEsc)
	if len(link.sent) != 1 || link.sent[0].Type != contract.SocketCommand || link.sent[0].Text != "stop" {
		t.Errorf("Escape while a task runs sent %+v, and it sends the stop command", link.sent)
	}
}

func TestUpRecallsWhatWasSentAndDownComesBackToAnEmptyBox(t *testing.T) {
	screen, _ := screenWithLink()
	typeWord(screen, "the first thing")
	pressKey(screen, tea.KeyEnter)
	typeWord(screen, "the second thing")
	pressKey(screen, tea.KeyEnter)

	pressKey(screen, tea.KeyUp)
	if screen.input.text() != "the second thing" {
		t.Errorf("Up put %q in the box, and the newest message is %q", screen.input.text(), "the second thing")
	}
	pressKey(screen, tea.KeyUp)
	if screen.input.text() != "the first thing" {
		t.Errorf("a second Up put %q in the box", screen.input.text())
	}
	pressKey(screen, tea.KeyDown)
	pressKey(screen, tea.KeyDown)
	if screen.input.text() != "" {
		t.Errorf("walking back past the newest left %q in the box, and it should be empty", screen.input.text())
	}
}

func TestScrollingUpShowsTheOlderMarkAndScrollingBackClearsIt(t *testing.T) {
	screen, _ := screenWithLink()
	for range 40 {
		screen.remember(block{kind: blockReply, text: "one more line of a long conversation"})
	}
	if strings.Contains(statusStrip(screen), olderMark) {
		t.Fatal("the older mark is showing while the view is at the bottom")
	}

	pressKey(screen, tea.KeyPgUp)
	if !strings.Contains(statusStrip(screen), olderMark) {
		t.Errorf("the status strip is %q after scrolling up, and it should say the view is on older rows", statusStrip(screen))
	}
	pressKey(screen, tea.KeyPgDown)
	if strings.Contains(statusStrip(screen), olderMark) {
		t.Error("the older mark is still showing after scrolling back to the bottom")
	}
}

func TestTheInputBoxGrowsToFiveRowsAndThenScrollsInside(t *testing.T) {
	screen, _ := screenWithLink()
	screen.input.setText(strings.Repeat("x", 76*7))
	if rows := len(screen.editorRows()); rows != maxInputRows {
		t.Errorf("the input box is %d rows for text that needs seven, and it grows to %d and then scrolls inside", rows, maxInputRows)
	}
}

func TestTheInputBoxRefusesMoreThanItsCap(t *testing.T) {
	screen, _ := screenWithLink()
	screen.input.insert(strings.Repeat("y", maxInputRunes+500))
	if len(screen.input.letters) != maxInputRunes {
		t.Errorf("the box holds %d characters and the cap is %d", len(screen.input.letters), maxInputRunes)
	}
	screen.input.insert("z")
	if len(screen.input.letters) != maxInputRunes {
		t.Error("the box took a character it had no room for")
	}
}

func TestTheCursorIsDrawnWhereItSitsInsideTheText(t *testing.T) {
	screen, _ := screenWithLink()
	typeWord(screen, "hello")
	pressKey(screen, tea.KeyLeft)
	pressKey(screen, tea.KeyLeft)

	drawn := screen.editorRows()[0]
	if !strings.Contains(drawn, reverseCode+"l") {
		t.Errorf("the input row is %q, and the cursor sits over the character it is on", drawn)
	}

	pressKey(screen, tea.KeyHome)
	if screen.input.cursor != 0 {
		t.Errorf("Home left the cursor at %d", screen.input.cursor)
	}
	pressKey(screen, tea.KeyEnd)
	if screen.input.cursor != 5 {
		t.Errorf("End left the cursor at %d and the text is five characters long", screen.input.cursor)
	}
	pressKey(screen, tea.KeyBackspace)
	if screen.input.text() != "hell" {
		t.Errorf("Backspace left %q in the box", screen.input.text())
	}
}

func TestAHandoffShowsThePathWhenTheTerminalCannotDrawPictures(t *testing.T) {
	screen, _ := screenWithLink()
	send(screen, contract.SocketEnvelope{
		Type:        contract.SocketHandoff,
		ID:          "9",
		Text:        "log in and then press a",
		Attachments: []string{"/tmp/a-screenshot.png"},
	})

	frame := screen.frame()
	for _, wanted := range []string{handoffTitle, "open this file to see the page", "a-screenshot.png"} {
		if !strings.Contains(frame, wanted) {
			t.Errorf("the handoff card does not hold %q", wanted)
		}
	}
}

func TestAHandoffDrawsThePictureWhenTheTerminalCan(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "screenshot.png")
	if err := os.WriteFile(path, []byte("not really a picture, but bytes all the same"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the test picture: %v", err)
	}

	screen := New(Options{
		Clock:  testkitClock(),
		Width:  80,
		Height: 24,
		Environment: func(name string) string {
			return map[string]string{"NO_COLOR": "1", "TERM": "xterm-kitty"}[name]
		},
	})
	screen.link = &recordingLink{}
	send(screen, contract.SocketEnvelope{Type: contract.SocketHandoff, ID: "9", Attachments: []string{path}})

	frame := screen.frame()
	if strings.Contains(frame, "open this file to see the page") {
		t.Error("a terminal that draws pictures was given the path instead")
	}
	if !strings.Contains(frame, "\x1b_G") {
		t.Error("the kitty graphics escape is not in the frame")
	}
}

func TestAPictureThatCannotBeReadFallsBackToItsPath(t *testing.T) {
	screen := New(Options{
		Clock:  testkitClock(),
		Width:  80,
		Height: 24,
		Environment: func(name string) string {
			return map[string]string{"NO_COLOR": "1", "TERM_PROGRAM": "iTerm.app"}[name]
		},
	})
	screen.link = &recordingLink{}
	send(screen, contract.SocketEnvelope{Type: contract.SocketHandoff, Attachments: []string{"/no/such/picture.png"}})

	if !strings.Contains(screen.frame(), "open this file to see the page") {
		t.Error("a picture that cannot be read did not fall back to its path")
	}
}

func TestAnErrorMessageFromTheProgramBecomesAnErrorCard(t *testing.T) {
	screen, _ := screenWithLink()
	send(screen, contract.SocketEnvelope{Type: contract.SocketError, Text: "the tool failed.", Reason: "the file is not there."})

	frame := screen.frame()
	for _, wanted := range []string{errorTitle, "the tool failed.", "the file is not there."} {
		if !strings.Contains(frame, wanted) {
			t.Errorf("the error card does not hold %q", wanted)
		}
	}
}

func TestAnErrorWithNothingInItStillSaysSomething(t *testing.T) {
	screen, _ := screenWithLink()
	send(screen, contract.SocketEnvelope{Type: contract.SocketError})
	if !strings.Contains(screen.frame(), "did not say what it was") {
		t.Error("an error with no words in it drew an empty card")
	}
}

func TestAVeryLongReplyKeepsItsEndAndSaysSo(t *testing.T) {
	screen, clock := newTestScreen(80, 24)
	screen.link = &recordingLink{}
	send(screen, contract.SocketEnvelope{
		Type: contract.SocketDelta,
		Text: strings.Repeat("a", maxBlockRunes) + "THEENDOFIT",
	})
	advance(screen, clock, heartbeatInterval)

	texts := replyTexts(screen)
	if len(texts) != 1 {
		t.Fatalf("there are %d reply blocks", len(texts))
	}
	if !strings.HasSuffix(texts[0], "THEENDOFIT") {
		t.Error("the block did not keep the end of the reply, which is the part being read")
	}
	if !strings.Contains(texts[0], "is not shown here") {
		t.Error("the block threw the beginning away without saying so")
	}
}

func TestTheTranscriptDropsItsOldestBlockWhenItIsFull(t *testing.T) {
	screen, _ := screenWithLink()
	screen.remember(block{kind: blockReply, text: "the very first thing said"})
	for range maxTranscriptBlocks {
		screen.remember(block{kind: blockReply, text: "another thing"})
	}
	if len(screen.blocks) != maxTranscriptBlocks {
		t.Fatalf("the transcript holds %d blocks and the cap is %d", len(screen.blocks), maxTranscriptBlocks)
	}
	if screen.blocks[0].text == "the very first thing said" {
		t.Error("the oldest block is still there, and the oldest is what is dropped")
	}
}

func TestANarrowTerminalDropsTheRightHandSideRatherThanWrapping(t *testing.T) {
	screen, _ := newTestScreen(50, 24)
	screen.Update(linkMessage{up: true})
	send(screen, aFullStatus())

	if strings.Contains(headerOf(screen), "healthy") {
		t.Error("a fifty-column terminal still draws the health dot, and the right side is dropped below sixty")
	}
	if strings.Contains(statusStrip(screen), "Ctrl+J") {
		t.Error("a fifty-column terminal still draws the key hints, and they are dropped below sixty")
	}
	for _, line := range strings.Split(screen.frame(), "\n") {
		if displayWidth(line) > 50 {
			t.Errorf("the row %q is wider than the terminal", line)
		}
	}
}

func TestATinyTerminalIsDrawnAtTheSmallestSizeRatherThanBreaking(t *testing.T) {
	screen, _ := newTestScreen(80, 24)
	aTalkedTranscript(screen)
	resizeTo(screen, 2, 1)

	rows := strings.Split(screen.frame(), "\n")
	if len(rows) != smallestHeight {
		t.Errorf("a terminal of one row drew %d rows, and the smallest frame is %d", len(rows), smallestHeight)
	}
	for _, line := range rows {
		if displayWidth(line) > smallestWidth {
			t.Errorf("the row %q is wider than the smallest frame", line)
		}
	}
}
