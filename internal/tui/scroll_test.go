// What the first human trial found on the scroll: the person turned the mouse
// wheel over the transcript and nothing moved, because the screen had never
// asked the terminal to report the wheel. Each test here drives the screen
// through the wheel and the keys that scroll, on the frames it draws, and the
// last one drives the whole program through Run the way cmd/coeus does.
package tui

import (
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// olderMark is what the status strip says while the person is scrolled up, so
// that they know why new text is not appearing.
const olderMark = "↑ older"

// repliesInALongTalk is how many replies fill the transcript past the nineteen
// rows an eighty by twenty-four terminal has for it.
const repliesInALongTalk = 12

// theCountingMessage is what the person asks for in the long conversation.
const theCountingMessage = "Count to twelve for me, one reply at a time."

// aLongConversation fills an attached screen with one message from the person
// and twelve numbered replies, which is more rows than the transcript has room
// for.
func aLongConversation(screen *Screen) {
	screen.Update(linkMessage{up: true})
	typeAndSend(screen, theCountingMessage)
	for number := 1; number <= repliesInALongTalk; number++ {
		send(screen, aReply("reply "+strconv.Itoa(number)))
	}
}

// aReply is one finished reply from the program.
func aReply(text string) contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketReply, Text: text}
}

// wheel turns the mouse wheel over the screen one notch.
func wheel(screen *Screen, button tea.MouseButton) {
	screen.Update(tea.MouseWheelMsg{Button: button})
}

// wheelTimes turns the wheel a number of notches.
func wheelTimes(screen *Screen, button tea.MouseButton, notches int) {
	for range notches {
		wheel(screen, button)
	}
}

// pressWithShift gives the screen one named key with the shift key held.
func pressWithShift(screen *Screen, which rune) {
	screen.Update(tea.KeyPressMsg{Code: which, Mod: tea.ModShift})
}

// transcriptOnTheFrame is the rows of the frame between the two rules, which is
// the part that scrolls, with the paint out to the right-hand edge taken off.
func transcriptOnTheFrame(screen *Screen) []string {
	rows := strings.Split(plainText(screen.frame()), "\n")
	return trimmedRight(rows[2 : len(rows)-2-len(screen.inputRows())])
}

// everyTranscriptRow is the whole transcript as rows, oldest first.
func everyTranscriptRow(screen *Screen) []string {
	return trimmedRight(screen.transcriptRows(1 << 20))
}

// trimmedRight takes the blanks off the end of every row.
func trimmedRight(rows []string) []string {
	trimmed := make([]string, len(rows))
	for at, drawn := range rows {
		trimmed[at] = strings.TrimRight(drawn, " ")
	}
	return trimmed
}

// olderRows is the view when it is scrolled a number of rows up from the newest
// row: as many rows as the transcript has room for, ending that many rows above
// the end.
func olderRows(every []string, height int, above int) []string {
	return every[len(every)-height-above : len(every)-above]
}

// expectRows fails unless the rows shown are the rows wanted.
func expectRows(t *testing.T, what string, shown []string, wanted []string) {
	t.Helper()
	if !slices.Equal(shown, wanted) {
		t.Errorf("%s, and the transcript shows:\n%s\n--- rather than ---\n%s",
			what, strings.Join(shown, "\n"), strings.Join(wanted, "\n"))
	}
}

// expectMark fails unless the status strip carries the older mark, or does not,
// as the test says it should.
func expectMark(t *testing.T, screen *Screen, shown bool, when string) {
	t.Helper()
	if strings.Contains(statusStrip(screen), olderMark) != shown {
		t.Errorf("the status strip is %q %s, and the %q mark is for a view scrolled up and nothing else",
			statusStrip(screen), when, olderMark)
	}
}

func TestAWheelUpShowsThreeOlderRowsAndAWheelDownThreeNewer(t *testing.T) {
	screen, _ := screenWithLink()
	aLongConversation(screen)
	every := everyTranscriptRow(screen)
	atTheBottom := transcriptOnTheFrame(screen)
	height := len(atTheBottom)

	wheel(screen, tea.MouseWheelUp)
	expectRows(t, "one wheel up shows the view three rows older", transcriptOnTheFrame(screen), olderRows(every, height, 3))
	expectMark(t, screen, true, "after one wheel up")

	wheel(screen, tea.MouseWheelDown)
	expectRows(t, "one wheel down brings the view back to the newest", transcriptOnTheFrame(screen), atTheBottom)
	expectMark(t, screen, false, "back at the bottom")
}

func TestScrollingStopsAtTheOldestRowAndAtTheNewest(t *testing.T) {
	screen, _ := screenWithLink()
	aLongConversation(screen)
	every := everyTranscriptRow(screen)
	atTheBottom := transcriptOnTheFrame(screen)
	height := len(atTheBottom)

	wheelTimes(screen, tea.MouseWheelUp, 100)
	expectRows(t, "a hundred wheel ups stop at the oldest row", transcriptOnTheFrame(screen), every[:height])
	wheel(screen, tea.MouseWheelUp)
	expectRows(t, "one more wheel up at the top changes nothing", transcriptOnTheFrame(screen), every[:height])

	wheelTimes(screen, tea.MouseWheelDown, 100)
	expectRows(t, "a hundred wheel downs stop at the newest row", transcriptOnTheFrame(screen), atTheBottom)
	wheel(screen, tea.MouseWheelDown)
	expectRows(t, "one more wheel down at the bottom changes nothing", transcriptOnTheFrame(screen), atTheBottom)
	expectMark(t, screen, false, "back at the bottom")
}

func TestAConversationThatFitsDoesNotScrollAtAll(t *testing.T) {
	screen, _ := screenWithLink()
	screen.Update(linkMessage{up: true})
	typeAndSend(screen, "hello")
	send(screen, aReply("hello to you"))
	before := screen.frame()

	wheelTimes(screen, tea.MouseWheelUp, 3)
	if screen.frame() != before {
		t.Errorf("a wheel up over a transcript that fits changed the frame:\n%s", plainText(screen.frame()))
	}
	expectMark(t, screen, false, "with nothing older to show")
}

func TestNewOutputWhileScrolledUpLeavesTheViewWhereItIs(t *testing.T) {
	screen, clock := newTestScreen(80, 24)
	screen.link = &recordingLink{}
	aLongConversation(screen)
	wheelTimes(screen, tea.MouseWheelUp, 2)
	held := transcriptOnTheFrame(screen)

	send(screen, aReply("reply 13"))
	expectRows(t, "a finished reply arrived while scrolled up", transcriptOnTheFrame(screen), held)

	send(screen, contract.SocketEnvelope{Type: contract.SocketDelta, Text: "reply 14 is on"})
	advance(screen, clock, heartbeatInterval)
	expectRows(t, "a streamed reply began while scrolled up", transcriptOnTheFrame(screen), held)

	send(screen, contract.SocketEnvelope{Type: contract.SocketDelta, Text: " its way\nover two lines"})
	advance(screen, clock, heartbeatInterval)
	expectRows(t, "a streamed reply grew while scrolled up", transcriptOnTheFrame(screen), held)

	send(screen, aToolLine("▸ read notes.md · r9 read: 12 lines"))
	expectRows(t, "a tool line arrived while scrolled up", transcriptOnTheFrame(screen), held)
	expectMark(t, screen, true, "while new output arrives above a view scrolled up")
}

func TestAWheelDownToTheBottomSnapsTheViewBackToTheNewest(t *testing.T) {
	screen, _ := screenWithLink()
	aLongConversation(screen)
	wheelTimes(screen, tea.MouseWheelUp, 2)
	send(screen, aReply("reply 13"))
	if strings.Contains(plainText(screen.frame()), "reply 13") {
		t.Fatal("the new reply is on the frame while the view is scrolled up, so the view was yanked down")
	}

	// The reply took three rows, so the view is nine rows up and three notches
	// bring it to the bottom.
	wheelTimes(screen, tea.MouseWheelDown, 3)
	if !strings.Contains(plainText(screen.frame()), "reply 13") {
		t.Errorf("the newest reply is not on the frame after wheeling down to the bottom:\n%s", plainText(screen.frame()))
	}
	expectMark(t, screen, false, "after wheeling down to the bottom")

	fresh, _ := screenWithLink()
	aLongConversation(fresh)
	send(fresh, aReply("reply 13"))
	expectRows(t, "wheeling down to the bottom shows the newest rows", transcriptOnTheFrame(screen), transcriptOnTheFrame(fresh))
}

func TestAMessageThePersonSendsSnapsTheViewBackToTheNewest(t *testing.T) {
	screen, _ := screenWithLink()
	aLongConversation(screen)
	wheelTimes(screen, tea.MouseWheelUp, 3)
	typeAndSend(screen, "and then?")

	if !strings.Contains(plainText(screen.frame()), "and then?") {
		t.Errorf("the message the person just sent is not on the frame:\n%s", plainText(screen.frame()))
	}
	expectMark(t, screen, false, "after the person sent a message")
}

func TestPageUpPageDownShiftUpAndShiftDownScrollByTheKeyboard(t *testing.T) {
	screen, _ := screenWithLink()
	aLongConversation(screen)
	every := everyTranscriptRow(screen)
	atTheBottom := transcriptOnTheFrame(screen)
	height := len(atTheBottom)

	pressKey(screen, tea.KeyPgUp)
	expectRows(t, "Page Up shows the view ten rows older", transcriptOnTheFrame(screen), olderRows(every, height, scrollRows))
	expectMark(t, screen, true, "after Page Up")
	pressKey(screen, tea.KeyPgDown)
	expectRows(t, "Page Down brings the view back", transcriptOnTheFrame(screen), atTheBottom)

	pressWithShift(screen, tea.KeyUp)
	expectRows(t, "Shift+Up shows the view three rows older, as one notch of the wheel does", transcriptOnTheFrame(screen), olderRows(every, height, 3))
	pressWithShift(screen, tea.KeyDown)
	expectRows(t, "Shift+Down brings the view back", transcriptOnTheFrame(screen), atTheBottom)
	expectMark(t, screen, false, "after Shift+Down")

	pressKey(screen, tea.KeyUp)
	if screen.input.text() != theCountingMessage {
		t.Errorf("a plain Up put %q in the box, and Up without shift still recalls what was sent", screen.input.text())
	}
	expectMark(t, screen, false, "after a plain Up, which belongs to the history")
}

func TestTheScrollKeysStillWorkWhileACardHoldsTheKeys(t *testing.T) {
	screen, link := screenWithLink()
	aLongConversation(screen)
	send(screen, aPreview())

	pressWithShift(screen, tea.KeyUp)
	expectMark(t, screen, true, "after Shift+Up while a card waits")
	press(screen, 'a')
	if last := link.sent[len(link.sent)-1]; last.Type != contract.SocketApprove {
		t.Errorf("the card lost its answer key after a scroll; the last thing sent was %+v", last)
	}
}

func TestTheScrolledFrameIsDrawnAsTheGoldenFileHasIt(t *testing.T) {
	screen, _ := screenWithLink()
	aLongConversation(screen)
	wheelTimes(screen, tea.MouseWheelUp, 2)
	testkit.Golden(t, "scrolled-up-80x24.txt", []byte(screen.frame()))
}

// wheelUpBytes is what a terminal sends for one notch of the wheel turned up
// over column ten, row five, once it has been asked to report the mouse.
const wheelUpBytes = "\x1b[<64;10;5M"

// The codes that turn mouse reporting on and off: cell-motion reporting, which
// is what makes a terminal send the wheel, and the extended coordinates that
// come with it.
const (
	mouseReportingOn  = "\x1b[?1002h"
	mouseReportingOff = "\x1b[?1002l"
	wideMouseOff      = "\x1b[?1006l"
)

// wholeProgramLimit is how long the whole program is given to draw a thing
// before the test gives up, which is generous because the suite runs in
// parallel with everything else under make check.
const wholeProgramLimit = 5 * time.Second

// terminalCodes matches every control sequence the terminal reads and does not
// draw, so that what the whole program painted can be searched for words.
var terminalCodes = regexp.MustCompile("\x1b\\[[0-9;?<>=]*[A-Za-z@`]|\x1b\\][^\x07]*\x07")

// olderMarkPainted matches the mark however the renderer painted it: a renderer
// that repaints only the cells that changed may step over the blank between the
// arrow and the word.
var olderMarkPainted = regexp.MustCompile("↑ ?older")

// wholeProgram is the screen running through Run on a pair of pipes, the way
// cmd/coeus runs it on a terminal.
type wholeProgram struct {
	keyboard *os.File
	drawn    *frameSoFar
	stopped  chan error
}

// startTheWholeProgram runs the screen through Run with a fake link to a
// program, and closes the pipes when the test ends.
func startTheWholeProgram(t *testing.T, dialer Dialer) *wholeProgram {
	t.Helper()
	keys, keyboard, err := os.Pipe()
	if err != nil {
		t.Fatalf("there is no pipe to type into: %v", err)
	}
	frames, painted, err := os.Pipe()
	if err != nil {
		t.Fatalf("there is no pipe to draw on: %v", err)
	}
	t.Cleanup(func() {
		for _, file := range []*os.File{keyboard, keys, painted, frames} {
			_ = file.Close()
		}
	})
	program := &wholeProgram{keyboard: keyboard, drawn: readWhatIsDrawn(frames), stopped: make(chan error, 1)}
	go func() {
		program.stopped <- Run(Options{
			Environment: plainEnvironment,
			Width:       80,
			Height:      24,
			Input:       keys,
			Output:      painted,
			Dialer:      dialer,
		})
	}()
	return program
}

// typeBytes sends bytes to the screen as a terminal would.
func (program *wholeProgram) typeBytes(t *testing.T, typed string) {
	t.Helper()
	if _, err := program.keyboard.WriteString(typed); err != nil {
		t.Fatalf("typing %q at the screen failed: %v", typed, err)
	}
}

// painted is everything drawn so far with the terminal's codes taken out.
func (program *wholeProgram) painted() string {
	return terminalCodes.ReplaceAllString(program.drawn.text(), "")
}

// wheelUntilScrolled turns the wheel a notch at a time until the older mark is
// on the frame, because a notch turned before the replies are drawn has nothing
// to scroll and is rightly ignored.
func (program *wholeProgram) wheelUntilScrolled(t *testing.T) {
	t.Helper()
	giveUpAt := time.Now().Add(wholeProgramLimit)
	for time.Now().Before(giveUpAt) {
		program.typeBytes(t, wheelUpBytes)
		time.Sleep(lookAgainAfter)
		if olderMarkPainted.MatchString(program.painted()) {
			return
		}
	}
	t.Fatalf("the wheel never scrolled the running screen; the frame so far was:\n%s", program.painted())
}

// quit types Ctrl+C twice, waits for Run to return, and gives back everything
// that was drawn, codes and all, once the last of it has come through the pipe.
func (program *wholeProgram) quit(t *testing.T) string {
	t.Helper()
	program.typeBytes(t, "\x03\x03")
	select {
	case err := <-program.stopped:
		if err != nil {
			t.Fatalf("the screen stopped badly: %v", err)
		}
	case <-time.After(wholeProgramLimit):
		t.Fatal("the screen did not quit on two Ctrl+C presses")
	}
	giveUpAt := time.Now().Add(wholeProgramLimit)
	for time.Now().Before(giveUpAt) {
		if strings.Contains(program.drawn.text(), mouseReportingOff) {
			return program.drawn.text()
		}
		time.Sleep(lookAgainAfter)
	}
	return program.drawn.text()
}

func TestTheWheelScrollsTheWholeProgramAndMouseReportingIsOffWhenItQuits(t *testing.T) {
	dialer := newFakeDialer()
	program := startTheWholeProgram(t, dialer)
	socket := dialer.nextLink(t)
	for number := 1; number <= 8; number++ {
		socket.push(aReply("reply " + strconv.Itoa(number)))
	}
	program.wheelUntilScrolled(t)

	painted := program.quit(t)
	if !strings.Contains(painted, mouseReportingOn) {
		t.Fatal("the screen never asked the terminal to report the mouse, so no real terminal would send the wheel")
	}
	if strings.LastIndex(painted, mouseReportingOff) < strings.LastIndex(painted, mouseReportingOn) {
		t.Error("mouse reporting was still on when the screen quit, which leaves the shell printing mouse codes")
	}
	if !strings.Contains(painted, wideMouseOff) {
		t.Error("the extended mouse coordinates were left on when the screen quit")
	}
}
