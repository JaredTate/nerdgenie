package tui

import (
	"strconv"
	"strings"
)

// The three colours docs/TUI_DESIGN.md names, each written twice so that it
// reads on a dark terminal and on a light one: a calm blue accent, a grey with
// enough contrast to read, and a muted red for failures. They are twenty-four
// bit escape codes, which is what every terminal Coeus supports understands.
const (
	accentOnDark  = "\x1b[38;2;59;130;246m"
	accentOnLight = "\x1b[38;2;29;78;216m"
	dimOnDark     = "\x1b[38;2;156;163;175m"
	dimOnLight    = "\x1b[38;2;107;114;128m"
	errorOnDark   = "\x1b[38;2;248;113;113m"
	errorOnLight  = "\x1b[38;2;185;28;28m"
	boldCode      = "\x1b[1m"
	reverseCode   = "\x1b[7m"
	resetCode     = "\x1b[0m"
)

// style is one of the six ways a piece of the frame can be drawn. There is one
// accent, one dim, and one error colour and nothing else, so that the reply is
// the loudest thing on the screen.
type style int

const (
	// styleNormal is the terminal's own foreground, and is what a reply and a
	// person's message are drawn in.
	styleNormal style = iota
	// styleDim is the grey every quiet thing is drawn in: the header, the rules,
	// the tool lines, and the status strip.
	styleDim
	// styleAccent is the one colour that draws the eye: the prompt glyph, the
	// bar beside a person's message, and a running task.
	styleAccent
	// styleError is the muted red a failure is drawn in.
	styleError
	// styleBold is the terminal's foreground made bold, which is what light
	// markdown uses for bold text and headings.
	styleBold
	// styleReverse swaps the foreground and the background, which is how the
	// cursor is drawn when it sits inside the text rather than at the end.
	styleReverse
)

// theme turns a style into the escape codes that draw it. It answers two
// questions once, when the screen is built: whether the person asked for no
// colour at all, and whether the terminal has a light background.
type theme struct {
	// colored is false when the NO_COLOR environment variable is set, and then
	// every style but the cursor draws as plain text.
	colored bool
	// light is true when the terminal's background is a light one, which picks
	// the darker half of each colour pair.
	light bool
}

// newTheme reads the environment for the two questions colour depends on.
func newTheme(environment func(string) string) theme {
	return theme{
		colored: environment("NO_COLOR") == "",
		light:   hasLightBackground(environment),
	}
}

// wrap puts the escape codes for a style around a piece of text. The cursor is
// the one style that survives NO_COLOR, because reversing the foreground and the
// background is not a colour and the person still has to see where they are
// typing.
func (colors theme) wrap(chosen style, text string) string {
	if text == "" {
		return ""
	}
	if chosen == styleReverse {
		return reverseCode + text + resetCode
	}
	if !colors.colored || chosen == styleNormal {
		return text
	}
	return colors.code(chosen) + text + resetCode
}

// code is the escape sequence one style begins with.
func (colors theme) code(chosen style) string {
	switch chosen {
	case styleDim:
		if colors.light {
			return dimOnLight
		}
		return dimOnDark
	case styleAccent:
		if colors.light {
			return accentOnLight
		}
		return accentOnDark
	case styleError:
		if colors.light {
			return errorOnLight
		}
		return errorOnDark
	case styleBold:
		return boldCode
	default:
		return ""
	}
}

// hasLightBackground reads COLORFGBG, which terminals set to the foreground and
// the background colour numbers separated by a semicolon. A background of seven
// or fifteen is one of the two light greys, and anything else is treated as
// dark, because dark is the safer guess when nothing says.
func hasLightBackground(environment func(string) string) bool {
	setting := environment("COLORFGBG")
	if setting == "" {
		return false
	}
	parts := strings.Split(setting, ";")
	background, err := strconv.Atoi(strings.TrimSpace(parts[len(parts)-1]))
	if err != nil {
		return false
	}
	return background == 7 || background == 15
}
