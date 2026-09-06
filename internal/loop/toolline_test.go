package loop_test

import (
	"errors"
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

// TestARefusedCallWithALongArgumentStillSaysRefusedOnItsLine is the fifth
// game build's screen: a click with a whole sentence for its intent was
// refused, the line was cut to its ninety runes before the word reached the
// end, and the screen drew a green check on a call that failed. The word that
// says a call went wrong is kept whatever the argument's length.
func TestARefusedCallWithALongArgumentStillSaysRefusedOnItsLine(t *testing.T) {
	intent := "Start the Tater Tots Tetris game by clicking START GAME so that the play-test can begin at last"
	built := newHarness(t, []testkit.Step{
		callStep("I will click Start.", callFor("c1", "browser_click", `{"element":"e3","intent":"`+intent+`"}`)),
		answerStep("The click was refused. What changed: nothing. What I checked: the click. What is left: the fix."),
	}, &failingTool{name: "browser_click", reason: errors.New("cannot click the element e3: The page could not be read after 3000 milliseconds: the page did not answer the scan call")})

	built.ask(t, "play-test the game")

	lines := built.sentToolLines()
	if len(lines) != 2 {
		t.Fatalf("the loop sent %d tool lines, want two: %v", len(lines), lines)
	}
	answered := lines[1]
	if !strings.HasSuffix(answered, loop.ToolLineSeparator+"refused") {
		t.Errorf("the line for a refused call reads %q, and it ends with the word refused whatever the argument's length", answered)
	}
	if runes := len([]rune(answered)); runes > loop.MaxToolLineRunes {
		t.Errorf("the line is %d runes, over the %d a line may be", runes, loop.MaxToolLineRunes)
	}
	if !strings.Contains(answered, "cannot click") {
		t.Errorf("the line %q does not carry the start of what came back", answered)
	}
}
