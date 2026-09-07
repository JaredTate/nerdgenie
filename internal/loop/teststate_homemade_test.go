package loop

import (
	"os"
	"strings"
	"testing"
)

// TestAHomeMadeRunnerWithBallotMarksAndAFullStopReadsItsFailures is run
// eighteen's r45, word for word: the model's own runner printed a ballot
// cross for each failure and "25 passed, 9 failed." with a full stop, exited
// 1, and the harness said "all passing", so the model wrote that the harness
// was wrong every time and stopped trusting it.
func TestAHomeMadeRunnerWithBallotMarksAndAFullStopReadsItsFailures(t *testing.T) {
	text, err := os.ReadFile("testdata/home-made-runner-ballot-marks.txt")
	if err != nil {
		t.Fatal(err)
	}
	state, found := testStateIn(string(text))
	if !found {
		t.Fatal("the runner's output was not read as a test run")
	}
	if state.failed != 9 || state.total != 34 {
		t.Errorf("the reader counted %d failing of %d, want 9 of 34", state.failed, state.total)
	}
	if len(state.failing) == 0 || state.failing[0] != "refuses a move once the game is over" {
		t.Errorf("the failing names read %v, want the first to be the move refused after the game is over", state.failing)
	}
	line := state.line()
	if !strings.HasPrefix(line, "tests: 9 failing of 34") {
		t.Errorf("the line reads %q, want it to open with the counts", line)
	}
	if !strings.Contains(line, `read from "25 passed, 9 failed."`) {
		t.Errorf("the line %q does not carry the summary line the reader used, so a wrong reading stays invisible", line)
	}
}

// TestATestCommandThatExitsNonZeroIsNeverAllPassing holds the one rule that
// cannot be wrong: a test command that exited with anything but zero did not
// pass, whatever the reader made of its marks.
func TestATestCommandThatExitsNonZeroIsNeverAllPassing(t *testing.T) {
	text := "finished with exit code 1\n  ✓ one\n  ✓ two\n  ✗ three\n"
	state, found := testStateIn(text)
	if !found {
		t.Fatal("marked tests were not read as a run")
	}
	if state.failed == 0 {
		t.Errorf("a run that exited 1 reads as all passing: %q", state.line())
	}
	if !strings.Contains(state.line(), "exit 1") {
		t.Errorf("the line %q does not say the command exited 1", state.line())
	}
	green := "finished with exit code 0\n  ✓ one\n  ✓ two\n"
	if state, _ := testStateIn(green); state.failed != 0 {
		t.Errorf("a run that exited 0 with only ticks reads as failing: %q", state.line())
	}
}

// TestFailureMarksOfEveryShapeAreCounted holds that the marks runners draw for
// a failure are all failures: ✖, ✕, ✗, ✘ and ×.
func TestFailureMarksOfEveryShapeAreCounted(t *testing.T) {
	for _, mark := range []string{"✖", "✕", "✗", "✘", "×"} {
		state, found := testStateIn("  ✓ one\n  " + mark + " two\n")
		if !found || state.failed != 1 || len(state.failing) != 1 || state.failing[0] != "two" {
			t.Errorf("the mark %s reads as %d failing %v", mark, state.failed, state.failing)
		}
	}
}
