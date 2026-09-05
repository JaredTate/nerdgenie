// The flat card, a coloured bar down the left of every message and a fill a
// step darker than the ground in place of a box around it, is OpenCode's
// message drawing, read at
// ~/Code/opencode/packages/tui/src/routes/session/index.tsx, where each
// message is a box with a border on its left side only. Nothing was copied:
// that file is TypeScript drawing through a layout engine, and this is Go
// writing rows of its own.

package tui

import (
	"strings"
)

// bubblePadding is the one blank column inside a card on each side of its
// words, so that the letters never touch the bar or the edge of the fill.
const bubblePadding = 1

// bubbleFrame is the columns a card's bar and padding take, which is what a
// wrapped line has to fit inside: the bar, and a blank on each side of the
// words.
const bubbleFrame = 1 + 2*bubblePadding

// bubble is the shape of one card of talk in the transcript.
type bubble struct {
	// bar is how the bar down the left of the card is drawn, which is the
	// one thing that tells the person's card from the agent's on a terminal
	// with colour; on one without, the side the card leans to does.
	bar style
	// fill is how the blanks inside the card are drawn, so that the card is a
	// solid shape a step below the ground.
	fill style
	// leaningRight puts the card against the right-hand edge of the frame,
	// which is where the person's own words sit.
	leaningRight bool
}

// personBubble is the card a message the person typed is drawn in: against
// the right-hand edge, with its bar in the light blue.
func personBubble() bubble {
	return bubble{bar: stylePersonBar, fill: styleCard, leaningRight: true}
}

// agentBubble is the card the agent's reply is drawn in: against the left-hand
// edge, with its bar in DigiByte's own blue, so that the reply itself is the
// loudest thing on the row.
func agentBubble() bubble {
	return bubble{bar: styleBar, fill: styleCard}
}

// bubbleWidth is the widest card the transcript will draw: the frame less its
// margins, and never wider than a line is comfortable to read.
func (screen *Screen) bubbleWidth() int {
	widest := screen.transcriptColumns() - 2*marginColumns
	if widest > widestTranscript+bubbleFrame {
		widest = widestTranscript + bubbleFrame
	}
	return max(widest, bubbleFrame+1)
}

// bubbleRows draws already-styled lines as a card: one row per line, each
// beginning with the bar, and no lid or floor, so a card that is still growing
// keeps its shape as every row arrives. The card is only as wide as the widest
// line in it, so a two-word answer gets a two-word card, and it is never wider
// than the transcript allows.
func (screen *Screen) bubbleRows(lines []row, shape bubble) []string {
	inner := screen.bubbleWidth() - bubbleFrame
	widest := 0
	for at := range lines {
		lines[at].keepWithin(inner)
		widest = max(widest, lines[at].width)
	}
	widest = max(widest, 1)
	indent := marginColumns
	if shape.leaningRight {
		indent = screen.transcriptColumns() - marginColumns - widest - bubbleFrame
	}

	drawn := []string{}
	for _, line := range lines {
		drawn = append(drawn, screen.bubbleBodyRow(shape, line, widest, indent))
	}
	return drawn
}

// bubbleBodyRow draws one line of a card: the bar, a blank of padding, the
// words moved onto the card's own fill, and padding out to the widest line so
// that the right-hand edge of the card stays straight.
func (screen *Screen) bubbleBodyRow(shape bubble, line row, widest int, indent int) string {
	full := row{}
	full.blanks(indent)
	full.add(shape.bar, string(cardBarGlyph))
	full.padWith(shape.fill, bubblePadding)
	for _, piece := range line.spans {
		full.add(onTheCard(piece.style), piece.text)
	}
	full.padWith(shape.fill, widest-line.width+bubblePadding)
	return full.render(screen.colors)
}

// onTheCard is the style a piece of text takes once it sits on a card rather
// than on the ground: plain words and the person's words go white on the
// card, bold words go bold white on the card, which is the keycap paint, and
// dim words, such as a code span, go dim on the card. Every other style is
// left as it is.
func onTheCard(chosen style) style {
	switch chosen {
	case styleNormal, styleChip:
		return styleCard
	case styleBold:
		return styleKey
	case styleDim:
		return styleCardDim
	default:
		return chosen
	}
}

// pillParts is one tool line read into its pieces, so that each can be drawn
// in its own style.
type pillParts struct {
	// tool is the tool's name, the first word of the line.
	tool string
	// argument is the one argument that says what the call does, which is the
	// rest of the line before the program's separator.
	argument string
	// summary is what came back, with the result id taken out of it.
	summary string
	// id is the result id, such as r27, when the summary carried one.
	id string
	// count is the repeat count the transcript writes on the end, such as
	// "× 13", when the same call was made again and again.
	count string
	// done says the call has a result, which is when the line carries the
	// separator; without it the call is still in flight.
	done bool
	// failed says the result says the call was refused or failed.
	failed bool
}

// The two words a result carries when a call went wrong: the program writes
// "refused" after the separator when it refused the call, and a tool's own
// summary says "failed" when the tool did.
var failureWords = []string{"refused", "failed"}

// readPill reads a tool line into its pieces. The program writes a line as
// "<tool> <argument> · <id> <summary>", with the arrow in front while the call
// is in flight, and the transcript may add "× N" on the end; a record line
// has the same shape and reads the same way.
func readPill(text string) pillParts {
	read := pillParts{}
	text, read.count = withoutRepeatCount(withoutLeadingArrow(text))
	call, rest, done := strings.Cut(text, toolLineSeparator)
	read.tool, read.argument, _ = strings.Cut(strings.TrimSpace(call), " ")
	read.argument = strings.TrimSpace(read.argument)
	read.done = done
	if !done {
		return read
	}
	read.summary, read.id = withoutResultID(rest)
	lowered := strings.ToLower(rest)
	for _, word := range failureWords {
		if strings.Contains(lowered, word) {
			read.failed = true
		}
	}
	return read
}

// withoutRepeatCount takes a repeat count such as "× 13" off the end of a
// line and hands it back on its own.
func withoutRepeatCount(text string) (string, string) {
	mark := " " + string(repeatGlyph) + " "
	at := strings.LastIndex(text, mark)
	if at < 0 {
		return text, ""
	}
	after := text[at+len(mark):]
	if after == "" || strings.Trim(after, "0123456789") != "" {
		return text, ""
	}
	return text[:at], string(repeatGlyph) + " " + after
}

// withoutResultID takes the result id out of what came back of a call and
// hands the rest back with the separators tidied, so that "2,100 characters ·
// r3" reads as the summary "2,100 characters" and the id "r3".
func withoutResultID(rest string) (string, string) {
	kept := []string{}
	id := ""
	for _, word := range strings.Fields(rest) {
		if id == "" && isResultID(word) {
			id = word
			continue
		}
		kept = append(kept, word)
	}
	summary := strings.Join(kept, " ")
	summary = strings.Trim(summary, " ·")
	return strings.Join(strings.Fields(summary), " "), id
}

// isResultID says whether a word is a result id: the letter r and then digits
// and nothing else.
func isResultID(word string) bool {
	if len(word) < 2 || word[0] != 'r' {
		return false
	}
	return strings.Trim(word[1:], "0123456789") == ""
}

// stateSpan is the glyph in front of a pill and the colour it is drawn in: the
// pointer in the accent while the call is in flight, a green check once it
// has a result, and a red cross when that result says it was refused or
// failed. A focused pill draws its glyph with the letters and the ground
// swapped, which is a change of colour and never of words.
func (read pillParts) stateSpan(focused bool) span {
	mark := span{style: styleAccent, text: string(runningGlyph)}
	switch {
	case read.failed:
		mark = span{style: styleBad, text: string(failedGlyph)}
	case read.done:
		mark = span{style: styleDone, text: string(doneGlyph)}
	}
	if focused {
		mark.style = styleReverse
	}
	return mark
}

// bodySpans are the words of a pill in their styles: the tool's name in bold
// white on the card, and its argument, its summary and its repeat count dim.
func (read pillParts) bodySpans() []span {
	spans := []span{{style: styleKey, text: read.tool}}
	if read.argument != "" {
		spans = append(spans, span{style: styleCardDim, text: " " + read.argument})
	}
	if read.summary != "" {
		spans = append(spans, span{style: styleCardDim, text: toolLineSeparator + read.summary})
	}
	if read.count != "" {
		spans = append(spans, span{style: styleCardDim, text: " " + read.count})
	}
	return spans
}

// badgeColumns is the room a result id's badge takes on the end of a pill:
// one blank of ground and the id in a keycap.
func (read pillParts) badgeColumns() int {
	if read.id == "" {
		return 0
	}
	return 1 + displayWidth(read.id) + 2*bubblePadding
}

// pillGlyphColumns is the room the state glyph and the blank after it take at
// the front of a pill's first row, and the indent of every row after it.
const pillGlyphColumns = 2

// pillRows draws one tool call as a compact row: the state glyph on the
// ground, then the tool, its argument and its summary on the card fill, and
// the result id as a small badge on the end. A long line wraps under its own
// words, every row filled to the same edge, and the badge follows the last
// row. The result text itself is never drawn.
func (screen *Screen) pillRows(text string, focused bool) []string {
	read := readPill(text)
	indent := marginColumns + gutterColumns
	room := screen.bubbleWidth() - gutterColumns - pillGlyphColumns - 2*bubblePadding - read.badgeColumns()
	wrapped := wrapSpans(read.bodySpans(), max(room, 1))
	widest := 0
	for _, line := range wrapped {
		widest = max(widest, line.width)
	}

	drawn := []string{}
	for number, line := range wrapped {
		pill := row{}
		pill.blanks(indent)
		if number == 0 {
			pill.addSpan(read.stateSpan(focused))
			pill.blanks(pillGlyphColumns - 1)
		} else {
			pill.blanks(pillGlyphColumns)
		}
		pill.padWith(styleCard, bubblePadding)
		for _, piece := range line.spans {
			pill.addSpan(piece)
		}
		pill.padWith(styleCard, widest-line.width+bubblePadding)
		if number == len(wrapped)-1 && read.id != "" {
			pill.blanks(1)
			pill.add(styleKey, " "+read.id+" ")
		}
		drawn = append(drawn, pill.render(screen.colors))
	}
	return drawn
}

// withoutLeadingArrow takes the arrow off the front of a line that already
// carries one. The running program writes the line for a call in flight
// beginning with the design's own arrow, and the pill draws a state glyph of
// its own in that place, so a line handed over with the arrow would otherwise
// be drawn with both.
func withoutLeadingArrow(text string) string {
	trimmed := strings.TrimLeft(text, " ")
	if !strings.HasPrefix(trimmed, string(toolArrowGlyph)) {
		return text
	}
	return strings.TrimLeft(strings.TrimPrefix(trimmed, string(toolArrowGlyph)), " ")
}
