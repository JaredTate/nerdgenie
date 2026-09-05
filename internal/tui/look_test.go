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
// draws, so that a golden file shows the two cards, a pill in each of its
// three states, and the preview card at once.
func theWholeConversation(screen *Screen) {
	screen.Update(linkMessage{up: true})
	send(screen, aFullStatus())
	screen.remember(block{kind: blockPerson, text: "Post a tweet about the DigiByte anniversary. Use the product notes and keep it under 280 characters."})
	screen.remember(block{kind: blockReply, text: "Where I stand: the notes are read, drafting next."})
	screen.remember(block{kind: blockTool, text: "read memory/product.md · 2,100 characters · r3"})
	screen.remember(block{kind: blockTool, text: "browser_click e9 \"Delete account\" · r4 the call was refused: not on the allowed list"})
	screen.remember(block{kind: blockTool, text: "▸ browser_open x.com/compose/post"})
	send(screen, contract.SocketEnvelope{Type: contract.SocketPreview, ID: "3", Text: "browser_click e7 \"Post\""})
}

// TestThePersonsMessageLeansRightAndTheAgentsReplyLeansLeft holds the shape of
// the two flat cards: the agent's against the left-hand margin, the person's
// against the right, each row beginning with the bar, and no lid or floor
// above or below the words, so a one-line message is one row.
func TestThePersonsMessageLeansRightAndTheAgentsReplyLeansLeft(t *testing.T) {
	screen, _ := newTestScreen(80, 24)
	screen.remember(block{kind: blockPerson, text: "post it"})
	screen.remember(block{kind: blockReply, text: "posting it now"})

	mine := screen.blockLines(screen.blocks[0])
	theirs := screen.blockLines(screen.blocks[1])
	if len(mine) != 1 || len(theirs) != 1 {
		t.Fatalf("a one-line message drew %d rows and a one-line reply %d, and a flat card has no lid or floor", len(mine), len(theirs))
	}
	if leading := len(mine[0]) - len(strings.TrimLeft(mine[0], " ")); leading <= marginColumns {
		t.Errorf("the person's card starts %d columns in, and it leans to the right", leading)
	}
	if leading := len(theirs[0]) - len(strings.TrimLeft(theirs[0], " ")); leading != marginColumns {
		t.Errorf("the agent's card starts %d columns in, and it leans to the left", leading)
	}
	for name, drawn := range map[string]string{"the person's card": mine[0], "the agent's card": theirs[0]} {
		if !strings.HasPrefix(strings.TrimLeft(drawn, " "), string(cardBarGlyph)+" ") {
			t.Errorf("%s does not begin with the bar and one blank of padding: %q", name, drawn)
		}
		if strings.ContainsAny(drawn, "╭╮╰╯│") {
			t.Errorf("%s still has a border: %q", name, drawn)
		}
	}
	if theirs[0] != " "+string(cardBarGlyph)+" posting it now " {
		t.Errorf("the agent's card reads %q, and it is the bar, a blank, the words and a blank of padding", theirs[0])
	}
}

// TestTheCardsAreDrawnOnTheDeeperNavyWithTheirBarsInTheTwoBlues pins the
// colours of the two cards: the agent's bar in DigiByte's own blue and the
// person's in the light blue, both joined to a card filled with the deeper
// navy, and the padding filled too so the card has a straight edge.
func TestTheCardsAreDrawnOnTheDeeperNavyWithTheirBarsInTheTwoBlues(t *testing.T) {
	screen := newThemedScreen(80, 24)
	screen.remember(block{kind: blockPerson, text: "post it"})
	screen.remember(block{kind: blockReply, text: "posting **it** `now`"})
	colors := screen.colors

	mine := screen.blockLines(screen.blocks[0])[0]
	theirs := screen.blockLines(screen.blocks[1])[0]
	if !strings.Contains(mine, colors.wrap(stylePersonBar, string(cardBarGlyph))) {
		t.Errorf("the person's card %q has no bar in the light blue", mine)
	}
	if !strings.Contains(theirs, colors.wrap(styleBar, string(cardBarGlyph))) {
		t.Errorf("the agent's card %q has no bar in DigiByte's own blue", theirs)
	}
	for name, drawn := range map[string]string{"the person's card": mine, "the agent's card": theirs} {
		if strings.Contains(drawn, backgroundOf(accentTone, depthTruecolor)) {
			t.Errorf("%s is still filled with the accent: %q", name, drawn)
		}
		if !strings.HasSuffix(drawn, colors.wrap(styleCard, " ")) {
			t.Errorf("%s does not end in a blank of padding on the card fill: %q", name, drawn)
		}
	}
	if !strings.Contains(mine, colors.wrap(styleCard, "post it")) {
		t.Errorf("the person's words are not white on the card: %q", mine)
	}
	for _, wanted := range []string{colors.wrap(styleCard, "posting"), colors.wrap(styleKey, " it"), colors.wrap(styleCardDim, " now")} {
		if !strings.Contains(theirs, wanted) {
			t.Errorf("the reply %q does not hold %q, and markdown inside a card is drawn on the card", theirs, wanted)
		}
	}
}

// TestAReplyInParagraphsIsOneRowPerLineWithTheBarOnEveryRow holds that a
// card that grows, which is what a streaming reply does, keeps its shape: the
// bar on every row, and as many rows as there are lines, wrapped at the widest
// the transcript ever wraps.
func TestAReplyInParagraphsIsOneRowPerLineWithTheBarOnEveryRow(t *testing.T) {
	screen, _ := newTestScreen(200, 40)
	screen.remember(block{kind: blockReply, text: "first\nsecond\n" + strings.Repeat("a long third line ", 12)})

	drawn := screen.blockLines(screen.blocks[0])
	if len(drawn) < 4 || !strings.HasPrefix(drawn[0], " "+string(cardBarGlyph)+" first ") || !strings.HasPrefix(drawn[1], " "+string(cardBarGlyph)+" second ") {
		t.Fatalf("two short lines and one long one drew these rows, want one each for the short lines and the long one wrapped:\n%s", strings.Join(drawn, "\n"))
	}
	for number, line := range drawn {
		if !strings.HasPrefix(line, " "+string(cardBarGlyph)+" ") {
			t.Errorf("row %d of the card is %q, and every row begins with the bar", number+1, line)
		}
		if width := displayWidth(line); width > widestTranscript+bubbleFrame+marginColumns {
			t.Errorf("row %d of the card is %d columns wide on a 200-column terminal, and a card never wraps wider than %d", number+1, width, widestTranscript)
		}
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

// TestAToolLineIsDrawnAsACompactPill holds the pill's row: the state glyph on
// the ground in its own colour, then the tool's name in bold, its argument and
// its summary dim, all on the card fill, and the result id moved to the end as
// a keycap badge after one blank of ground.
func TestAToolLineIsDrawnAsACompactPill(t *testing.T) {
	screen := newThemedScreen(80, 24)
	screen.remember(block{kind: blockTool, text: "read memory/product.md · 2,100 characters · r3"})
	colors := screen.colors

	drawn := screen.blockLines(screen.blocks[0])
	if len(drawn) != 1 {
		t.Fatalf("one tool call drew %d rows, and it is one pill", len(drawn))
	}
	plain := plainText(drawn[0])
	if !strings.Contains(plain, string(doneGlyph)+"  read memory/product.md · 2,100 characters") {
		t.Errorf("the pill does not read as its glyph, its name, its argument and its summary: %q", plain)
	}
	if !strings.HasSuffix(plain, " r3 ") {
		t.Errorf("the pill does not end in the result id as a badge: %q", plain)
	}
	for _, wanted := range []string{
		colors.wrap(styleDone, string(doneGlyph)),
		colors.wrap(styleKey, "read"),
		colors.wrap(styleCardDim, " memory/product.md"),
		colors.wrap(styleCardDim, " ·") + colors.wrap(styleCardDim, " 2,100") + colors.wrap(styleCardDim, " characters"),
		colors.wrap(styleNormal, " ") + colors.wrap(styleKey, " r3 "),
	} {
		if !strings.Contains(drawn[0], wanted) {
			t.Errorf("the pill %q does not hold %q", drawn[0], wanted)
		}
	}
	if strings.Contains(drawn[0], backgroundOf(accentTone, depthTruecolor)) {
		t.Errorf("the pill is still filled with the accent: %q", drawn[0])
	}
}

// TestAPillsGlyphSaysWhetherTheCallIsInFlightDoneOrFailed reads the three
// shapes a tool line takes and holds the glyph and colour each gets, and that
// a record line, which the transcript draws through the same pill, reads as a
// finished thing.
func TestAPillsGlyphSaysWhetherTheCallIsInFlightDoneOrFailed(t *testing.T) {
	screen := newThemedScreen(80, 24)
	colors := screen.colors
	for _, one := range []struct {
		line   string
		glyph  rune
		drawn  style
		saying string
	}{
		{"▸ shell npm test", runningGlyph, styleAccent, "a call with no result yet is in flight"},
		{"shell npm test · r27 tests: all 51 passing", doneGlyph, styleDone, "a call with a result is done"},
		{"browser_click e9 · refused", failedGlyph, styleBad, "a call the program refused has failed"},
		{"shell rm -rf / · r4 the call was refused: not on the list", failedGlyph, styleBad, "a call that was refused has failed"},
		{"shell npm test · r5 tests: 3 failed", failedGlyph, styleBad, "a call whose result says failed has failed"},
		{"task 3 started · Build the game", doneGlyph, styleDone, "a record line is a finished thing"},
	} {
		drawn := screen.pillRows(one.line, false)[0]
		if !strings.Contains(drawn, colors.wrap(one.drawn, string(one.glyph))) {
			t.Errorf("the line %q is drawn %q, and %s", one.line, plainText(drawn), one.saying)
		}
	}
}

// TestAPillReadsTheToolTheArgumentTheIdAndTheCountApart holds the reading of
// a tool line into its pieces, whatever order the program wrote them in.
func TestAPillReadsTheToolTheArgumentTheIdAndTheCountApart(t *testing.T) {
	for _, one := range []struct {
		line string
		want pillParts
	}{
		{"▸ shell npm test", pillParts{tool: "shell", argument: "npm test"}},
		{"shell npm test · r27 tests: all passing", pillParts{tool: "shell", argument: "npm test", summary: "tests: all passing", id: "r27", done: true}},
		{"read memory/product.md · 2,100 characters · r3", pillParts{tool: "read", argument: "memory/product.md", summary: "2,100 characters", id: "r3", done: true}},
		{"web DigiByte · r2 web: 3 results × 3", pillParts{tool: "web", argument: "DigiByte", summary: "web: 3 results", id: "r2", count: "× 3", done: true}},
		{"▸ web DigiByte × 13", pillParts{tool: "web", argument: "DigiByte", count: "× 13"}},
		{"browser_click e9 · refused", pillParts{tool: "browser_click", argument: "e9", summary: "refused", done: true, failed: true}},
		{"task", pillParts{tool: "task"}},
	} {
		if got := readPill(one.line); got != one.want {
			t.Errorf("the line %q reads as %+v, want %+v", one.line, got, one.want)
		}
	}
}

// TestALongPillWrapsUnderItsOwnWordsAndKeepsItsBadgeOnTheLastRow holds the
// wrapping: a long line breaks into rows indented under the first row's
// words, every row on the card fill to the same edge, with the badge after the
// last row and nothing wider than the transcript.
func TestALongPillWrapsUnderItsOwnWordsAndKeepsItsBadgeOnTheLastRow(t *testing.T) {
	screen, _ := newTestScreen(60, 24)
	drawn := screen.pillRows("shell "+strings.Repeat("a long command ", 8)+"· r9 exit 0", false)
	if len(drawn) < 2 {
		t.Fatalf("a line far wider than the terminal drew %d rows, and it wraps", len(drawn))
	}
	widths := map[int]bool{}
	for number, line := range drawn {
		if displayWidth(line) > 60-marginColumns {
			t.Errorf("row %d of the pill is %d columns wide on a 60-column terminal: %q", number+1, displayWidth(line), line)
		}
		if number > 0 && !strings.HasPrefix(line, strings.Repeat(" ", marginColumns+gutterColumns+2)) {
			t.Errorf("row %d of the pill is %q, and a wrapped row sits under the first row's words", number+1, line)
		}
		if number < len(drawn)-1 {
			widths[displayWidth(line)] = true
		}
	}
	if len(widths) > 1 {
		t.Errorf("the rows of the pill end at different columns %v, and the card has a straight edge", widths)
	}
	if !strings.HasSuffix(drawn[len(drawn)-1], " r9 ") {
		t.Errorf("the last row %q does not end in the badge", drawn[len(drawn)-1])
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
	for _, size := range [][2]int{{80, 24}, {120, 40}, {160, 50}} {
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
