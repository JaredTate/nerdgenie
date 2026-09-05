package tui

import "testing"

// TestThePanelPutAwayWithItsKeyIsNotDrawn holds the seam between the key that
// hides the side panel and the geometry that draws it: a wide screen with the
// panel put away draws none, and the transcript takes the whole width.
func TestThePanelPutAwayWithItsKeyIsNotDrawn(t *testing.T) {
	screen, _ := newTestScreen(120, 40)
	if !screen.showsPanel() {
		t.Fatal("a screen of 120 columns shows the panel, and this one does not")
	}
	screen.panelHidden = true
	if screen.showsPanel() {
		t.Error("the panel is drawn while it is put away")
	}
	if screen.transcriptColumns() != 120 {
		t.Errorf("the transcript is %d columns wide with the panel put away, want all 120", screen.transcriptColumns())
	}
}

// TestEveryPanelLineHasATarget holds the seam between the panel's drawing and
// a click on it: panelTargets answers one entry per line the panel draws, so a
// row index on the screen maps to the job or task under it, or to nothing.
func TestEveryPanelLineHasATarget(t *testing.T) {
	screen, _ := newTestScreen(120, 40)
	screen.readStatus(map[string]string{"model": "opus", "task": "17", "taskAsk": "post the tweet", "plan": "[ ] draft it\n[ ] post it"})
	if lines, targets := screen.panelLines(), screen.panelTargets(); len(lines) != len(targets) {
		t.Errorf("the panel draws %d lines and names %d targets, and the two must line up", len(lines), len(targets))
	}
}

// TestAFocusedPillIsDrawnDifferently holds the seam between the keyboard focus
// and the pill's drawing: the same text drawn focused and unfocused differs.
func TestAFocusedPillIsDrawnDifferently(t *testing.T) {
	screen, _ := newTestScreen(120, 40)
	plain := screen.pillRows("shell npm test · r27 tests: all passing", false)
	focused := screen.pillRows("shell npm test · r27 tests: all passing", true)
	if len(plain) == 0 || len(plain) != len(focused) {
		t.Fatalf("the pill is %d rows plain and %d focused, want the same count", len(plain), len(focused))
	}
	if plain[0] == focused[0] {
		t.Error("the focused pill is drawn exactly like the plain one, so a person cannot see where the focus is")
	}
	if plainText(plain[0]) != plainText(focused[0]) {
		t.Errorf("focus changed the pill's words from %q to %q, and it may change only the colours", plainText(plain[0]), plainText(focused[0]))
	}
}
