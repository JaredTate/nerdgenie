package loop

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// TestAPollOrTailIsNotCountedTowardAWall: a shell poll or tail is meant to be
// read again and again while something runs, so its repeats are waiting, not a
// wall, and the caller leaves them out.
func TestAPollOrTailIsNotCountedTowardAWall(t *testing.T) {
	shell := func(input string) contract.ToolCall {
		return contract.ToolCall{Name: contract.ToolShell, Input: json.RawMessage(input)}
	}
	cases := []struct {
		name string
		call contract.ToolCall
		poll bool
	}{
		{"a shell poll", shell(`{"action":"poll"}`), true},
		{"a shell tail", shell(`{"action":"tail"}`), true},
		{"a shell run", shell(`{"action":"run","command":"npm test"}`), false},
		{"a shell with no action", shell(`{"command":"ls"}`), false},
		{"an edit is never a poll", contract.ToolCall{Name: contract.ToolEdit, Input: json.RawMessage(`{"path":"x"}`)}, false},
	}
	for _, c := range cases {
		if got := isAPollOrTail(c.call); got != c.poll {
			t.Errorf("%s: isAPollOrTail = %v, want %v", c.name, got, c.poll)
		}
	}
}

// TestOneResultOverAndOverIsAWall: when one result comes back
// SameWallRecurrences times inside the window, whatever calls came between it
// and whatever those returned, the work has hit a wall and the detector names
// the call that kept hitting it. This is the case both older nets miss: the
// guard's streak resets on a new result in between, and the progress meter
// resets on a new file written, so a model running the same failing tests
// between edits is invisible to both.
func TestOneResultOverAndOverIsAWall(t *testing.T) {
	var wall sameWall
	// Run the tests (the same failure), then edit a different file each time
	// (a new result every round), around and around.
	for i := 0; i < SameWallRecurrences-1; i++ {
		wall.note("run-tests", "12 failing of 42")
		wall.note("edit-file", fmt.Sprintf("wrote change %d", i))
	}
	if _, hit := wall.hit(); hit {
		t.Fatalf("the wall was called after only %d test runs, want it to hold until %d", SameWallRecurrences-1, SameWallRecurrences)
	}
	wall.note("run-tests", "12 failing of 42")
	mark, hit := wall.hit()
	if !hit {
		t.Fatalf("the wall was not called after the same result came back %d times", SameWallRecurrences)
	}
	if mark != "run-tests" {
		t.Errorf("the wall named %q, want the call that kept hitting it, run-tests", mark)
	}
}

// TestConvergingWorkIsNotAWall: a check whose result changes as the work
// converges — twelve failing, then eight, then three, then none — never trips
// the wall, however many rounds it takes, because each result is new.
func TestConvergingWorkIsNotAWall(t *testing.T) {
	var wall sameWall
	for _, failing := range []string{"12 failing", "11 failing", "9 failing", "6 failing", "3 failing", "1 failing", "0 failing", "all passing"} {
		wall.note("run-tests", failing)
		wall.note("edit-file", "wrote "+failing) // a distinct edit each round
	}
	if _, hit := wall.hit(); hit {
		t.Errorf("converging work was called a wall")
	}
}

// TestTheWindowForgetsOldResults: a result that recurred long ago falls out of
// the window as new results push it past the edge, so a wall the model already
// broke through is not held against it forever.
func TestTheWindowForgetsOldResults(t *testing.T) {
	var wall sameWall
	for i := 0; i < SameWallRecurrences-1; i++ {
		wall.note("run-tests", "12 failing")
	}
	for i := 0; i < SameWallWindow; i++ {
		wall.note("run-tests", fmt.Sprintf("new result %d", i))
	}
	if _, hit := wall.hit(); hit {
		t.Errorf("an old wall still counted after the window moved past it")
	}
}

// TestForgetEmptiesTheWall: a fresh window after a rethink starts the count
// again, so the recurrences before it do not fire the wall a second time at
// once.
func TestForgetEmptiesTheWall(t *testing.T) {
	var wall sameWall
	for i := 0; i < SameWallRecurrences; i++ {
		wall.note("run-tests", "12 failing")
	}
	if _, hit := wall.hit(); !hit {
		t.Fatal("precondition: the wall should have been called")
	}
	wall.forget()
	if _, hit := wall.hit(); hit {
		t.Errorf("the wall was still called after forget()")
	}
}
