package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// escapeCodes matches the colour and style codes a terminal reads and a person
// does not see, so that a test can compare the structure of two frames.
var escapeCodes = regexp.MustCompile("\x1b\\[[0-9;]*m")

// plainText is a frame with every escape code taken out of it.
func plainText(frame string) string {
	return escapeCodes.ReplaceAllString(frame, "")
}

// frameGlyphs are the characters that draw the frame rather than say anything:
// the bars, the arrows, and the box rules. They change with the width, and the
// words between them do not.
var frameGlyphs = strings.NewReplacer(
	string(personBarGlyph), " ", string(toolArrowGlyph), " ", string(ruleGlyph), " ",
	"│", " ", "┌", " ", "┐", " ", "└", " ", "┘", " ")

// transcriptWords is every word the transcript is drawing, with the frame's own
// glyphs taken out. This is what must survive a resize even though the wrapping
// does not.
func transcriptWords(screen *Screen) string {
	drawn := frameGlyphs.Replace(strings.Join(screen.transcriptRows(1<<20), " "))
	return strings.Join(strings.Fields(drawn), " ")
}

// resizeTo tells the screen the terminal is a new size.
func resizeTo(screen *Screen, width int, height int) {
	screen.Update(tea.WindowSizeMsg{Width: width, Height: height})
}

// aTalkedTranscript fills a screen with one of each kind of block, so that a
// test about the frame as a whole covers the whole frame.
func aTalkedTranscript(screen *Screen) {
	screen.remember(block{kind: blockPerson, text: "Post a tweet about the DigiByte anniversary. Use the product notes and keep it under 280 characters."})
	screen.remember(block{kind: blockReply, text: "Where I stand: the notes are read, drafting next."})
	screen.remember(block{kind: blockTool, text: "read memory/product.md · 2,100 characters · r3"})
	screen.remember(block{kind: blockCard, shown: card{kind: cardPreview, id: "3", title: previewTitle, body: "browser_click e7 \"Post\""}})
}

func TestResizingRewrapsTheSameWordsAndLeavesNothingBehind(t *testing.T) {
	screen, _ := newTestScreen(80, 24)
	aTalkedTranscript(screen)

	wide := screen.View()
	wideWords := transcriptWords(screen)

	resizeTo(screen, 60, 24)
	narrow := screen.View()
	narrowWords := transcriptWords(screen)

	resizeTo(screen, 80, 24)
	wideAgain := screen.View()

	if narrowWords != wideWords {
		t.Errorf("the transcript reads %q at sixty columns and %q at eighty, and a resize changes only the wrapping", narrowWords, wideWords)
	}
	if narrow == wide {
		t.Error("the frame at sixty columns is the same text as at eighty, so nothing was re-wrapped")
	}
	if wideAgain != wide {
		t.Error("going back to eighty columns did not give back the same frame, so something was left behind")
	}
	for name, sized := range map[string]struct {
		frame string
		width int
	}{"eighty": {wide, 80}, "sixty": {narrow, 60}} {
		for number, line := range strings.Split(sized.frame, "\n") {
			if displayWidth(plainText(line)) > sized.width {
				t.Errorf("at %s columns row %d is %d wide: %q", name, number+1, displayWidth(plainText(line)), line)
			}
		}
	}
}

func TestEveryFrameHasExactlyAsManyRowsAsTheTerminal(t *testing.T) {
	screen, _ := newTestScreen(80, 24)
	aTalkedTranscript(screen)
	for _, size := range [][2]int{{80, 24}, {60, 24}, {120, 40}, {40, 10}, {200, 60}} {
		resizeTo(screen, size[0], size[1])
		if rows := len(strings.Split(screen.View(), "\n")); rows != size[1] {
			t.Errorf("at %d by %d the frame is %d rows", size[0], size[1], rows)
		}
	}
}

func TestNoColorRendersTheSameStructure(t *testing.T) {
	plain, _ := newTestScreen(80, 24)
	aTalkedTranscript(plain)

	colored := New(Options{
		Clock:  plain.clock,
		Width:  80,
		Height: 24,
		Environment: func(name string) string {
			return map[string]string{"COLORTERM": "truecolor", "TERM": "xterm-256color"}[name]
		},
	})
	aTalkedTranscript(colored)

	if !strings.Contains(colored.View(), "\x1b[") {
		t.Fatal("the coloured frame holds no escape codes at all, so nothing is coloured")
	}
	if strings.Contains(plain.View(), "\x1b[") {
		t.Fatal("the frame holds escape codes even though NO_COLOR is set")
	}
	if plainText(colored.View()) != plain.View() {
		t.Error("taking the colour out of the coloured frame does not give the plain frame, so the two do not have the same structure")
	}
	for _, carrying := range []string{string(personBarGlyph), string(toolArrowGlyph), string(ruleGlyph), "┌", "└"} {
		if !strings.Contains(plain.View(), carrying) {
			t.Errorf("the plain frame has no %q, and without colour the glyphs are what carry the structure", carrying)
		}
	}
}
