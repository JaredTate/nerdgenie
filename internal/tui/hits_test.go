package tui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// click gives the screen one press of the left mouse button at a column and a
// row of the frame, both counted from zero at the top left.
func click(screen *Screen, column int, rowOnScreen int) {
	screen.Update(tea.MouseClickMsg{X: column, Y: rowOnScreen, Button: tea.MouseLeft})
}

// rowHolding is the row of the frame that holds some words, or minus one.
func rowHolding(screen *Screen, words string) int {
	for at, drawn := range strings.Split(plainText(screen.frame()), "\n") {
		if strings.Contains(drawn, words) {
			return at
		}
	}
	return -1
}

// TestAHandBuiltRowMapFindsPillsPanelRowsAndNothing holds the hit-testing on
// its own: a map of six rows between the rules, of which the first block owns
// two and the second block two, beside a panel of three lines whose second and
// third name a target.
func TestAHandBuiltRowMapFindsPillsPanelRowsAndNothing(t *testing.T) {
	hits := hitMap{firstRow: 2, owners: []int{-1, 0, 0, -1, 1, 1}, targets: []string{"", "job:4", "task:t3"}, panelFrom: 92}
	for name, one := range map[string]struct {
		column, rowOnScreen int
		wanted              hit
	}{
		"the header":                          {10, 0, nothingHit()},
		"the blank row above the first block": {10, 2, nothingHit()},
		"the first block's first row":         {10, 3, hit{block: 0}},
		"the first block's second row":        {10, 4, hit{block: 0}},
		"the blank between the blocks":        {10, 5, nothingHit()},
		"the second block":                    {10, 7, hit{block: 1}},
		"the column where the panel begins":   {92, 3, hit{block: -1, target: "job:4"}},
		"a panel line with no target":         {95, 2, nothingHit()},
		"the panel's task line":               {95, 4, hit{block: -1, target: "task:t3"}},
		"a panel row below its lines":         {95, 6, nothingHit()},
		"a row below the rows":                {10, 9, nothingHit()},
		"a column left of nothing":            {0, 3, hit{block: 0}},
	} {
		if found := hits.at(one.column, one.rowOnScreen); found != one.wanted {
			t.Errorf("a click on %s at column %d row %d found %+v, want %+v", name, one.column, one.rowOnScreen, found, one.wanted)
		}
	}

	noPanel := hitMap{firstRow: 2, owners: []int{0}, panelFrom: -1}
	if found := noPanel.at(95, 2); found != (hit{block: 0}) {
		t.Errorf("with no panel a click at column 95 found %+v, and the whole width is the transcript's", found)
	}
}

// TestTheRowMapIsBuiltFromTheFrame holds that the map a click is looked up in
// says the same as the frame: the pill's row belongs to the pill's block, the
// panel begins where the transcript ends, and a panel put away is nowhere.
func TestTheRowMapIsBuiltFromTheFrame(t *testing.T) {
	screen, _ := aScreenWithAPill()
	hits := screen.hits()
	pillRow := rowHolding(screen, "r27 tests: all passing")
	if found := hits.at(5, pillRow); found.block != 1 {
		t.Errorf("the pill's row %d maps to %+v, and the pill is block 1", pillRow, found)
	}
	if hits.panelFrom != screen.transcriptColumns() {
		t.Errorf("the panel begins at column %d on the map and %d on the frame", hits.panelFrom, screen.transcriptColumns())
	}
	if len(hits.targets) != len(screen.panelTargets()) {
		t.Errorf("the map has %d panel targets and the panel names %d", len(hits.targets), len(screen.panelTargets()))
	}
	screen.panelHidden = true
	if screen.hits().panelFrom != -1 {
		t.Error("the map still has a panel after the panel was put away")
	}
}

func TestAClickOnAnyRowOfAPillTogglesItAndAClickElsewhereDoesNothing(t *testing.T) {
	screen, link := aScreenWithAPill()
	pillRow := rowHolding(screen, "r27 tests: all passing")

	click(screen, 40, pillRow)
	if len(screen.expanded) != 1 || len(showsSent(link)) != 1 {
		t.Fatal("a click on the pill did not open it and ask for its text")
	}
	fetchingRow := rowHolding(screen, "fetching r27")
	if fetchingRow < 0 {
		t.Fatal("the fetching line is not on the frame after the click")
	}
	click(screen, 6, fetchingRow)
	if len(screen.expanded) != 0 {
		t.Error("a click on the row under the pill did not fold it")
	}

	before := screen.frame()
	click(screen, 3, rowHolding(screen, "ready to post"))
	click(screen, screen.transcriptColumns()+3, 2)
	click(screen, 10, 0)
	screen.Update(tea.MouseClickMsg{X: 40, Y: pillRow, Button: tea.MouseRight})
	if screen.frame() != before || len(showsSent(link)) != 1 {
		t.Error("a click on the reply, on a panel row with no target, on the header, or with the right button changed something")
	}
}

// leftClickBytes is what a terminal sends for the left button pressed and
// let go at a column and a row, both counted from one, once the screen has
// asked it to report the mouse.
func leftClickBytes(column int, rowOnScreen int) string {
	place := strconv.Itoa(column) + ";" + strconv.Itoa(rowOnScreen)
	return "\x1b[<0;" + place + "M" + "\x1b[<0;" + place + "m"
}

// waitUntil looks again and again, at most for the whole-program limit, until
// what is looked for is true, and says whether it ever was.
func waitUntil(condition func() bool) bool {
	giveUpAt := time.Now().Add(wholeProgramLimit)
	for time.Now().Before(giveUpAt) {
		if condition() {
			return true
		}
		time.Sleep(lookAgainAfter)
	}
	return false
}

// showsOn is every show the whole program has sent on a link.
func showsOn(socket *fakeSocket) []contract.SocketEnvelope {
	shows := []contract.SocketEnvelope{}
	for _, envelope := range socket.everySent() {
		if envelope.Type == contract.SocketShow {
			shows = append(shows, envelope)
		}
	}
	return shows
}

// TestAClickOnAPillInTheWholeProgramAsksForItsText drives the whole program
// through Run the way the wheel test does, on the pipes a terminal would be,
// feeds it the bytes a terminal sends for a click on every row of the
// transcript from the top down, and reads one show for the pill off the fake
// socket. The pseudo-terminal harness cannot be used for this, because the
// child it runs has no link to a program and so no pill to click.
func TestAClickOnAPillInTheWholeProgramAsksForItsText(t *testing.T) {
	dialer := newFakeDialer()
	program := startTheWholeProgram(t, dialer)
	socket := dialer.nextLink(t)
	socket.push(contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldTask:     "17",
		contract.StatusFieldToolLine: theTestPillLine,
	}})
	if !waitUntil(func() bool { return strings.Contains(program.painted(), "r27 tests: all passing") }) {
		t.Fatalf("the pill was never painted; the frame so far was:\n%s", program.painted())
	}

	// The transcript is rows three to twenty-one of a twenty-four-row
	// terminal, counted from one, and the pill rests on the last of them;
	// clicking every row from the top down lands on it once and on nothing
	// else, whichever row it settles on.
	for rowOnScreen := 3; rowOnScreen <= 21; rowOnScreen++ {
		program.typeBytes(t, leftClickBytes(5, rowOnScreen))
	}
	if !waitUntil(func() bool { return len(showsOn(socket)) >= 1 }) {
		t.Fatalf("no click asked the program for the pill's text; the frame so far was:\n%s", program.painted())
	}
	shows := showsOn(socket)
	if len(shows) != 1 || shows[0].Fields["id"] != "r27" || shows[0].Fields["task"] != "17" {
		t.Errorf("the clicks sent %+v, and one click on the pill sends one show for r27 in task 17", shows)
	}
	if !waitUntil(func() bool { return strings.Contains(program.painted(), "fetching r27") }) {
		t.Errorf("the fetching line was never painted; the frame so far was:\n%s", program.painted())
	}
	program.quit(t)
}
