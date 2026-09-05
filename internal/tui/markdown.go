package tui

import "strings"

// inlineMarkers are the two pieces of markdown drawn inside a line: bold text
// and a code span. docs/TUI_DESIGN.md asks for markdown rendered lightly, so
// there is nothing else here on purpose.
var inlineMarkers = []struct {
	// mark is the text that opens and closes the piece.
	mark string
	// drawn is how the text between the marks is drawn.
	drawn style
}{
	{mark: "**", drawn: styleBold},
	{mark: "`", drawn: styleDim},
}

// markdownRows renders a reply as rows: bold, code spans, fenced code in a dim
// frame, list items with a dash, and headings no larger than the text.
func markdownRows(text string, width int) []row {
	drawn := []row{}
	fenced := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			drawn = append(drawn, fencedRow(line, width))
			continue
		}
		drawn = append(drawn, plainRows(line, width)...)
	}
	return drawn
}

// fencedRow draws one line of a fenced code block: dim, inside a thin frame, and
// cut rather than wrapped, because code broken across lines is harder to read
// than code with its end missing.
func fencedRow(line string, width int) row {
	drawn := row{}
	drawn.add(styleDim, "│ ")
	drawn.add(styleDim, cutTo(strings.ReplaceAll(line, "\t", "    "), width-2))
	return drawn
}

// plainRows draws one ordinary line of a reply. A heading loses its hashes and
// is drawn bold rather than larger, a list item keeps a dash, and everything
// else is wrapped with its bold and its code spans in place.
func plainRows(line string, width int) []row {
	text := strings.TrimRight(line, " ")
	forced := styleNormal
	if body, isHeading := strings.CutPrefix(strings.TrimLeft(text, "#"), " "); isHeading && strings.HasPrefix(text, "#") {
		text = strings.TrimSpace(body)
		forced = styleBold
	}
	if item, isStar := strings.CutPrefix(strings.TrimSpace(text), "* "); isStar {
		text = "- " + item
	}
	return wrapSpans(inlineSpans(text, forced), width)
}

// inlineSpans splits one line into the runs of text between its markers, each
// with the style it is drawn in. A marker that never closes is left as it was
// written, because guessing at the writer's meaning is worse than showing it.
func inlineSpans(text string, forced style) []span {
	spans := []span{}
	for text != "" {
		at, mark, drawn := earliestMarker(text)
		if at < 0 {
			return append(spans, span{style: forced, text: text})
		}
		if at > 0 {
			spans = append(spans, span{style: forced, text: text[:at]})
		}
		body := text[at+len(mark):]
		closes := strings.Index(body, mark)
		if closes < 0 {
			return append(spans, span{style: forced, text: text[at:]})
		}
		spans = append(spans, span{style: drawn, text: body[:closes]})
		text = body[closes+len(mark):]
	}
	return spans
}

// earliestMarker finds the first marker in the text, and returns minus one when
// there is none.
func earliestMarker(text string) (int, string, style) {
	found := -1
	mark := ""
	drawn := styleNormal
	for _, marker := range inlineMarkers {
		at := strings.Index(text, marker.mark)
		if at >= 0 && (found < 0 || at < found) {
			found, mark, drawn = at, marker.mark, marker.drawn
		}
	}
	return found, mark, drawn
}

// wrapSpans breaks styled text into rows no wider than the width, keeping each
// word in the style it was written in.
func wrapSpans(spans []span, width int) []row {
	if width < 1 {
		width = 1
	}
	drawn := []row{}
	current := row{}
	for _, one := range wordsOf(spans) {
		for at, part := range splitLongWord(one.text, width) {
			separator := " "
			if current.width == 0 || (at == 0 && one.glued) {
				separator = ""
			}
			if current.width > 0 && current.width+displayWidth(separator+part) > width {
				drawn = append(drawn, current)
				current = row{}
				separator = ""
			}
			current.add(one.style, separator+part)
		}
	}
	return append(drawn, current)
}

// wordsOf splits styled runs of text into the words inside them, each keeping
// the style of the run it came from. The first word of a run that begins
// where the last run ended, with no blank on either side of the seam, is
// glued to the word before it, so that "`add`," is drawn as one word and not
// as "add ,".
func wordsOf(spans []span) []span {
	words := []span{}
	endedWithABlank := true
	for _, piece := range spans {
		if piece.text == "" {
			continue
		}
		startsWithABlank := strings.HasPrefix(piece.text, " ") || strings.HasPrefix(piece.text, "\t")
		for number, one := range strings.Fields(piece.text) {
			glued := number == 0 && !startsWithABlank && !endedWithABlank && len(words) > 0
			words = append(words, span{style: piece.style, text: one, glued: glued})
		}
		endedWithABlank = strings.HasSuffix(piece.text, " ") || strings.HasSuffix(piece.text, "\t")
	}
	return words
}

// splitLongWord breaks a word that is wider than a whole line into pieces that
// fit, and leaves every other word alone.
func splitLongWord(text string, width int) []string {
	if displayWidth(text) <= width {
		return []string{text}
	}
	pieces := []string{}
	letters := []rune(text)
	for len(letters) > 0 {
		end := hardFit(letters, width)
		pieces = append(pieces, string(letters[:end]))
		letters = letters[end:]
	}
	return pieces
}

// hardFit returns how many characters fit in the width, counting at least one so
// that a character wider than the whole line still moves the loop along.
func hardFit(letters []rune, width int) int {
	used := 0
	for at, letter := range letters {
		used += runeWidth(letter)
		if used > width {
			if at == 0 {
				return 1
			}
			return at
		}
	}
	return len(letters)
}

// cutTo shortens text to a number of columns, leaving it alone when it already
// fits.
func cutTo(text string, width int) string {
	if width < 1 {
		return ""
	}
	if displayWidth(text) <= width {
		return text
	}
	letters := []rune(text)
	return string(letters[:hardFit(letters, width)])
}
