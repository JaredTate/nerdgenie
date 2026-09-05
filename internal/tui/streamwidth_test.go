package tui

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// theReplyFromTheLiveRun is a reply the local model wrote on 2026-09-05, code
// spans, arrows, dashes and check marks included, which the first capture of
// the live screen showed garbled while it streamed.
const theReplyFromTheLiveRun = `All four done lines are met and pinned — tests pass (r8) and the README exists (r9). Nothing left to do.

**What changed:**
- ` + "`" + `test_inventory.py` + "`" + ` — plain-assert tests for adding stock, removing stock, refusing over-remove, and sorted listing; each test points ` + "`" + `DATA_FILE` + "`" + ` at a throwaway temp file so your real data is untouched.
- ` + "`" + `inventory.py` + "`" + ` — ` + "`" + `add` + "`" + `, ` + "`" + `remove` + "`" + `, ` + "`" + `list_items` + "`" + `, backed by ` + "`" + `inventory.json` + "`" + `. Over-remove raises ` + "`" + `ValueError` + "`" + ` and leaves the file unchanged; zero-count items are dropped.
- ` + "`" + `README.md` + "`" + ` — API, storage notes, and how to run the tests.

**Checked:** ran ` + "`" + `python3 test_inventory.py` + "`" + ` → ` + "`" + `all tests passed` + "`" + `, exit 0. Watched it fail first (ModuleNotFoundError before ` + "`" + `inventory.py` + "`" + ` existed), then pass after implementation.

Done when:
- [x] test_inventory.py written with plain asserts for add, remove, refuse-over-remove, sorted list -> r5
- [x] Tests run and observed failing before implementation -> r6
- [x] inventory.py implemented until every assert passes -> r8
- [x] README.md exists -> r9`

// TestEveryRowOfAStreamingFrameIsExactlyTheWidth holds the frame's rows to
// the terminal's width while a reply streams in beside a pill and the panel,
// with the words of a real reply from a live run: a row that is a cell too
// wide wraps in the terminal and shifts every row under it, which reads as
// stray letters from an older frame, and a row a cell short leaves a column
// of the old frame standing.
func TestEveryRowOfAStreamingFrameIsExactlyTheWidth(t *testing.T) {
	text := theReplyFromTheLiveRun
	screen, _ := newTestScreen(120, 40)
	screen.Update(linkMessage{up: true})
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldTask: "3", contract.StatusFieldTaskAsk: "build inventory.py", contract.StatusFieldPlan: "[ ] write the tests\n[ ] make them pass",
		contract.StatusFieldSituation: "files: test_inventory.py, inventory.py\nran: python3 test_inventory.py, exit 0\ntests: all 4 passing",
		contract.StatusFieldToolLine:  "▸ shell python3 test_inventory.py · r9 tests: all 4 passing",
	}})
	pieces := strings.SplitAfter(text, " ")
	for at, piece := range pieces {
		send(screen, contract.SocketEnvelope{Type: contract.SocketDelta, Text: piece})
		if at%7 != 0 {
			continue
		}
		screen.beat(screen.now)
		for number, drawn := range strings.Split(screen.frame(), "\n") {
			if width := displayWidth(plainText(drawn)); width != 120 {
				t.Fatalf("after %d pieces, row %d is %d cells wide and every row must be exactly 120:\n%q", at+1, number+1, width, plainText(drawn))
			}
		}
	}
}
