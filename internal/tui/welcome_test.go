package tui

import (
	"strings"
	"testing"
)

// TestTheWelcomeBlockIsDrawnAtSixtyByFourteenAndNotBelowIt pins the smallest
// area the block wordmark is drawn in: the transcript area at sixty columns by
// fourteen rows gets the block letters, and one column or one row less gets the
// one-line wordmark instead, because a wordmark cut in half is worse than a
// wordmark written small.
func TestTheWelcomeBlockIsDrawnAtSixtyByFourteenAndNotBelowIt(t *testing.T) {
	if welcomeColumns != 60 || welcomeRows != 14 {
		t.Fatalf("the welcome block is drawn from %d by %d, and the brief says sixty by fourteen", welcomeColumns, welcomeRows)
	}
	for _, one := range []struct {
		width, height int
		blocks        bool
		saying        string
	}{
		{60, 19, true, "sixty columns by fourteen rows"},
		{59, 19, false, "fifty-nine columns by fourteen rows"},
		{60, 18, false, "sixty columns by thirteen rows"},
	} {
		screen, _ := newTestScreen(one.width, one.height)
		if area := screen.height - 4 - len(screen.inputRows()); area != one.height-5 {
			t.Fatalf("at %s the transcript area is %d rows, and the test meant %d", one.saying, area, one.height-5)
		}
		frame := screen.frame()
		if drawn := strings.Contains(frame, string(blockGlyph)); drawn != one.blocks {
			t.Errorf("at %s the block wordmark is drawn: %v, and it should be: %v\n%s", one.saying, drawn, one.blocks, frame)
		}
		if !one.blocks && !strings.Contains(frame, wordmarkFirst+wordmarkSecond) {
			t.Errorf("at %s the one-line wordmark is missing:\n%s", one.saying, frame)
		}
	}
}

// TestTheBlockWordmarkIsAsWideAsItSaysAndFiveRowsTall bounds the art: the two
// words and the gap between them are exactly wordmarkColumns wide on every row,
// which fits inside the smallest area the block is drawn in, and the block is
// blockRows tall.
func TestTheBlockWordmarkIsAsWideAsItSaysAndFiveRowsTall(t *testing.T) {
	first, second := blockWord(wordmarkFirst), blockWord(wordmarkSecond)
	for at := range blockRows {
		if width := displayWidth(first[at]) + wordGap + displayWidth(second[at]); width != wordmarkColumns {
			t.Errorf("row %d of the wordmark is %d columns, and every row is %d", at+1, width, wordmarkColumns)
		}
	}
	if wordmarkColumns > welcomeColumns-2*marginColumns {
		t.Errorf("the wordmark is %d columns and the smallest welcome area leaves %d for it", wordmarkColumns, welcomeColumns-2*marginColumns)
	}

	screen, _ := newTestScreen(80, 24)
	tall := 0
	for _, line := range strings.Split(screen.frame(), "\n") {
		if strings.Contains(line, string(blockGlyph)) {
			tall++
		}
	}
	if tall != blockRows {
		t.Errorf("the wordmark is drawn %d rows tall, and it is %d", tall, blockRows)
	}
}

// TestTheWordmarkIsNerdInWhiteAndGenieInDigiByteBlue holds the brand's two
// colours on both wordmarks: the block letters of the welcome and the one line
// at the top of every frame, with the dim tagline after it in the header.
func TestTheWordmarkIsNerdInWhiteAndGenieInDigiByteBlue(t *testing.T) {
	screen := newThemedScreen(80, 24)
	frame := screen.frame()
	first, second := blockWord(wordmarkFirst), blockWord(wordmarkSecond)
	for at := range blockRows {
		if !strings.Contains(frame, screen.colors.wrap(styleBold, first[at])) {
			t.Errorf("row %d of NERD is not drawn in bold white", at+1)
		}
		if !strings.Contains(frame, screen.colors.wrap(styleBrand, second[at])) {
			t.Errorf("row %d of GENIE is not drawn in DigiByte blue", at+1)
		}
	}

	header := headerOf(screen)
	if !strings.Contains(header, screen.colors.wrap(styleBold, wordmarkFirst)+screen.colors.wrap(styleBrand, wordmarkSecond)) {
		t.Errorf("the header is %q, and its wordmark is NERD in bold white followed by GENIE in DigiByte blue", header)
	}
	if !strings.Contains(plainText(header), wordmarkFirst+wordmarkSecond+" · "+taglineText) {
		t.Errorf("the header reads %q, and the tagline follows the wordmark", plainText(header))
	}
}

// TestTheWelcomeSaysTheTaglineTheWishAndHowToStart holds the three lines under
// the wordmark, the wish in dim italics, and that nothing else is drawn there:
// the model and the state belong to the header and the strip.
func TestTheWelcomeSaysTheTaglineTheWishAndHowToStart(t *testing.T) {
	screen := newThemedScreen(80, 24)
	frame := screen.frame()
	for _, wanted := range []string{taglineText, wishText, askHintText} {
		if !strings.Contains(plainText(frame), wanted) {
			t.Errorf("the welcome does not say %q:\n%s", wanted, plainText(frame))
		}
	}
	if !strings.Contains(frame, screen.colors.wrap(styleItalic, wishText)) {
		t.Error("the wish line is not drawn in dim italics")
	}
	if strings.Contains(plainText(frame), "connecting") && strings.Count(plainText(frame), "connecting") != 2 {
		t.Errorf("the state is written somewhere other than the header and the strip:\n%s", plainText(frame))
	}
}

func TestTheWelcomeScrollsAwayOnceTheConversationStarts(t *testing.T) {
	screen, _ := newTestScreen(80, 24)
	if !strings.Contains(screen.frame(), string(blockGlyph)) {
		t.Fatal("the welcome is not on the first frame")
	}
	screen.remember(block{kind: blockPerson, text: "hello"})
	after := screen.frame()
	if strings.Contains(after, string(blockGlyph)) || strings.Contains(after, wishText) {
		t.Errorf("the welcome is still on the frame once the conversation started:\n%s", after)
	}
}

func TestANarrowTerminalGetsTheWordmarkInPlainLettersRatherThanBlocks(t *testing.T) {
	screen, _ := newTestScreen(40, 24)
	frame := screen.frame()

	if strings.Contains(frame, string(blockGlyph)) {
		t.Errorf("a forty-column terminal drew the block letters, which do not fit:\n%s", frame)
	}
	if !strings.Contains(frame, wordmarkFirst+wordmarkSecond) {
		t.Errorf("a forty-column terminal lost the wordmark altogether:\n%s", frame)
	}
	for number, line := range strings.Split(frame, "\n") {
		if displayWidth(line) > 40 {
			t.Errorf("row %d is %d columns wide: %q", number+1, displayWidth(line), line)
		}
	}
}

// TestTheHeaderDropsTheTaglineBeforeItDropsTheStatus holds that the tagline is
// a nicety and the status is information: on a terminal too narrow for both,
// the tagline goes first and the task, the model and the cost stay.
func TestTheHeaderDropsTheTaglineBeforeItDropsTheStatus(t *testing.T) {
	wide, _ := newTestScreen(160, 24)
	wide.Update(linkMessage{up: true})
	send(wide, aFullStatus())
	if header := plainText(headerOf(wide)); !strings.Contains(header, taglineText) || !strings.Contains(header, "task 17 running") {
		t.Errorf("at a hundred and sixty columns the header is %q, and there is room for the tagline and the status", header)
	}

	narrow, _ := newTestScreen(80, 24)
	narrow.Update(linkMessage{up: true})
	send(narrow, aFullStatus())
	header := plainText(headerOf(narrow))
	if strings.Contains(header, taglineText) {
		t.Errorf("at eighty columns the header is %q, and the tagline is dropped before the status is cut", header)
	}
	for _, kept := range []string{wordmarkFirst + wordmarkSecond, "opus", "task 17 running"} {
		if !strings.Contains(header, kept) {
			t.Errorf("at eighty columns the header is %q, and it keeps %q", header, kept)
		}
	}
}
