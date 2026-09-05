package tui

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theShownText is what the program sends back for r27: the call, its
// arguments, a blank line, and the stored result, as the socket contract says.
const theShownText = "call: shell\ncommand: npm test\n\nresult:\n> nerdgenie@0.1.0 test\n> go test ./...\n\nok  internal/tui 1.7s\n51 passing"

// aShownResult is the program's answer to a show for one result of task 17.
func aShownResult(id string, text string) contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketShown, Text: text, Fields: map[string]string{
		"id": id, "task": "17",
	}}
}

// rowsUnderThePill is every row of the frame between the pill's own row and
// the reply under it, with the paint out to the panel taken off, which is
// where whatever hangs under the pill is drawn.
func rowsUnderThePill(screen *Screen) []string {
	rows := strings.Split(plainText(screen.frame()), "\n")
	from := -1
	for at, drawn := range rows {
		if strings.Contains(drawn, "tests: all passing") {
			from = at
		}
	}
	if from < 0 {
		return nil
	}
	under := []string{}
	for _, drawn := range rows[from+1:] {
		if strings.Contains(drawn, string(cardBarGlyph)) {
			break
		}
		under = append(under, strings.TrimRight(string([]rune(drawn)[:screen.transcriptColumns()]), " "))
	}
	return under
}

// expandTheFocusedPill focuses the one pill and presses Enter on it.
func expandTheFocusedPill(screen *Screen) {
	pressTab(screen)
	pressKey(screen, tea.KeyEnter)
}

func TestEnterOnAFocusedPillAsksForItsTextOnceAndSaysItIsFetching(t *testing.T) {
	screen, link := aScreenWithAPill()
	expandTheFocusedPill(screen)

	shows := showsSent(link)
	if len(shows) != 1 {
		t.Fatalf("opening a pill sent %d shows, and it sends exactly one", len(shows))
	}
	if shows[0].Fields["id"] != "r27" || shows[0].Fields["task"] != "17" {
		t.Errorf("the show carried the fields %v, and it names the result r27 and the task 17", shows[0].Fields)
	}
	if !strings.Contains(plainText(screen.frame()), "fetching r27…") {
		t.Errorf("the frame does not say the text is being fetched:\n%s", plainText(screen.frame()))
	}
	testkit.Golden(t, "fetching-pill-120x40.txt", []byte(screen.frame()))

	pressKey(screen, tea.KeyEnter)
	if strings.Contains(plainText(screen.frame()), "fetching") {
		t.Error("a second Enter did not fold the pill back up")
	}
	pressKey(screen, tea.KeyEnter)
	if len(showsSent(link)) != 1 {
		t.Errorf("opening the same pill again sent %d shows in all, and an id is never asked for twice", len(showsSent(link)))
	}
}

func TestTheShownTextIsDrawnUnderThePillIndentedAndLabelled(t *testing.T) {
	screen, _ := aScreenWithAPill()
	expandTheFocusedPill(screen)
	send(screen, aShownResult("r27", theShownText))

	under := rowsUnderThePill(screen)
	if len(under) == 0 || strings.Contains(strings.Join(under, "\n"), "fetching") {
		t.Fatalf("the text did not take the place of the fetching line:\n%s", strings.Join(under, "\n"))
	}
	for _, wanted := range []string{"call: shell", "command: npm test", "result:", "51 passing"} {
		if !strings.Contains(strings.Join(under, "\n"), wanted) {
			t.Errorf("the text under the pill does not hold %q:\n%s", wanted, strings.Join(under, "\n"))
		}
	}
	for _, drawn := range under {
		if drawn != "" && !strings.HasPrefix(drawn, strings.Repeat(" ", marginColumns+gutterColumns+2)) {
			t.Errorf("the line %q is not indented two columns past the pill", drawn)
		}
	}
	testkit.Golden(t, "expanded-pill-120x40.txt", []byte(screen.frame()))
}

func TestTheLabelsAreDimAndTheWordsAfterThemAreNot(t *testing.T) {
	screen, _ := aScreenWithAPill()
	screen.colors = newTheme(func(name string) string { return map[string]string{"COLORTERM": "truecolor"}[name] })
	labelled := screen.labelledLine("call: shell")
	if !strings.Contains(labelled, screen.colors.wrap(styleDim, "call:")) || !strings.Contains(labelled, screen.colors.wrap(styleNormal, " shell")) {
		t.Errorf("the labelled line %q does not draw the label dim and the words plain", labelled)
	}
}

func TestALongTextIsCappedAtSixtyLinesAndSaysHowManyMore(t *testing.T) {
	screen, _ := aScreenWithAPill()
	expandTheFocusedPill(screen)
	lines := make([]string, 100)
	for at := range lines {
		lines[at] = "line " + strconv.Itoa(at+1)
	}
	send(screen, aShownResult("r27", strings.Join(lines, "\n")))

	drawn := screen.blockLines(screen.blocks[1])
	pill := len(screen.pillRows(theTestPillLine, false))
	if len(drawn) != pill+maxExpandedLines {
		t.Fatalf("the open pill is drawn on %d rows, and it is the pill's %d and sixty of text", len(drawn), pill)
	}
	if last := plainText(drawn[len(drawn)-1]); !strings.Contains(last, "… and 41 more lines") {
		t.Errorf("the last line is %q, and it says how many lines were left off", last)
	}
}

func TestAnErrorAnsweringAShowIsDrawnUnderThePillAndNotAsACard(t *testing.T) {
	screen, _ := aScreenWithAPill()
	expandTheFocusedPill(screen)
	blocks := len(screen.blocks)
	send(screen, contract.SocketEnvelope{Type: contract.SocketError, Text: "there is no result r27 in task 17", Fields: map[string]string{"id": "r27", "task": "17"}})

	if len(screen.blocks) != blocks {
		t.Errorf("the error made %d blocks out of %d, and an error answering a show is drawn under the pill rather than as a card", len(screen.blocks), blocks)
	}
	if under := strings.Join(rowsUnderThePill(screen), "\n"); !strings.Contains(under, "there is no result r27 in task 17") {
		t.Errorf("the error's words are not under the pill:\n%s", under)
	}
}

func TestAnErrorAboutSomethingElseIsStillACard(t *testing.T) {
	screen, _ := aScreenWithAPill()
	blocks := len(screen.blocks)
	send(screen, contract.SocketEnvelope{Type: contract.SocketError, Text: "the model is not answering", Fields: map[string]string{"id": "r99"}})
	if len(screen.blocks) != blocks+1 {
		t.Errorf("an error for a result nobody asked for made %d blocks out of %d, and it is an ordinary error card", len(screen.blocks), blocks)
	}
}

func TestEscFoldsEveryOpenPillOnceTheFocusIsGoneAndThenStops(t *testing.T) {
	screen, link := aScreenWithAPill()
	expandTheFocusedPill(screen)
	pressKey(screen, tea.KeyEsc)
	if len(screen.expanded) != 1 {
		t.Fatal("the first Esc, which clears the focus, folded the pill as well")
	}
	pressKey(screen, tea.KeyEsc)
	if len(screen.expanded) != 0 {
		t.Fatal("the second Esc did not fold the open pill")
	}
	if !stopsOnEscape(screen, link) {
		t.Error("the third Esc, with nothing left to close, did not stop the task")
	}
}

func TestACallStillInFlightCannotBeOpened(t *testing.T) {
	screen, link := aScreenWithAPill()
	send(screen, aToolLine(theCallInFlightLine))
	screen.togglePillAt(len(screen.blocks) - 1)
	if len(screen.expanded) != 0 || len(showsSent(link)) != 0 {
		t.Error("a pill with no result id yet was opened or asked for")
	}
}

func TestTheShownTextsAreCappedAndTheOldestOnTheTranscriptGoesFirst(t *testing.T) {
	screen, _ := aScreenWithAPill()
	send(screen, aToolLine("▸ read notes.md · r28 read: 12 lines"))
	for number := 1; number <= maxShownTexts; number++ {
		screen.rememberShown("r"+strconv.Itoa(number), "text "+strconv.Itoa(number))
	}
	screen.rememberShown("r999", "one more")
	if len(screen.shown) != maxShownTexts {
		t.Errorf("the screen holds %d shown texts, and it keeps at most %d", len(screen.shown), maxShownTexts)
	}
	if _, kept := screen.shown["r27"]; kept {
		t.Error("r27, the oldest pill on the transcript, was kept while something else was let go")
	}
	if _, kept := screen.shown["r28"]; !kept {
		t.Error("r28, still on the transcript after r27, was let go")
	}
}

func TestAClearForgetsTheOpenPillsAndTheFocus(t *testing.T) {
	screen, _ := aScreenWithAPill()
	expandTheFocusedPill(screen)
	send(screen, contract.SocketEnvelope{Type: contract.SocketReply, Text: "cleared", Clear: true})
	if len(screen.expanded) != 0 || screen.focusAt != noFocus {
		t.Error("a clear left a pill open or the focus set, and there is nothing on the screen for either")
	}
}

func TestOpeningAPillWithNoLinkFoldsItAndSaysWhy(t *testing.T) {
	screen, _ := newTestScreen(120, 40)
	screen.remember(block{kind: blockTool, text: theTestPillLine})
	screen.togglePillAt(0)
	if len(screen.expanded) != 0 {
		t.Error("the pill stayed open with no link to ask over, and it would say fetching for ever")
	}
	if last := screen.blocks[len(screen.blocks)-1]; last.kind != blockCard || last.shown.kind != cardError {
		t.Error("nothing told the person why the pill could not be opened")
	}
}
