package tui

import "strings"

// The shape of the wordmark. Every letter is five rows tall and five columns
// wide, letters inside a word are one column apart, and the two words are three
// columns apart.
const (
	// blockGlyph is the solid square the block letters are drawn out of.
	blockGlyph = '█'
	// blockRows is how many rows tall a block letter is.
	blockRows = 5
	// letterGap is how many columns sit between two letters of a word.
	letterGap = 1
	// wordGap is how many columns sit between the two words.
	wordGap = 3
)

// wordmarkText is what the banner says, and the letters the block font has to
// know how to draw.
const wordmarkText = "COEUS AGENT"

// taglineText is the one line under the wordmark: what this agent is for, in
// the fewest words that say it.
const taglineText = "the agent that does not forget what it is doing"

// commandHintText is the line under the tag, and it is the only thing on a first
// frame that says where the tasks, the jobs and everything else are to be found.
// The first person to use the screen asked how to see what jobs and tasks it
// had, and nothing on the frame answered.
const commandHintText = "type / to see the commands"

// blockLetters is the five-row block font the wordmark is drawn in, written here
// rather than taken from a library because nine letters are nine letters. A
// space is drawn by the gap between the two words rather than by a letter.
var blockLetters = map[rune][blockRows]string{
	'C': {"█████", "█    ", "█    ", "█    ", "█████"},
	'O': {"█████", "█   █", "█   █", "█   █", "█████"},
	'E': {"█████", "█    ", "████ ", "█    ", "█████"},
	'U': {"█   █", "█   █", "█   █", "█   █", "█████"},
	'S': {"█████", "█    ", "█████", "    █", "█████"},
	'A': {"█████", "█   █", "█████", "█   █", "█   █"},
	'G': {"█████", "█    ", "█  ██", "█   █", "█████"},
	'N': {"█   █", "██  █", "█ █ █", "█  ██", "█   █"},
	'T': {"█████", "  █  ", "  █  ", "  █  ", "  █  "},
}

// wordmarkRows draws COEUS AGENT in block letters, five rows tall, or says
// false when the terminal is too narrow to hold them.
func wordmarkRows(width int) ([blockRows]string, bool) {
	drawn := [blockRows]string{}
	afterLetter := false
	for _, letter := range wordmarkText {
		shape, known := blockLetters[letter]
		if !known {
			for at := range drawn {
				drawn[at] += strings.Repeat(" ", wordGap)
			}
			afterLetter = false
			continue
		}
		for at := range drawn {
			if afterLetter {
				drawn[at] += strings.Repeat(" ", letterGap)
			}
			drawn[at] += shape[at]
		}
		afterLetter = true
	}
	return drawn, displayWidth(drawn[0]) <= width
}

// bannerRows draws the whole banner into the rows the transcript has while there
// is nothing in the transcript: the wordmark, one line saying what this agent
// is for, and a small filled tag naming the model and what the program is doing.
// It is centred in the space it is given and is gone the moment a conversation
// starts, because from then on the transcript is the thing worth looking at.
func (screen *Screen) bannerRows(height int) []string {
	middle := screen.wordmarkLines()
	middle = append(middle, "")
	middle = append(middle, screen.centredRow(styleDim, taglineText))
	middle = append(middle, screen.centredRow(styleChip, " "+screen.bannerWords()+" "))
	middle = append(middle, screen.centredRow(styleDim, commandHintText))

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

// wordmarkLines is the wordmark as rows of the frame: the block letters where
// they fit, and the plain word where they do not, because a wordmark cut in half
// is worse than a wordmark written small.
func (screen *Screen) wordmarkLines() []string {
	shape, fits := wordmarkRows(screen.width - 2*marginColumns)
	if !fits {
		return []string{screen.centredRow(styleBold, wordmarkText)}
	}
	drawn := []string{}
	for _, line := range shape {
		drawn = append(drawn, screen.centredRow(styleBold, line))
	}
	return drawn
}

// bannerWords is the small tag under the tagline: the model in use and what the
// program is doing, which is the whole of what a person needs before they have
// typed anything.
func (screen *Screen) bannerWords() string {
	if screen.modelAlias == "" {
		return screen.stateWords()
	}
	return screen.modelAlias + " · " + screen.stateWords()
}

// centredRow draws one piece of text in the middle of the frame, cut to fit
// rather than wrapped, because everything the banner draws is one line long.
func (screen *Screen) centredRow(chosen style, text string) string {
	text = cutTo(text, screen.width-2*marginColumns)
	line := row{}
	line.blanks(max((screen.width-displayWidth(text))/2, marginColumns))
	line.add(chosen, text)
	return line.render(screen.colors)
}
