// The row of key hints under the status strip, each key drawn as a small
// keycap with its verb in quiet letters after it, is OpenCode's footer, read
// at ~/Code/opencode/packages/tui/src/routes/session/footer.tsx and the
// keymap hints its prompt draws. Nothing was copied: that is TypeScript
// drawing through a layout engine, and this is Go writing one row.

package tui

import "strings"

// extraHints answers the hints the other half of this screen adds, such as
// "tab focus" and "↵ expand", each as the key, a blank, and the verb. It is
// a package-level variable rather than a call into focus.go because the two
// halves are built apart: the merge sets extraHints = interactionHints, and
// until then it answers nothing.
var extraHints = func() []string { return nil }

// maxFooterHints is the most hints the footer will draw, so that a list
// answered by mistake cannot make the row a loop worth worrying about; the
// row is cut to the width anyway.
const maxFooterHints = 16

// The hints the screen's own keys get, in the words the footer draws.
const (
	sendHint    = "↵ send"
	newlineHint = "^J newline"
	stopHint    = "esc stop"
	quitHint    = "^C quit"
)

// hintsRow draws the key hints: each key as a keycap and its verb dim after
// it, two blanks apart, the screen's own first and the other half's after
// them, and whole hints dropped from the right until the row fits. It sits
// under the input box, and the status strip under it is the frame's last
// row, because the strip reports the scroll position and only the transcript,
// drawn after the input box, knows how far there is to scroll.
func (screen *Screen) hintsRow() string {
	line := row{}
	line.blanks(marginColumns)
	room := screen.width - marginColumns
	for at, hint := range screen.hints() {
		piece := hintSpans(hint)
		if at > 0 {
			piece = append([]span{{style: styleNormal, text: "  "}}, piece...)
		}
		if line.width+spansWidth(piece) > room {
			break
		}
		for _, part := range piece {
			line.addSpan(part)
		}
	}
	return line.render(screen.colors)
}

// hints are the hints in the order they are drawn: the ones for what the
// screen is waiting on, and then whatever the other half adds, never more
// than maxFooterHints in all.
func (screen *Screen) hints() []string {
	listed := append(screen.ownHints(), extraHints()...)
	if len(listed) > maxFooterHints {
		listed = listed[:maxFooterHints]
	}
	return listed
}

// ownHints are the screen's own keys, which change with what the screen is
// waiting for: a masked prompt, the palette, the reason for a refusal, a card
// with single-key answers, or the input box, where Escape is offered only
// while there is something for it to stop.
func (screen *Screen) ownHints() []string {
	switch {
	case screen.input.secret:
		return []string{sendHint, "esc cancel", "nothing is shown"}
	case screen.paletteOpen:
		return []string{"tab complete", "↵ run", "esc close"}
	case screen.askingWhyNot:
		return []string{"↵ send the reason", "esc go back"}
	case screen.focusedCard() != nil:
		return screen.focusedCard().hints()
	case screen.busy():
		return []string{sendHint, newlineHint, stopHint, quitHint}
	default:
		return []string{sendHint, newlineHint, quitHint}
	}
}

// hints for a card are the single keys that answer it.
func (shown card) hints() []string {
	switch shown.kind {
	case cardPreview:
		return []string{"a approve", "A always", "r reject"}
	case cardHandoff:
		return []string{"a finished", "r give up"}
	default:
		return []string{"↵ answer", "esc dismiss"}
	}
}

// hintSpans draws one hint: the key in a keycap, bold white on the card fill
// with a blank of padding on each side, and the verb dim after it. A hint
// that is words alone, such as "nothing is shown", is drawn dim with no key.
func hintSpans(hint string) []span {
	key, verb := keyAndVerb(hint)
	if !looksLikeAKey(key) {
		return []span{{style: styleDim, text: hint}}
	}
	spans := []span{{style: styleKey, text: " " + key + " "}}
	if verb != "" {
		spans = append(spans, span{style: styleDim, text: " " + verb})
	}
	return spans
}

// keyAndVerb reads a hint as its first word, the key, and the rest, the verb.
func keyAndVerb(hint string) (string, string) {
	key, verb, _ := strings.Cut(strings.TrimSpace(hint), " ")
	return key, strings.TrimSpace(verb)
}

// looksLikeAKey says whether the first word of a hint names a key rather
// than beginning a sentence: a single character, a control chord such as ^C,
// a named key such as esc or tab, or a single letter answer.
func looksLikeAKey(word string) bool {
	if len([]rune(word)) <= 2 || strings.HasPrefix(word, "^") {
		return true
	}
	switch strings.ToLower(word) {
	case "esc", "tab", "enter", "space", "ctrl", "shift":
		return true
	}
	return false
}

// spansWidth is how many columns a run of spans takes.
func spansWidth(spans []span) int {
	width := 0
	for _, piece := range spans {
		width += displayWidth(piece.text)
	}
	return width
}
