package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestEveryCallSendsALineWhenItStartsAndWhenItAnswers pins the seam of the gate
// review's seventh finding, which main already fixed: a person watching a task
// used to see a spinner and nothing else from the first call to the last,
// because nothing told a screen which tool was running. The loop sends one line
// when a call starts and one more when its result comes back.
func TestEveryCallSendsALineWhenItStartsAndWhenItAnswers(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("Nothing is read yet. I will read the notes.",
			callFor("c1", "read", `{"path":"notes.md"}`),
			taskCall("c1t", `{"why":"the user wants the notes","doneWhen":["the notes are read"]}`)),
		answerStep("The notes are read. Shall I go on?"),
	}, scriptedTool("read", "the notes say the launch is in March"))

	built.ask(t, "read the notes")

	lines := built.sentToolLines()
	if len(lines) != 4 {
		t.Fatalf("the loop sent %d tool lines for two calls, want four: one as each starts and one as each answers: %v",
			len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], loop.ToolLineMark+" read notes.md") {
		t.Errorf("the first line reads %q, and a line names the tool and the one argument that says what it will do", lines[0])
	}
	if !strings.Contains(lines[1], "the notes say the launch is in March") {
		t.Errorf("the second line reads %q, and the line for a result says what came back", lines[1])
	}
	if !strings.Contains(lines[3], "updated the record") {
		t.Errorf("the fourth line reads %q, and the line for a record write says what was written", lines[3])
	}
}
