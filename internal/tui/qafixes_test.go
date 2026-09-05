package tui

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// TestTheCostWordsAreHumanSized holds what the live program sends against
// what a person can read: the program writes the session's tokens as bare
// counts and its cost as a bare number with four decimals, and the header drew
// them as "5950245 in 121317 out · 7.2818". A count is drawn the way the
// context meter draws one, and money with a dollar sign and two decimals; a
// value already written for people is left as it is.
func TestTheCostWordsAreHumanSized(t *testing.T) {
	screen, _ := newTestScreen(120, 40)
	screen.readStatus(map[string]string{"tokensIn": "5950245", "tokensOut": "121317", "cost": "7.2818"})
	if words := screen.costWords(); words != "6M in 121k out · $7.28" {
		t.Errorf("the cost words read %q, want %q", words, "6M in 121k out · $7.28")
	}
	screen.readStatus(map[string]string{"tokensIn": "6.1k", "tokensOut": "0.4k", "cost": "$0.04"})
	if words := screen.costWords(); words != "6.1k in 0.4k out · $0.04" {
		t.Errorf("the cost words read %q, and a value already written for people is left alone", words)
	}
	screen.readStatus(map[string]string{"tokensIn": "23043", "tokensOut": "922", "cost": ""})
	if words := screen.costWords(); words != "23k in 922 out" {
		t.Errorf("the cost words read %q, want %q", words, "23k in 922 out")
	}
}

// TestAPillDoesNotSayTheToolsNameTwice holds the pill's summary against the
// program's own line, which begins the summary with the tool's name and a
// colon: "shell npm test · r27 shell: finished with exit code 1" drew the word
// shell twice on one row.
func TestAPillDoesNotSayTheToolsNameTwice(t *testing.T) {
	read := readPill("▸ shell npm test · r27 shell: finished with exit code 1")
	if read.summary != "finished with exit code 1" {
		t.Errorf("the summary reads %q, want the tool's name and colon taken off the front", read.summary)
	}
	if read.tool != "shell" || read.id != "r27" {
		t.Errorf("the pill read as tool %q and id %q", read.tool, read.id)
	}
	if read := readPill("▸ read notes.md · r28 12 lines"); read.summary != "12 lines" {
		t.Errorf("a summary that does not begin with the tool's name reads %q, and it is left alone", read.summary)
	}
}

// TestTheStateFactsFitThePanelByShortLabelsAndWrapping holds the state block
// against a twenty-eight column panel, where "files changed in this task:"
// alone ate the row and the files were never seen: the long labels the record
// writes become short ones, and a fact runs onto a second row rather than
// ending in an ellipsis.
func TestTheStateFactsFitThePanelByShortLabelsAndWrapping(t *testing.T) {
	screen, _ := newTestScreen(120, 40)
	screen.readStatus(map[string]string{
		"task":      "17",
		"situation": "files changed in this task: shapes.py, test_shapes.py\nlast command: python3 test_shapes.py, exit 1\ntests: 2 failing of 8: area of a circle; perimeter of a square",
	})
	drawn := []string{}
	for _, line := range screen.statePanelLines() {
		drawn = append(drawn, plainText(line.render(screen.colors)))
	}
	joined := strings.Join(drawn, "\n")
	for _, wanted := range []string{"files: shapes.py,", "test_shapes.py", "ran: python3", "tests: 2 failing of 8:"} {
		if !strings.Contains(joined, wanted) {
			t.Errorf("the state block reads:\n%s\nwant it to say %q", joined, wanted)
		}
	}
	if strings.Contains(joined, "files changed in this ta") || strings.Contains(drawn[0], "…") {
		t.Errorf("the state block reads:\n%s\nand a long label or an ellipsis on the first row hides the fact", joined)
	}
}

// TestAPillRemembersItsTaskSoItOpensAfterTheTaskEnds holds that a pill
// carries the task it was drawn under: the live screen asked the program for
// r8 with no task named once task 1 had ended and the status had cleared the
// task, and the program refused. The show names the pill's own task.
func TestAPillRemembersItsTaskSoItOpensAfterTheTaskEnds(t *testing.T) {
	screen, link := aScreenWithAPill()
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{contract.StatusFieldTask: ""}})
	expandTheFocusedPill(screen)
	shows := showsSent(link)
	if len(shows) != 1 || shows[0].Fields["task"] != "17" || shows[0].Fields["id"] != "r27" {
		t.Errorf("after the task ended the show sent was %+v, want id r27 with task 17, the task the pill was drawn under", shows)
	}
}
