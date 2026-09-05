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

	"github.com/JaredTate/nerdgenie/internal/contract"
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

// cardRowsOf are the rows of a frame that carry a card's bar, cut to the
// transcript's columns, which is where the person's message and the agent's
// replies are drawn.
func cardRowsOf(screen *Screen) []string {
	rows := []string{}
	for _, line := range strings.Split(plainText(screen.frame()), "\n") {
		beside := string([]rune(line)[:screen.transcriptColumns()])
		if strings.Contains(beside, string(cardBarGlyph)) {
			rows = append(rows, beside)
		}
	}
	return rows
}

func TestThePersonsCardIsDrawnWholeOrNotAtAll(t *testing.T) {
	// The transcript fills up as the pills arrive, and every count from a
	// transcript with room to spare to one long past full has to leave the
	// card whole or leave it out, because the row the view starts at falls in
	// a different place in each of them. A flat card has no lid to catch, so
	// the proof is that every row with the bar is a row of the message and
	// that together they are the whole message.
	for pills := 20; pills <= 34; pills++ {
		screen := aTrialScreen()
		typeAndSend(screen, theHaikuMessage)
		for at := range pills {
			send(screen, aToolLine("▸ web DigiByte cryptocurrency blockchain overview · r"+strconv.Itoa(at)))
		}

		said := []string{}
		for _, drawn := range cardRowsOf(screen) {
			said = append(said, insideTheCard(drawn))
		}
		if joined := strings.Join(said, " "); len(said) > 0 && joined != theHaikuMessage {
			t.Fatalf("with %d pills the person's card holds %q, and it is drawn whole or not at all:\n%s", pills, joined, screen.frame())
		}
	}
}

func TestAReplyTallerThanTheWholeTranscriptIsStillShown(t *testing.T) {
	screen := aTrialScreen()
	typeAndSend(screen, theHaikuMessage)
	send(screen, contract.SocketEnvelope{Type: contract.SocketReply,
		Text: strings.Repeat("Salt on the wind, and the tide comes in again. ", 100)})
	send(screen, aToolLine("▸ task 1 done · Write a haiku about the sea"))

	frame := plainText(screen.frame())
	if !strings.Contains(frame, "Salt on the wind") {
		t.Errorf("a reply taller than the transcript is not on the frame at all, and its newest rows are what is being read:\n%s", frame)
	}
	if !strings.Contains(frame, "task 1 done") {
		t.Errorf("the pill after the long reply is not on the frame, and the newest thing is always at the bottom:\n%s", frame)
	}
}

func TestThePersonsCardLeansRightAndIsOnlyAsWideAsItsWords(t *testing.T) {
	screen := aTrialScreen()
	typeAndSend(screen, theHaikuMessage)

	rows := cardRowsOf(screen)
	if len(rows) == 0 {
		t.Fatalf("the card is not on the frame:\n%s", screen.frame())
	}
	said := []string{}
	for _, drawn := range rows {
		said = append(said, insideTheCard(drawn))
	}
	if joined := strings.Join(said, " "); joined != theHaikuMessage {
		t.Errorf("the card holds %q, and it should hold the whole message", joined)
	}

	first := rows[0]
	left := displayWidth(first) - displayWidth(strings.TrimLeft(first, " "))
	right := screen.transcriptColumns() - displayWidth(strings.TrimRight(first, " "))
	if left <= right {
		t.Errorf("the card has %d columns to its left and %d to its right, and the person's card leans against the right-hand edge",
			left, right)
	}
	if right != marginColumns {
		t.Errorf("the card stops %d columns short of the transcript's right-hand edge, and the design leaves it the one blank margin", right)
	}
	widest := 0
	for _, one := range said {
		widest = max(widest, displayWidth(one))
	}
	if wide := displayWidth(strings.TrimRight(first, " ")) - left; wide != widest+bubbleFrame {
		t.Errorf("the card is %d columns wide for %d columns of words, and it is only as wide as its words", wide, widest)
	}
}

// insideTheCard is the words on one row of a card, with the bar and the
// padding taken off.
func insideTheCard(drawn string) string {
	_, words, _ := strings.Cut(drawn, string(cardBarGlyph))
	return strings.TrimSpace(words)
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
	if strings.Contains(drawn, string(toolArrowGlyph)) {
		t.Errorf("the pill is drawn as %q, and the arrow the program wrote is taken off, since the state glyph stands in its place", drawn)
	}
	if !strings.Contains(drawn, string(doneGlyph)+"  read note.txt · read: 1 line") || !strings.HasSuffix(drawn, " r1 ") {
		t.Errorf("the pill is drawn as %q, and it should read as the glyph, the line, and the result id as a badge", drawn)
	}
}

func TestTheHeaderSaysTheTaskInWordsWhenTheProgramSendsTheNumberAlone(t *testing.T) {
	screen := aTrialScreen()
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldTask: "24",
	}})

	if header := plainText(headerOf(screen)); !strings.Contains(header, "· task 24") {
		t.Errorf("the header is %q, and the design writes a task the screen knows no state for as \"task 24\"", header)
	}

	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldTaskState: "running",
	}})
	if header := plainText(headerOf(screen)); !strings.Contains(header, "· task 24 running") {
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
