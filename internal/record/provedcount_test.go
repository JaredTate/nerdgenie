package record_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// TestAJobHeaderCountsItsProvedCheckedLines: a job whose done list carries
// checks the harness runs says on its header line how many of them are
// proved, and a job without such lines says nothing new.
func TestAJobHeaderCountsItsProvedCheckedLines(t *testing.T) {
	checked := contract.Record{
		Header: contract.Header{Kind: contract.RecordJob, ID: "2", Status: contract.StatusRunning, TasksDone: 1, TasksTotal: 3},
		Goal: contract.Goal{Ask: "build the game", DoneWhen: []contract.DoneLine{
			{Text: "Every test passes. [tests pass: npm test]", Done: true, ResultID: "j2.1"},
			{Text: `The game loads. [shows: "board" at http://x]`},
			{Text: "A whole game has been played."},
		}},
	}
	first := strings.SplitN(string(record.Print(checked)), "\n", 2)[0]
	if !strings.Contains(first, "1 of 3 tasks done") || !strings.HasSuffix(first, "done lines proved: 1 of 2") {
		t.Errorf("the header reads %q, want the task count and then the proved checks, 1 of 2", first)
	}

	plain := checked
	plain.Goal.DoneWhen = []contract.DoneLine{{Text: "the post is up", Done: true, ResultID: "j2.1"}}
	if first := strings.SplitN(string(record.Print(plain)), "\n", 2)[0]; strings.Contains(first, "done lines proved") {
		t.Errorf("a job with no checked line counts proved checks: %q", first)
	}
}
