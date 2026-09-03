package tui

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestAnErrorCardAboveAPreviewDoesNotStealItsAnswers(t *testing.T) {
	for name, pressed := range map[string]rune{"approving once": 'a', "approving this session": 'A'} {
		screen, link := screenWithLink()
		send(screen, aPreview())
		send(screen, contract.SocketEnvelope{Type: contract.SocketError, Text: "the queue is full."})
		press(screen, pressed)

		if len(link.sent) != 1 || link.sent[0].Type != contract.SocketApprove || link.sent[0].ID != "3" {
			t.Errorf("%s after an error card sent %+v, and the preview still holds the single keys", name, link.sent)
		}
		if screen.input.text() != "" {
			t.Errorf("%s after an error card typed %q into the input box instead", name, screen.input.text())
		}
	}
}

func TestAnErrorCardAboveAPreviewDoesNotStealTheRejectKey(t *testing.T) {
	screen, link := screenWithLink()
	send(screen, aPreview())
	send(screen, contract.SocketEnvelope{Type: contract.SocketError, Text: "the queue is full."})
	press(screen, 'r')

	if !strings.Contains(screen.frame(), "Why not?") {
		t.Fatal("r after an error card did not ask for a reason, and the preview still holds the single keys")
	}
	typeWord(screen, "not that account")
	pressKey(screen, tea.KeyEnter)
	if len(link.sent) != 1 || link.sent[0].Type != contract.SocketDeny || link.sent[0].ID != "3" {
		t.Errorf("rejecting after an error card sent %+v, and it denies the preview that is waiting", link.sent)
	}
}

func TestTheFirstFrameIsDrawnAtTheSizeTheTerminalReports(t *testing.T) {
	frame := New(Options{
		Clock:       testkitClock(),
		Environment: plainEnvironment,
		Width:       100,
		Height:      30,
	}).frame()

	rows := strings.Split(frame, "\n")
	if len(rows) != 30 {
		t.Fatalf("the first frame is %d rows and the terminal is 30", len(rows))
	}
	if width := displayWidth(rows[0]); width != 100 {
		t.Errorf("the header is %d columns and the terminal is 100", width)
	}
}

func TestTheSizeForTheFirstFrameComesFromTheTerminalAndFallsBackToEightyByTwentyFour(t *testing.T) {
	width, height := firstFrameSize(Options{Size: func() (int, int, bool) { return 100, 30, true }})
	if width != 100 || height != 30 {
		t.Errorf("the terminal said one hundred by thirty and the first frame is %d by %d", width, height)
	}
	width, height = firstFrameSize(Options{Size: func() (int, int, bool) { return 0, 0, false }})
	if width != fallbackWidth || height != fallbackHeight {
		t.Errorf("a terminal that will not say its size drew %d by %d, and the fallback is %d by %d",
			width, height, fallbackWidth, fallbackHeight)
	}
	width, height = firstFrameSize(Options{Width: 42, Height: 12, Size: func() (int, int, bool) { return 100, 30, true }})
	if width != 42 || height != 12 {
		t.Errorf("a caller that asked for forty-two by twelve got %d by %d", width, height)
	}
}

func TestRunAsksTheTerminalHowBigItIsBeforeItDrawsAnything(t *testing.T) {
	asked := make(chan struct{}, 1)
	stopped := make(chan error, 1)
	go func() {
		stopped <- Run(Options{
			Environment: plainEnvironment,
			Input:       strings.NewReader("\x03\x03"),
			Output:      &bytes.Buffer{},
			Size: func() (int, int, bool) {
				select {
				case asked <- struct{}{}:
				default:
				}
				return 100, 30, true
			},
		})
	}()

	select {
	case <-asked:
	case <-time.After(30 * time.Second):
		t.Fatal("Run never asked the terminal how big it is, so the first frame is drawn at the fallback size")
	}
	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("Run stopped with %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Run did not stop after two Ctrl+C presses")
	}
}

// keptOrDropped says whether a row keeps a right-hand piece properly: either the
// piece was dropped because it did not fit, or there is a real gap in front of
// it. A piece glued straight onto the words to its left is the bug.
func keptOrDropped(line string, piece string) bool {
	at := strings.Index(line, piece)
	if at < 0 {
		return true
	}
	return at >= 2 && line[at-2:at] == "  "
}

func TestTheHeaderAndTheStripDropTheirRightSideRatherThanGluingItOn(t *testing.T) {
	for _, width := range []int{60, 62, 64, 66, 68, 70, 72, 76, 80} {
		screen, _ := newTestScreen(width, 24)
		screen.Update(linkMessage{up: true})
		send(screen, aFullStatus())

		if !keptOrDropped(headerOf(screen), string(filledDotGlyph)) {
			t.Errorf("at %d columns the header is %q, and the health dot is glued onto the words beside it", width, headerOf(screen))
		}
		if !keptOrDropped(statusStrip(screen), "Enter") {
			t.Errorf("at %d columns the status strip is %q, and the key hints are glued onto the words beside them", width, statusStrip(screen))
		}
	}
}

func TestTheNarrowFramesAreDrawnAsTheGoldenFilesHaveThem(t *testing.T) {
	for _, width := range []int{60, 68} {
		screen, _ := newTestScreen(width, 24)
		screen.link = &recordingLink{}
		screen.Update(linkMessage{up: true})
		send(screen, aFullStatus())
		aTalkedTranscript(screen)
		testkit.Golden(t, "narrow-"+strconv.Itoa(width)+"-frame.txt", []byte(screen.frame()))
	}
}

func TestTheDesignsOwnNumbersAreWhatTheDesignSays(t *testing.T) {
	for name, measured := range map[string]struct {
		got    time.Duration
		wanted time.Duration
	}{
		"the heartbeat":     {heartbeatInterval, 30 * time.Millisecond},
		"the spinner rate":  {spinnerRate, 125 * time.Millisecond},
		"the spinner delay": {spinnerDelay, 500 * time.Millisecond},
		"the spinner hold":  {spinnerHold, 3 * time.Second},
		"the health window": {healthFreshFor, 10 * time.Second},
	} {
		if measured.got != measured.wanted {
			t.Errorf("%s is %s, and docs/TUI_DESIGN.md says %s", name, measured.got, measured.wanted)
		}
	}
	for name, measured := range map[string]struct {
		got    int
		wanted int
	}{
		"the widest a transcript wraps": {widestTranscript, 100},
		"the tallest the input box is":  {maxInputRows, 5},
		"the width the design draws at": {narrowWidth, 60},
	} {
		if measured.got != measured.wanted {
			t.Errorf("%s is %d, and docs/TUI_DESIGN.md says %d", name, measured.got, measured.wanted)
		}
	}
}

func TestAHandoffPictureIsReadOffTheDiskOnlyOnce(t *testing.T) {
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

	if err := os.Remove(path); err != nil {
		t.Fatalf("cannot take the test picture away again: %v", err)
	}
	if !strings.Contains(screen.frame(), "\x1b_G") {
		t.Error("the picture is read off the disk on every frame, and it is encoded once when the card is made")
	}
}

func TestAReplyThatCarriesAFileShowsWhereTheFileIs(t *testing.T) {
	screen, clock := newTestScreen(80, 24)
	screen.link = &recordingLink{}
	send(screen, contract.SocketEnvelope{
		Type:        contract.SocketReply,
		Text:        "here is the report",
		Attachments: []string{"/tmp/coeus-report.pdf"},
	})
	advance(screen, clock, heartbeatInterval)

	if !strings.Contains(screen.frame(), "/tmp/coeus-report.pdf") {
		t.Errorf("the frame does not say where the file is:\n%s", screen.frame())
	}
}

func TestADroppedLinkKeepsTheModelTheTaskAndTheCostInTheHeader(t *testing.T) {
	screen, _ := newTestScreen(120, 24)
	screen.link = &recordingLink{}
	screen.Update(linkMessage{up: true})
	send(screen, aFullStatus())
	screen.Update(linkMessage{up: false, detail: "the socket closed"})

	header := headerOf(screen)
	for _, wanted := range []string{"opus", "task 17", "$0.04", "offline"} {
		if !strings.Contains(header, wanted) {
			t.Errorf("the header is %q after the link dropped, and it should still hold %q", header, wanted)
		}
	}
}

func TestEscapeAtAPreviewWithdrawsFromItByName(t *testing.T) {
	screen, link := screenWithLink()
	send(screen, aPreview())
	pressKey(screen, tea.KeyEsc)

	if len(link.sent) != 1 || link.sent[0].Type != contract.SocketCancel || link.sent[0].ID != "3" {
		t.Fatalf("Escape at a preview sent %+v, and it withdraws from preview 3", link.sent)
	}
	typeWord(screen, "hello")
	if screen.input.text() != "hello" {
		t.Errorf("the input box holds %q after the preview was withdrawn from", screen.input.text())
	}
}
