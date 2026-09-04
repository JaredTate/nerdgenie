package tui

import "strings"

// The shape of the wordmark. Every letter is five rows tall and five columns
// wide, letters inside a word are one column apart, and the two words are three
// columns apart. The font is written here rather than taken from a tool,
// because six letters are six letters.
const (
	// blockGlyph is the solid square the block letters are drawn out of.
	blockGlyph = '█'
	// blockRows is how many rows tall a block letter is.
	blockRows = 5
	// letterGap is how many columns sit between two letters of a word.
	letterGap = 1
	// wordGap is how many columns sit between the two words.
	wordGap = 3
	// wordmarkColumns is how wide the block wordmark is: NERD, the gap, and
	// GENIE. A test holds the drawing to it.
	wordmarkColumns = 55
	// welcomeColumns and welcomeRows are the smallest transcript area the block
	// wordmark is drawn in. A smaller area gets the wordmark on one line,
	// because a wordmark cut in half is worse than a wordmark written small.
	welcomeColumns = 60
	welcomeRows    = 14
)

// The words of the welcome. The wordmark is two words in two colours, NERD in
// white and GENIE in DigiByte blue, drawn on one line at the top of every frame
// and in block letters while there is nothing else to show.
const (
	// wordmarkFirst is the white half of the wordmark.
	wordmarkFirst = "NERD"
	// wordmarkSecond is the blue half.
	wordmarkSecond = "GENIE"
	// taglineText is what this program is, in the brand's words.
	taglineText = "the open-source agent harness"
	// wishText is the brand's promise, drawn under the tagline in dim italics.
	wishText = "your wish is its command."
	// askHintText is the one line that says how to start.
	askHintText = "type an ask, or /help"
)

// blockLetters is the five-row block font, one shape per letter of the wordmark.
var blockLetters = map[rune][blockRows]string{
	'N': {"█   █", "██  █", "█ █ █", "█  ██", "█   █"},
	'E': {"█████", "█    ", "████ ", "█    ", "█████"},
	'R': {"████ ", "█   █", "████ ", "█  █ ", "█   █"},
	'D': {"████ ", "█   █", "█   █", "█   █", "████ "},
	'G': {"█████", "█    ", "█  ██", "█   █", "█████"},
	'I': {"█████", "  █  ", "  █  ", "  █  ", "█████"},
}

// blankLetter is what a letter the font does not have is drawn as, so that the
// rows of a word stay the same width whatever it says.
var blankLetter = [blockRows]string{"     ", "     ", "     ", "     ", "     "}

// blockWord draws one word in block letters, five rows tall, its letters one
// column apart.
func blockWord(word string) [blockRows]string {
	drawn := [blockRows]string{}
	for at, letter := range word {
		shape, known := blockLetters[letter]
		if !known {
			shape = blankLetter
		}
		for line := range drawn {
			if at > 0 {
				drawn[line] += strings.Repeat(" ", letterGap)
			}
			drawn[line] += shape[line]
		}
	}
	return drawn
}

// welcomeRows draws the welcome into the rows the transcript has while there is
// nothing in it: the wordmark, the tagline under it, the wish in dim italics,
// and one line saying how to start, centred in the area it is given. It is gone
// the moment a conversation starts, because from then on the transcript is the
// thing worth looking at.
func (screen *Screen) welcomeRows(height int) []string {
	middle := screen.wordmarkLines(height)
	middle = append(middle, "")
	middle = append(middle, screen.centredRow(styleDim, taglineText))
	middle = append(middle, screen.centredRow(styleItalic, wishText))
	middle = append(middle, "")
	middle = append(middle, screen.centredRow(styleDim, askHintText))

	if len(middle) > height {
		middle = middle[len(middle)-height:]
	}
	rows := make([]string, (height-len(middle))/2)
	rows = append(rows, middle...)
	for len(rows) < height {
		rows = append(rows, "")
	}
	return rows
}

// wordmarkLines is the wordmark as rows of the frame: five rows of block
// letters when the transcript area is at least welcomeColumns by welcomeRows,
// and the one-line wordmark otherwise.
func (screen *Screen) wordmarkLines(height int) []string {
	if screen.transcriptColumns() < welcomeColumns || height < welcomeRows {
		line := row{}
		line.add(styleBold, wordmarkFirst)
		line.add(styleBrand, wordmarkSecond)
		return []string{screen.centred(line)}
	}
	first, second := blockWord(wordmarkFirst), blockWord(wordmarkSecond)
	drawn := []string{}
	for at := range blockRows {
		line := row{}
		line.add(styleBold, first[at])
		line.blanks(wordGap)
		line.add(styleBrand, second[at])
		drawn = append(drawn, screen.centred(line))
	}
	return drawn
}

// centred draws a row in the middle of the transcript, cut to fit rather than
// wrapped, because everything the welcome draws is one line long.
func (screen *Screen) centred(line row) string {
	line.keepWithin(screen.transcriptColumns() - 2*marginColumns)
	full := row{}
	full.blanks(max((screen.transcriptColumns()-line.width)/2, marginColumns))
	for _, piece := range line.spans {
		full.addSpan(piece)
	}
	return full.render(screen.colors)
}

// centredRow draws one piece of text in one style in the middle of the
// transcript.
func (screen *Screen) centredRow(chosen style, text string) string {
	line := row{}
	line.add(chosen, text)
	return screen.centred(line)
}
