// What the second trial found, in the frame the orchestrator captured by
// running the real screen on a pseudo-terminal at a hundred and twenty columns
// by thirty-six against a running serve: a person's bubble drawn in pieces, a
// new pill on every heartbeat for a call that had not changed, two arrows on
// every tool line, and a task said as a bare number. Each test here drives the
// screen through the same envelopes that serve sends.
package tui

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/JaredTate/coeus/internal/contract"
)

// theHaikuMessage is what the person typed in the trial, kept whole because its
// length is what put the bubble against the right-hand edge of the frame.
const theHaikuMessage = "Write a haiku about the sea, then write it to a file called haiku.txt in the working folder."

// aTrialScreen is the screen the trial ran: a hundred and twenty columns by
// thirty-six, attached to a program that has reported itself.
func aTrialScreen() *Screen {
	screen := New(Options{
		Clock:       testkitClock(),
		Environment: plainEnvironment,
		Width:       120,
		Height:      36,
	})
	screen.link = &recordingLink{}
	screen.Update(linkMessage{up: true})
	return screen
}

// aToolLine is a status carrying the line for the call in flight, which the
// program sends again on every heartbeat until the call changes.
func aToolLine(line string) contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldToolLine: line,
	}}
}

// typeAndSend puts words in the input box and presses Enter, which is how the
// person's own message reaches the transcript.
func typeAndSend(screen *Screen, text string) {
	screen.input.setText(text)
	pressKey(screen, tea.KeyEnter)
}

// boxesLeftOpen counts the rounded boxes on a frame that were drawn without
// their other half: a lid with no box under it, or a box with no lid. Both are
// what a person sees when a bubble is cut in two.
func boxesLeftOpen(frame string) int {
	open, unopened := 0, 0
	for _, line := range strings.Split(plainText(frame), "\n") {
		switch {
		case strings.Contains(line, "╭"):
			open++
		case strings.Contains(line, "╰"):
			if open == 0 {
				unopened++
				continue
			}
			open--
		}
	}
	return open + unopened
}

func TestThePersonsBubbleIsDrawnWholeOrNotAtAll(t *testing.T) {
	// The transcript fills up as the pills arrive, and every count from a
	// transcript with room to spare to one long past full has to leave whole
	// bubbles, because the row the view starts at falls in a different place in
	// each of them.
	for pills := 20; pills <= 34; pills++ {
		screen := aTrialScreen()
		typeAndSend(screen, theHaikuMessage)
		for at := range pills {
			send(screen, aToolLine("▸ web DigiByte cryptocurrency blockchain overview · r"+strconv.Itoa(at)))
		}

		frame := screen.frame()
		if open := boxesLeftOpen(frame); open != 0 {
			t.Fatalf("with %d pills the frame draws %d box or boxes cut in two, and half a bubble is worse than none:\n%s",
				pills, open, frame)
		}
		if strings.Contains(plainText(frame), string(personBarGlyph)) && !strings.Contains(plainText(frame), "haiku.txt") {
			t.Fatalf("with %d pills the person's bubble is drawn with none of the person's words in it:\n%s", pills, frame)
		}
	}
}

func TestThePersonsBubbleLeansRightAndIsOnlyAsWideAsItsWords(t *testing.T) {
	screen := aTrialScreen()
	typeAndSend(screen, theHaikuMessage)

	lid, words, floor := "", "", ""
	for _, line := range strings.Split(plainText(screen.frame()), "\n") {
		switch {
		case strings.Contains(line, "╭"):
			lid = line
		case strings.Contains(line, string(personBarGlyph)):
			words = line
		case strings.Contains(line, "╰"):
			floor = line
		}
	}
	if lid == "" || words == "" || floor == "" {
		t.Fatalf("the bubble is missing one of its three rows:\n%s", screen.frame())
	}
	if !strings.Contains(words, theHaikuMessage) {
		t.Errorf("the bubble's one row is %q, and it should hold the whole message", words)
	}

	left := displayWidth(lid) - displayWidth(strings.TrimLeft(lid, " "))
	right := screen.width - displayWidth(strings.TrimRight(lid, " "))
	if left <= right {
		t.Errorf("the bubble has %d columns to its left and %d to its right, and the person's bubble leans against the right-hand edge",
			left, right)
	}
	if wide := displayWidth(strings.TrimRight(lid, " ")) - left; wide != displayWidth(theHaikuMessage)+bubbleFrame {
		t.Errorf("the bubble is %d columns wide for %d columns of words, and it is only as wide as its words",
			wide, displayWidth(theHaikuMessage))
	}
}

func TestAToolLineThatArrivesAgainOnEveryHeartbeatDrawsOnePill(t *testing.T) {
	screen := aTrialScreen()
	for range 8 {
		send(screen, aToolLine("▸ web DigiByte cryptocurrency blockchain overview"))
	}

	if said := pillTexts(screen); len(said) != 1 {
		t.Errorf("the same tool line on eight heartbeats drew the pills %q, and one call is one pill", said)
	}
}

func TestTheResultOfACallReplacesThePillTheCallAlreadyHas(t *testing.T) {
	screen := aTrialScreen()
	send(screen, aToolLine("▸ read note.txt"))
	send(screen, aToolLine("▸ read note.txt"))
	send(screen, aToolLine("▸ read note.txt · r1 read: 1 line"))

	said := pillTexts(screen)
	if len(said) != 1 {
		t.Fatalf("a call and its result drew the pills %q, and the result replaces the pill the call already has", said)
	}
	if said[0] != "▸ read note.txt · r1 read: 1 line" {
		t.Errorf("the pill says %q, and it should say the call and what came back of it", said[0])
	}
}

func TestEachCallStillGetsAPillOfItsOwn(t *testing.T) {
	screen := aTrialScreen()
	send(screen, aToolLine("▸ read note.txt"))
	send(screen, aToolLine("▸ read note.txt · r1 read: 1 line"))
	send(screen, aToolLine("▸ web DigiByte"))
	send(screen, aToolLine("▸ web DigiByte · r2 web: 3 results"))

	wanted := []string{"▸ read note.txt · r1 read: 1 line", "▸ web DigiByte · r2 web: 3 results"}
	said := pillTexts(screen)
	if len(said) != len(wanted) {
		t.Fatalf("two calls drew the pills %q, and each call gets one: %q", said, wanted)
	}
	for at, one := range wanted {
		if said[at] != one {
			t.Errorf("pill %d says %q, and it should say %q", at+1, said[at], one)
		}
	}
}

func TestAToolLineThatAlreadyCarriesTheArrowIsNotGivenASecondOne(t *testing.T) {
	screen := aTrialScreen()
	send(screen, aToolLine("▸ read note.txt · r1 read: 1 line"))

	drawn := plainText(strings.Join(screen.blockLines(screen.blocks[0]), "\n"))
	if strings.Contains(drawn, string(toolArrowGlyph)+" "+string(toolArrowGlyph)) {
		t.Errorf("the pill is drawn as %q, and the line it was sent already begins with the arrow", drawn)
	}
	if !strings.Contains(drawn, string(toolArrowGlyph)+" read note.txt · r1 read: 1 line") {
		t.Errorf("the pill is drawn as %q, and it should read as one arrow and the line", drawn)
	}
}

func TestTheHeaderSaysTheTaskInWordsWhenTheProgramSendsTheNumberAlone(t *testing.T) {
	screen := aTrialScreen()
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldTask: "24",
	}})

	if header := plainText(headerOf(screen)); !strings.Contains(header, "· task 24 ·") {
		t.Errorf("the header is %q, and the design writes a task the screen knows no state for as \"task 24\"", header)
	}

	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldTaskState: "running",
	}})
	if header := plainText(headerOf(screen)); !strings.Contains(header, "· task 24 running ·") {
		t.Errorf("the header is %q, and the design writes a running task as \"task 24 running\"", header)
	}
}

func TestAProgramThatAlreadySaysTheWordTaskIsNotMadeToSayItTwice(t *testing.T) {
	screen := aTrialScreen()
	send(screen, aFullStatus())

	if header := plainText(headerOf(screen)); !strings.Contains(header, "task 17 running") || strings.Contains(header, "task task") {
		t.Errorf("the header is %q, and a program that already writes \"task 17\" is left as it is", header)
	}
}
