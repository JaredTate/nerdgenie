package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theTestPillLine is the tool line the program sends for a finished call, with
// the result's id straight after the separator, as internal/loop writes it.
const theTestPillLine = "▸ shell npm test · r27 tests: all passing"

// theCallInFlightLine is the same call before it has come back: no separator,
// no id, nothing to expand yet.
const theCallInFlightLine = "▸ shell npm test"

// aScreenWithAPill is a hundred-and-twenty-column screen attached to a program
// that has reported a running task, holding the person's ask, one finished
// tool call carrying its result id, and the agent's reply.
func aScreenWithAPill() (*Screen, *recordingLink) {
	screen, _ := newTestScreen(120, 40)
	link := &recordingLink{}
	screen.link = link
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithAPlan())
	typeAndSend(screen, theTaskAsk)
	send(screen, aToolLine(theTestPillLine))
	send(screen, aReply("The tests pass, so the draft is ready to post."))
	return screen, link
}

// pressTab gives the screen Tab, and pressShiftTab gives it Shift+Tab, which a
// terminal reports as Tab with the shift key held.
func pressTab(screen *Screen) {
	screen.Update(tea.KeyPressMsg{Code: tea.KeyTab})
}

func pressShiftTab(screen *Screen) {
	screen.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
}

// showsSent is every show the screen has asked the program for.
func showsSent(link *recordingLink) []contract.SocketEnvelope {
	shows := []contract.SocketEnvelope{}
	for _, envelope := range link.sent {
		if envelope.Type == contract.SocketShow {
			shows = append(shows, envelope)
		}
	}
	return shows
}

// TestTheInteractionHintsNameTheFourKeys holds the seam with the footer: the
// words for the keys this side owns are one list the status strip reads.
func TestTheInteractionHintsNameTheFourKeys(t *testing.T) {
	hints := interactionHints()
	for _, wanted := range []string{"tab focus", "↵ expand", "^B panel", "⇞⇟ scroll"} {
		if !slices.Contains(hints, wanted) {
			t.Errorf("the interaction hints %q do not list %q", hints, wanted)
		}
	}
}

func TestTabFocusesThePillAndShiftTabWalksBackAndTheFocusWraps(t *testing.T) {
	screen, _ := aScreenWithAPill()
	send(screen, aToolLine("▸ read notes.md · r28 read: 12 lines"))

	if screen.focusedBlock() >= 0 {
		t.Fatal("something is focused before Tab was ever pressed")
	}
	pressTab(screen)
	first := screen.focusedBlock()
	if first < 0 || screen.blocks[first].text != theTestPillLine {
		t.Fatalf("the first Tab focused block %d rather than the oldest pill", first)
	}
	pressTab(screen)
	second := screen.focusedBlock()
	if second <= first {
		t.Fatalf("the second Tab focused block %d, and it should move on to the newer pill after block %d", second, first)
	}
	pressTab(screen)
	if screen.focusedBlock() != first {
		t.Errorf("a third Tab past the last pill focused block %d, and the focus wraps round to the first", screen.focusedBlock())
	}
	pressShiftTab(screen)
	if screen.focusedBlock() != second {
		t.Errorf("Shift+Tab from the first pill focused block %d, and it wraps back to the last", screen.focusedBlock())
	}
}

func TestACallStillInFlightAndARecordLineAreNotFocusable(t *testing.T) {
	screen, _ := screenWithLink()
	screen.Update(linkMessage{up: true})
	send(screen, aToolLine(theCallInFlightLine))
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldRecordLine: "task 17 started",
	}})

	pressTab(screen)
	if screen.focusedBlock() >= 0 {
		t.Errorf("Tab focused block %d, and neither a call with no result yet nor a record line can be opened", screen.focusedBlock())
	}
}

func TestTheFocusedPillIsDrawnApartAndTheGoldenFrameShowsIt(t *testing.T) {
	screen, _ := aScreenWithAPill()
	before := screen.frame()
	pressTab(screen)
	after := screen.frame()
	if before == after {
		t.Fatal("Tab changed nothing on the frame, so the person cannot see what is focused")
	}
	if plainText(before) != plainText(after) {
		t.Error("focusing a pill changed the words on the frame, and it may change only the colours")
	}
	testkit.Golden(t, "focused-pill-120x40.txt", []byte(screen.frame()))
}

func TestEscClearsTheFocusAndTheNextEscGoesOnToStop(t *testing.T) {
	screen, link := aScreenWithAPill()
	pressTab(screen)
	pressKey(screen, tea.KeyEsc)
	if screen.focusedBlock() >= 0 {
		t.Fatal("Esc did not clear the focus")
	}
	if stopsOnEscape(screen, link) != true {
		t.Error("with nothing focused, Esc no longer stops the running task")
	}
}

func TestFocusDoesNotStealTheLetters(t *testing.T) {
	screen, _ := aScreenWithAPill()
	pressTab(screen)
	typeWord(screen, "carry on")
	if screen.input.text() != "carry on" {
		t.Errorf("the input box holds %q after typing with a pill focused, and letters always go to the box", screen.input.text())
	}
	if screen.focusedBlock() < 0 {
		t.Error("typing letters took the focus away, and only Esc does that")
	}
}

func TestAPanelRowWithATargetIsFocusedAfterThePillsAndDrawnReversed(t *testing.T) {
	screen, _ := aScreenWithAPill()
	items := screen.focusablesFrom([]int{-1, 0, 0, 1, 1, -1}, []string{"", "job:4", "task:t3", ""}, 6)
	wanted := []focusable{{block: 0, line: -1}, {block: 1, line: -1}, {block: -1, line: 1, target: "job:4"}, {block: -1, line: 2, target: "task:t3"}}
	if !slices.Equal(items, wanted) {
		t.Errorf("the focusable items are %+v, want %+v", items, wanted)
	}

	send(screen, aStatusWithANamedJob())
	lines := screen.panelLines()
	plain := screen.panelRows(len(lines))[0]
	focused := screen.focusedPanelRow(0)
	if plain == focused {
		t.Error("the focused panel row is drawn exactly like the plain one")
	}
	if plainText(plain) != plainText(focused) {
		t.Errorf("focus changed the panel row's words from %q to %q", plainText(plain), plainText(focused))
	}
	if !strings.Contains(focused, reverseCode) {
		t.Errorf("the focused panel row %q does not carry the reverse code on its first glyph", focused)
	}
}

// TestFocusablesWorkWithAllEmptyTargets holds that the panel stub, which names
// no target on any line, adds nothing to the focus list and breaks nothing.
func TestFocusablesWorkWithAllEmptyTargets(t *testing.T) {
	screen, _ := aScreenWithAPill()
	items := screen.focusables()
	if len(items) != 1 || items[0].block < 0 {
		t.Errorf("the focusable items are %+v, and with no panel targets they are the one pill alone", items)
	}
}

func TestCtrlBPutsThePanelAwayAndBringsItBack(t *testing.T) {
	screen, _ := aScreenWithAPill()
	pressWithControl(screen, 'b')
	if screen.showsPanel() {
		t.Fatal("Ctrl+B did not put the panel away")
	}
	testkit.Golden(t, "panel-hidden-120x40.txt", []byte(screen.frame()))
	pressWithControl(screen, 'b')
	if !screen.showsPanel() {
		t.Error("a second Ctrl+B did not bring the panel back")
	}
}
