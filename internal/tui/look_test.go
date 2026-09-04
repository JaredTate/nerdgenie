package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// newThemedScreen builds a screen on a terminal that promises twenty-four bit
// colour, which is what the themed golden files are drawn on.
func newThemedScreen(width int, height int) *Screen {
	screen := New(Options{
		Clock:  testkitClock(),
		Width:  width,
		Height: height,
		Environment: func(name string) string {
			return map[string]string{"COLORTERM": "truecolor", "TERM": "xterm-256color"}[name]
		},
	})
	screen.link = &recordingLink{}
	return screen
}

// theWholeConversation fills a screen with one of every block the transcript
// draws, so that a golden file shows the bubbles, the pills and the card at
// once.
func theWholeConversation(screen *Screen) {
	screen.Update(linkMessage{up: true})
	send(screen, aFullStatus())
	screen.remember(block{kind: blockPerson, text: "Post a tweet about the DigiByte anniversary. Use the product notes and keep it under 280 characters."})
	screen.remember(block{kind: blockReply, text: "Where I stand: the notes are read, drafting next."})
	screen.remember(block{kind: blockTool, text: "read memory/product.md · 2,100 characters · r3"})
	send(screen, contract.SocketEnvelope{Type: contract.SocketPreview, ID: "3", Text: "browser_click e7 \"Post\""})
}

func TestThePersonsMessageLeansRightAndTheAgentsReplyLeansLeft(t *testing.T) {
	screen, _ := newTestScreen(80, 24)
	screen.remember(block{kind: blockPerson, text: "post it"})
	screen.remember(block{kind: blockReply, text: "posting it now"})

	mine := screen.blockLines(screen.blocks[0])
	theirs := screen.blockLines(screen.blocks[1])

	if leading := len(mine[0]) - len(strings.TrimLeft(mine[0], " ")); leading <= marginColumns {
		t.Errorf("the person's bubble starts %d columns in, and it leans to the right", leading)
	}
	if leading := len(theirs[0]) - len(strings.TrimLeft(theirs[0], " ")); leading != marginColumns {
		t.Errorf("the agent's bubble starts %d columns in, and it leans to the left", leading)
	}
	for name, drawn := range map[string][]string{"the person's bubble": mine, "the agent's bubble": theirs} {
		if !strings.Contains(drawn[0], "╭") || !strings.Contains(drawn[len(drawn)-1], "╯") {
			t.Errorf("%s has no rounded corners: %q", name, drawn)
		}
	}
	if !strings.Contains(strings.Join(mine, "\n"), string(personBarGlyph)) {
		t.Errorf("the person's bubble has lost its thick left edge: %q", mine)
	}
}

func TestATallMessageAndAShortOneGetBubblesOfTheirOwnWidth(t *testing.T) {
	screen, _ := newTestScreen(80, 24)
	screen.remember(block{kind: blockPerson, text: "hi"})
	screen.remember(block{kind: blockPerson, text: strings.Repeat("a long message ", 20)})

	short := boxWidth(screen.blockLines(screen.blocks[0])[0])
	long := boxWidth(screen.blockLines(screen.blocks[1])[0])
	if short >= long {
		t.Errorf("a two-letter message drew a bubble %d columns wide and a long one drew %d", short, long)
	}
	for _, drawn := range append(screen.blockLines(screen.blocks[0]), screen.blockLines(screen.blocks[1])...) {
		if displayWidth(drawn) > 80 {
			t.Errorf("the bubble row %q is wider than the terminal", drawn)
		}
	}
}

// boxWidth is how wide a bubble's own box is, leaving out the blanks that push a
// right-leaning bubble over to the right-hand edge.
func boxWidth(drawn string) int {
	return displayWidth(strings.TrimLeft(drawn, " "))
}

func TestATheToolLineIsDrawnAsASmallPill(t *testing.T) {
	screen := newThemedScreen(80, 24)
	screen.remember(block{kind: blockTool, text: "read memory/product.md · r3"})

	drawn := screen.blockLines(screen.blocks[0])
	if len(drawn) != 1 {
		t.Fatalf("one tool call drew %d rows, and it is one pill", len(drawn))
	}
	if !strings.Contains(drawn[0], backgroundOf(accentTone, depthTruecolor)) {
		t.Errorf("the tool pill is not drawn on the accent: %q", drawn[0])
	}
	if !strings.Contains(plainText(drawn[0]), string(toolArrowGlyph)+" read memory/product.md · r3") {
		t.Errorf("the tool pill does not read as one line: %q", plainText(drawn[0]))
	}
}

func TestTheThreeAnswersOnAPreviewAreDrawnAsButtons(t *testing.T) {
	screen := newThemedScreen(80, 24)
	send(screen, aPreview())

	frame := screen.frame()
	for _, wanted := range []string{"[ a ] approve once", "[ A ] always this session", "[ r ] reject with a reason"} {
		if !strings.Contains(plainText(frame), wanted) {
			t.Errorf("the preview card does not draw %q as a button:\n%s", wanted, plainText(frame))
		}
	}
	if !strings.Contains(frame, screen.colors.wrap(styleAccent, "┌ ")+screen.colors.wrap(styleAccent, previewTitle)) {
		t.Error("the preview card's border and title are not in the accent, and it is the one card the person must answer")
	}
}

func TestTheStatusStripDrawsHowMuchOfTheBudgetIsLeft(t *testing.T) {
	screen, _ := newTestScreen(80, 24)
	screen.Update(linkMessage{up: true})
	send(screen, aFullStatus())

	full := statusStrip(screen)
	if strings.Count(full, string(barFullGlyph)) != progressCells {
		t.Errorf("the first budget report drew %q, and the bar starts full", full)
	}

	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldBudget: "43 rounds, 20 min left",
	}})
	half := statusStrip(screen)
	if strings.Count(half, string(barFullGlyph)) != progressCells/2 {
		t.Errorf("half the budget drew %q, and half the bar should be full", half)
	}
	if strings.Count(half, string(barEmptyGlyph)) != progressCells-progressCells/2 {
		t.Errorf("half the budget drew %q, and the rest of the bar should be empty", half)
	}
}

func TestTheBudgetBarStartsAgainWithEveryTask(t *testing.T) {
	screen, _ := newTestScreen(80, 24)
	screen.Update(linkMessage{up: true})
	send(screen, aFullStatus())
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldBudget: "43 rounds, 20 min left",
	}})
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldTask:   "task 18",
		contract.StatusFieldBudget: "20 rounds, 60 min left",
	}})

	if count := strings.Count(statusStrip(screen), string(barFullGlyph)); count != progressCells {
		t.Errorf("the new task drew %d filled cells, and a task's first report is its full bar", count)
	}
}

func TestTheHealthDotIsAFilledCircleWhileTheProgramIsAnswering(t *testing.T) {
	screen := newThemedScreen(120, 24)
	screen.Update(linkMessage{up: true})
	send(screen, aFullStatus())

	header := headerOf(screen)
	if !strings.Contains(plainText(header), string(filledDotGlyph)+" healthy") {
		t.Errorf("the header is %q, and a program that is answering shows a filled circle", plainText(header))
	}
	if !strings.Contains(header, foregroundOf(accentTone, depthTruecolor)) {
		t.Error("the health dot is not drawn in the accent")
	}
}

func TestTheThemedAndThePlainFramesAreDrawnAsTheGoldenFilesHaveThem(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}} {
		name := strconv.Itoa(size[0]) + "x" + strconv.Itoa(size[1])

		plainBanner, _ := newTestScreen(size[0], size[1])
		testkit.Golden(t, "banner-"+name+".txt", []byte(plainBanner.frame()))
		testkit.Golden(t, "themed-banner-"+name+".txt", []byte(newThemedScreen(size[0], size[1]).frame()))

		plainTalk, _ := newTestScreen(size[0], size[1])
		plainTalk.link = &recordingLink{}
		theWholeConversation(plainTalk)
		testkit.Golden(t, "conversation-"+name+".txt", []byte(plainTalk.frame()))

		themedTalk := newThemedScreen(size[0], size[1])
		theWholeConversation(themedTalk)
		testkit.Golden(t, "themed-conversation-"+name+".txt", []byte(themedTalk.frame()))
	}
}

func TestEveryRowOfAThemedFrameIsPaintedRightToTheEdge(t *testing.T) {
	screen := newThemedScreen(80, 24)
	theWholeConversation(screen)

	for number, line := range strings.Split(screen.frame(), "\n") {
		if width := displayWidth(plainText(line)); width != 80 {
			t.Errorf("row %d is %d columns of paint and the terminal is 80: %q", number+1, width, plainText(line))
		}
		if !strings.Contains(line, "\x1b[48;") {
			t.Errorf("row %d has no background paint at all, so it would show through as a dark gap: %q", number+1, line)
		}
	}
}

func TestAnEscapeCharacterFromEitherSideNeverReachesTheTerminal(t *testing.T) {
	screen, _ := newTestScreen(80, 24)
	screen.link = &recordingLink{}
	screen.remember(block{kind: blockPerson, text: "before\x1b[2Jafter"})
	send(screen, contract.SocketEnvelope{Type: contract.SocketReply, Text: "a reply\x07with a bell"})
	screen.input.setText("typed\x1b]0;a new title\x07")

	frame := screen.frame()
	if strings.Contains(plainText(frame), "\x1b") || strings.Contains(frame, "\x07") {
		t.Errorf("a control character the screen was handed reached the frame:\n%q", frame)
	}
	for number, line := range strings.Split(frame, "\n") {
		if width := displayWidth(plainText(line)); width != 80 {
			t.Errorf("row %d measures %d columns and the terminal is 80: %q", number+1, width, line)
		}
	}
}
