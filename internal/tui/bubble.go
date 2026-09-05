package tui

import (
	"strings"
)

// bubblePadding is the one blank column inside a bubble on each side, so that
// the letters never touch the border.
const bubblePadding = 1

// border is the eight characters a box is drawn out of: the four corners, the
// two horizontal runs, and the two upright sides.
type border struct {
	// Top and Bottom are the characters the horizontal runs repeat.
	Top, Bottom string
	// Left and Right are the two upright sides.
	Left, Right string
	// TopLeft, TopRight, BottomLeft and BottomRight are the four corners.
	TopLeft, TopRight, BottomLeft, BottomRight string
}

// roundedBorder is the box with rounded corners docs/TUI_DESIGN.md draws every
// bubble with.
func roundedBorder() border {
	return border{
		Top: "─", Bottom: "─",
		Left: "│", Right: "│",
		TopLeft: "╭", TopRight: "╮",
		BottomLeft: "╰", BottomRight: "╯",
	}
}

// bubbleFrame is the two columns a bubble's border and padding take on each
// side, which is what a wrapped line has to fit inside.
const bubbleFrame = 2 * (1 + bubblePadding)

// bubble is one box of talk in the transcript.
type bubble struct {
	// edges are the border glyphs, which are the rounded border with the
	// person's thick left edge where the design asked for one.
	edges border
	// frame is how the border itself is drawn.
	frame style
	// fill is how the blanks inside the box are drawn, which is what makes the
	// person's bubble a solid shape and the agent's an outline.
	fill style
	// leaningRight puts the box against the right-hand edge of the frame, which
	// is where the person's own words sit.
	leaningRight bool
}

// personBubble is the box a message the person typed is drawn in: a solid
// accent-filled shape against the right-hand edge, with the thick left edge that
// docs/TUI_DESIGN.md drew as a bar beside the message kept as the bubble's own.
func personBubble() bubble {
	edges := roundedBorder()
	edges.Left = string(personBarGlyph)
	return bubble{edges: edges, frame: styleChip, fill: styleChip, leaningRight: true}
}

// agentBubble is the box the agent's reply is drawn in: an outline against the
// left-hand edge, so that the reply itself is the loudest thing on the row.
func agentBubble() bubble {
	return bubble{edges: roundedBorder(), frame: styleDim, fill: styleNormal}
}

// bubbleWidth is the widest box the transcript will draw: the frame less its
// margins, and never wider than a line is comfortable to read.
func (screen *Screen) bubbleWidth() int {
	widest := screen.transcriptColumns() - 2*marginColumns
	if widest > widestTranscript+bubbleFrame {
		widest = widestTranscript + bubbleFrame
	}
	return max(widest, bubbleFrame+1)
}

// bubbleRows draws already-styled lines inside a box. The box is only as wide as
// the widest line in it, so a two-word answer gets a two-word bubble, and it is
// never wider than the transcript allows.
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

	drawn := []string{screen.bubbleEdgeRow(shape, widest, indent, true)}
	for _, line := range lines {
		drawn = append(drawn, screen.bubbleBodyRow(shape, line, widest, indent))
	}
	return append(drawn, screen.bubbleEdgeRow(shape, widest, indent, false))
}

// bubbleEdgeRow draws the top or the bottom of a box, corners and all.
func (screen *Screen) bubbleEdgeRow(shape bubble, widest int, indent int, top bool) string {
	left, along, right := shape.edges.TopLeft, shape.edges.Top, shape.edges.TopRight
	if !top {
		left, along, right = shape.edges.BottomLeft, shape.edges.Bottom, shape.edges.BottomRight
	}
	line := row{}
	line.blanks(indent)
	line.add(shape.frame, left+strings.Repeat(along, widest+2*bubblePadding)+right)
	return line.render(screen.colors)
}

// bubbleBodyRow draws one line inside a box, padded so that the right-hand edge
// of the box stays straight.
func (screen *Screen) bubbleBodyRow(shape bubble, line row, widest int, indent int) string {
	full := row{}
	full.blanks(indent)
	full.add(shape.frame, shape.edges.Left)
	full.padWith(shape.fill, bubblePadding)
	for _, piece := range line.spans {
		full.addSpan(piece)
	}
	full.padWith(shape.fill, widest-line.width+bubblePadding)
	full.add(shape.frame, shape.edges.Right)
	return full.render(screen.colors)
}

// pillRows draws one tool call as a small filled pill: the arrow, the tool, its
// main argument, and its short summary, and never the result text.
func (screen *Screen) pillRows(text string, focused bool) []string {
	arrow := styleChip
	if focused {
		arrow = styleReverse
	}
	inner := screen.bubbleWidth() - bubbleFrame
	wrapped := wrapText(withoutLeadingArrow(text), max(inner-2, 1))
	widest := 0
	for _, line := range wrapped {
		widest = max(widest, displayWidth(line))
	}

	drawn := []string{}
	for number, line := range wrapped {
		pill := row{}
		pill.blanks(marginColumns + gutterColumns)
		pill.padWith(styleChip, bubblePadding)
		if number == 0 {
			pill.add(arrow, string(toolArrowGlyph)+" ")
		} else {
			pill.padWith(styleChip, 2)
		}
		pill.add(styleChip, line)
		pill.padWith(styleChip, widest-displayWidth(line)+bubblePadding)
		drawn = append(drawn, pill.render(screen.colors))
	}
	return drawn
}

// withoutLeadingArrow takes the arrow off the front of a line that already
// carries one. The running program writes the line for a call in flight
// beginning with the design's own arrow, and the pill draws that arrow itself,
// so a line handed over with one would otherwise be drawn with two.
func withoutLeadingArrow(text string) string {
	trimmed := strings.TrimLeft(text, " ")
	if !strings.HasPrefix(trimmed, string(toolArrowGlyph)) {
		return text
	}
	return strings.TrimLeft(strings.TrimPrefix(trimmed, string(toolArrowGlyph)), " ")
}
