package tui

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theRecordText is what the program prints for task 17's record.
const theRecordText = "# task 17\n\nask: Post a tweet about the DigiByte anniversary. Use the product notes and keep it under 280 characters.\n\nplan:\n[x] the product notes are read\n[x] a draft under 280 characters is written\n[ ] the tweet is posted\n\nsituation:\nnotes: read, 2,100 characters\ndraft: 214 characters\n\nresults:\nr27 shell npm test: tests: all passing"

// aShownRecord is the program's answer to a show for a task's or a job's
// record, carrying back the field it was asked with.
func aShownRecord(field string, name string, text string) contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketShown, Text: text, Fields: map[string]string{field: name}}
}

// aNumberedRecord is a record of one line per number, for scrolling through.
func aNumberedRecord(lines int) string {
	numbered := make([]string, lines)
	for at := range numbered {
		numbered[at] = "line " + strconv.Itoa(at+1)
	}
	return strings.Join(numbered, "\n")
}

// overlayOnTheFrame is the rows between the two rules left of the panel, with
// the blanks on the end taken off, which is where the overlay is drawn.
func overlayOnTheFrame(screen *Screen) []string {
	rows := strings.Split(plainText(screen.frame()), "\n")
	between := rows[2 : len(rows)-2-len(screen.inputRows())]
	drawn := make([]string, len(between))
	for at, line := range between {
		letters := []rune(line)
		if len(letters) > screen.transcriptColumns() {
			letters = letters[:screen.transcriptColumns()]
		}
		drawn[at] = strings.TrimRight(string(letters), " ")
	}
	return drawn
}

// firstTextLine is the first line of the overlay's text, which is the row
// under its title.
func firstTextLine(screen *Screen) string {
	return strings.TrimSpace(overlayOnTheFrame(screen)[1])
}

func TestOpeningATargetAsksForTheRecordItNames(t *testing.T) {
	screen, link := aScreenWithAPill()
	screen.openTarget("task:t3")
	screen.openTarget("job:4")
	screen.clickedOn(hit{block: -1, target: "task:19"})
	screen.openTarget("nonsense")
	screen.openTarget("task:")

	shows := showsSent(link)
	if len(shows) != 3 {
		t.Fatalf("the targets sent %d shows, and the three that name a task or a job send one each: %+v", len(shows), shows)
	}
	if shows[0].Fields["task"] != "t3" || shows[1].Fields["job"] != "4" || shows[2].Fields["task"] != "19" {
		t.Errorf("the shows carried %v, %v and %v", shows[0].Fields, shows[1].Fields, shows[2].Fields)
	}
	if screen.overlayOpen() {
		t.Error("the overlay opened before the program answered")
	}
}

func TestTheRecordIsDrawnOverTheTranscriptWhenTheAnswerArrives(t *testing.T) {
	screen, _ := aScreenWithAPill()
	send(screen, aShownRecord("task", "17", theRecordText))

	drawn := overlayOnTheFrame(screen)
	if strings.TrimSpace(drawn[0]) != "task 17" {
		t.Errorf("the overlay's first row is %q, and it is the title", drawn[0])
	}
	frame := plainText(screen.frame())
	for _, wanted := range []string{"ask: Post a tweet", "[ ] the tweet is posted", "r27 shell npm test", "TASK 17 · Post a tweet a…"} {
		if !strings.Contains(frame, wanted) {
			t.Errorf("the frame does not hold %q, and the record is drawn with the panel still beside it:\n%s", wanted, frame)
		}
	}
	if strings.Contains(strings.Join(drawn, "\n"), "╭") {
		t.Error("a bubble of the transcript shows through the overlay")
	}
}

func TestTheOverlayTitleIsBoldAndTheGoldenFrameShowsIt(t *testing.T) {
	screen, _ := aScreenWithAPill()
	send(screen, aShownRecord("task", "17", theRecordText))
	testkit.Golden(t, "overlay-120x40.txt", []byte(screen.frame()))

	colored := New(Options{Clock: screen.clock, Width: 120, Height: 40, Environment: func(name string) string {
		return map[string]string{"COLORTERM": "truecolor"}[name]
	}})
	send(colored, aShownRecord("task", "17", theRecordText))
	if !strings.Contains(colored.frame(), colored.colors.wrap(styleBold, "task 17")) {
		t.Error("the overlay's title is not drawn bold")
	}
}

func TestAJobsRecordIsTitledWithItsNameWhenTheScreenKnowsIt(t *testing.T) {
	screen, _ := aScreenWithAPill()
	send(screen, aStatusWithANamedJob())
	send(screen, aShownRecord("job", "4", "# job 4"))
	if title := strings.TrimSpace(overlayOnTheFrame(screen)[0]); title != "job 4 · Tater Tots Tetris" {
		t.Errorf("the overlay is titled %q, want %q", title, "job 4 · Tater Tots Tetris")
	}
	send(screen, aShownRecord("job", "9", "# job 9"))
	if title := strings.TrimSpace(overlayOnTheFrame(screen)[0]); title != "job 9" {
		t.Errorf("the overlay for a job the screen has no name for is titled %q, want %q", title, "job 9")
	}
}

func TestTheOverlayScrollsWithTheKeysAndTheWheelAndStopsAtItsEnds(t *testing.T) {
	screen, _ := aScreenWithAPill()
	send(screen, aShownRecord("task", "17", aNumberedRecord(100)))
	scrolledBefore := screen.scrollBack

	pressKey(screen, tea.KeyDown)
	if line := firstTextLine(screen); line != "line 2" {
		t.Errorf("after Down the first line is %q, want %q", line, "line 2")
	}
	pressKey(screen, tea.KeyUp)
	pressKey(screen, tea.KeyUp)
	if line := firstTextLine(screen); line != "line 1" {
		t.Errorf("after two Ups the first line is %q, and the overlay stops at its top", line)
	}
	pressKey(screen, tea.KeyPgDown)
	if line := firstTextLine(screen); line != "line "+strconv.Itoa(screen.pageRows()+1) {
		t.Errorf("after Page Down the first line is %q, want line %d", line, screen.pageRows()+1)
	}
	pressKey(screen, tea.KeyPgUp)
	if line := firstTextLine(screen); line != "line 1" {
		t.Errorf("after Page Up the first line is %q, want %q", line, "line 1")
	}
	wheel(screen, tea.MouseWheelDown)
	if line := firstTextLine(screen); line != "line "+strconv.Itoa(wheelRows+1) {
		t.Errorf("after one wheel down the first line is %q, want line %d", line, wheelRows+1)
	}
	wheel(screen, tea.MouseWheelUp)
	if line := firstTextLine(screen); line != "line 1" {
		t.Errorf("after one wheel up the first line is %q, want %q", line, "line 1")
	}
	for range 200 {
		pressKey(screen, tea.KeyPgDown)
	}
	drawn := overlayOnTheFrame(screen)
	if !strings.Contains(strings.Join(drawn, "\n"), "line 100") || strings.TrimSpace(drawn[1]) == "line 100" {
		t.Errorf("after paging past the end the overlay shows:\n%s\nand it stops with the last line at the bottom", strings.Join(drawn, "\n"))
	}
	if screen.scrollBack != scrolledBefore {
		t.Error("scrolling the overlay moved the transcript behind it")
	}
}

func TestEscClosesTheOverlayAndTheInputBoxWorksWhileItIsOpen(t *testing.T) {
	screen, link := aScreenWithAPill()
	send(screen, aShownRecord("task", "17", theRecordText))
	typeWord(screen, "carry on")
	if screen.input.text() != "carry on" {
		t.Errorf("the input box holds %q with the overlay open, and typing still goes to it", screen.input.text())
	}
	pressKey(screen, tea.KeyEnter)
	if last := link.sent[len(link.sent)-1]; last.Type != contract.SocketMessage || last.Text != "carry on" {
		t.Errorf("Enter with the overlay open sent %+v, and it sends what was typed", last)
	}
	if !screen.overlayOpen() {
		t.Fatal("sending a message closed the overlay")
	}
	pressKey(screen, tea.KeyEsc)
	if screen.overlayOpen() {
		t.Fatal("Esc did not close the overlay")
	}
	if !strings.Contains(plainText(screen.frame()), "r27 tests: all passing") {
		t.Error("the transcript is not back after the overlay closed")
	}
}

func TestEscLetsGoOfTheFocusThenTheOverlayThenThePills(t *testing.T) {
	screen, _ := aScreenWithAPill()
	expandTheFocusedPill(screen)
	send(screen, aShownRecord("task", "17", theRecordText))
	pressTab(screen)
	if screen.focusAt < 0 {
		t.Fatal("Tab with the overlay open focused nothing, and the panel's rows and the pills behind it are still there")
	}
	pressKey(screen, tea.KeyEsc)
	if screen.focusAt >= 0 || !screen.overlayOpen() || len(screen.expanded) != 1 {
		t.Fatal("the first Esc did more or less than clear the focus")
	}
	pressKey(screen, tea.KeyEsc)
	if screen.overlayOpen() || len(screen.expanded) != 1 {
		t.Fatal("the second Esc did more or less than close the overlay")
	}
	pressKey(screen, tea.KeyEsc)
	if len(screen.expanded) != 0 {
		t.Error("the third Esc did not fold the open pill")
	}
}

func TestAStatusThatChangesTheRunningTaskClosesNothing(t *testing.T) {
	screen, _ := aScreenWithAPill()
	expandTheFocusedPill(screen)
	send(screen, aShownRecord("task", "17", theRecordText))
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldTask: "18", contract.StatusFieldTaskAsk: "the next thing",
	}})
	if !screen.overlayOpen() || len(screen.expanded) != 1 {
		t.Error("a status naming a new running task closed the overlay or folded a pill")
	}
}

func TestARecordWithNoTextStillOpensAndSaysSo(t *testing.T) {
	screen, _ := aScreenWithAPill()
	send(screen, aShownRecord("task", "17", ""))
	if !screen.overlayOpen() || !strings.Contains(firstTextLine(screen), "nothing") {
		t.Errorf("an empty record drew %q, and it says the program sent nothing", firstTextLine(screen))
	}
}
