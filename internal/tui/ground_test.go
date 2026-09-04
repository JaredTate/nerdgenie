package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// No screenshot tool works on the development desktop, so the proof that the
// screen paints a real terminal in DigiByte's colours is a real process on a
// real pseudo-terminal that promises twenty-four bit colour. The bytes it
// writes are read back and searched for the ground, the accent, and the green
// of a check mark, and, once the person has quit, for the codes that hand the
// terminal its own colours back.
const (
	// groundHelperSetting names the setting that turns this test binary into
	// the child that draws a job on the pseudo-terminal.
	groundHelperSetting = "NERDGENIE_TUI_GROUND_CHILD"
	// groundHelperTestName is the test in this file the child runs.
	groundHelperTestName = "TestTheScreenHelperDrawsAJobOnATrueColourTerminal"
	// captureSetting names a file the raw bytes are written to when it is set,
	// so that a person can read exactly what the terminal was sent.
	captureSetting = "NERDGENIE_TUI_CAPTURE"
	// proofColumns and proofRows are the size of the terminal the proof runs
	// on, which is the size the brief asks the frame to be shown at.
	proofColumns = 200
	proofRows    = 50
	// proofWords is what the frame must show before its bytes are read: the
	// job's name, which only the checklist draws.
	proofWords = "Tater Tots Tetris"
	// frameMustAppearWithin is how long the child has to draw the checklist.
	frameMustAppearWithin = 5 * time.Second
	// quitMustHappenWithin is how long the child has to end after Ctrl+C twice.
	quitMustHappenWithin = 5 * time.Second
)

// theTaterTotsJob is the status the proof draws: a job of four tasks, one done
// and one running with a three-step plan of which one step is done.
func theTaterTotsJob() contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldModel:         "opus",
		contract.StatusFieldTask:          "24",
		contract.StatusFieldTaskState:     "running",
		contract.StatusFieldTokensIn:      "18.2k",
		contract.StatusFieldTokensOut:     "2.1k",
		contract.StatusFieldCost:          "$0.31",
		contract.StatusFieldBudget:        "72 rounds, 40 min left",
		contract.StatusFieldContextTokens: "21500",
		contract.StatusFieldContextWindow: "262144",
		contract.StatusFieldState:         contract.StateThinking,
		contract.StatusFieldJob:           "3",
		contract.StatusFieldJobName:       proofWords,
		contract.StatusFieldJobTask:       "t2",
		contract.StatusFieldJobTasks: "[x] t1 set up the board and the falling piece\n" +
			"[ ] t2 draw the tater tot pieces and their rotation\n" +
			"[ ] t3 add the score, the levels and the game over\n" +
			"[ ] t4 play-test the whole game and fix what breaks",
		contract.StatusFieldPlan: "[x] read the game plan and the sprite notes\n" +
			"[ ] draw the seven tater tot shapes\n" +
			"[ ] test the rotation on every shape",
	}}
}

// TestTheRealScreenPaintsTheDigiByteGroundAndHandsTheTerminalBackOnQuit runs
// the real screen on a pseudo-terminal that promises twenty-four bit colour,
// waits for the checklist, quits it with Ctrl+C twice the way a person does,
// and reads the bytes back for the palette and for the codes that restore the
// terminal's own colours.
func TestTheRealScreenPaintsTheDigiByteGroundAndHandsTheTerminalBackOnQuit(t *testing.T) {
	terminal := openPseudoTerminalOfSize(t, proofColumns, proofRows)
	child := startTheHelper(t, terminal, groundHelperTestName, []string{
		groundHelperSetting + "=1",
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	})
	defer stopTheChild(child)
	drawn := readWhatIsDrawn(terminal.near)

	giveUpAt := time.Now().Add(frameMustAppearWithin)
	for !strings.Contains(drawn.text(), proofWords) {
		if time.Now().After(giveUpAt) {
			t.Fatalf("the screen never drew %q within %s; what it drew was %q", proofWords, frameMustAppearWithin, drawn.text())
		}
		time.Sleep(lookAgainAfter)
	}

	for range 2 {
		if _, err := terminal.near.WriteString("\x03"); err != nil {
			t.Fatalf("pressing Ctrl+C at the screen failed: %v", err)
		}
		time.Sleep(5 * lookAgainAfter)
	}
	ended := make(chan error, 1)
	go func() { ended <- child.Wait() }()
	select {
	case <-ended:
	case <-time.After(quitMustHappenWithin):
		t.Errorf("the screen did not quit within %s of two Ctrl+C presses", quitMustHappenWithin)
	}
	time.Sleep(5 * lookAgainAfter)

	raw := drawn.text()
	if path := os.Getenv(captureSetting); path != "" {
		if err := os.WriteFile(path, []byte(raw), contract.DataFileMode); err != nil {
			t.Fatalf("writing the raw bytes to %s failed: %v", path, err)
		}
	}
	lower := strings.ToLower(raw)
	for _, one := range []struct {
		bytes  string
		saying string
	}{
		{"\x1b]11;#002352", "the terminal's own background is asked to be DigiByte dark blue for the program's lifetime"},
		{"\x1b]10;#ffffff", "the terminal's own foreground is asked to be white"},
		{"48;2;0;35;82", "the rows are painted on DigiByte dark blue"},
		{"38;2;0;102;204", "DigiByte blue draws the accent"},
		{"38;2;77;163;255", "the accent text is the lighter tint"},
		{"38;2;61;220;132", "the check mark is green"},
		{"\x1b]111", "the terminal gets its own background back on quit"},
		{"\x1b]110", "the terminal gets its own foreground back on quit"},
	} {
		if !strings.Contains(lower, strings.ToLower(one.bytes)) {
			t.Errorf("the terminal was never sent %q, and %s", one.bytes, one.saying)
		}
	}
}

// TestTheScreenHelperDrawsAJobOnATrueColourTerminal is the child half of the
// proof above: it builds the screen at the proof's size with the Tater Tots
// job on it and runs it on whatever terminal it was given. It does nothing
// unless the parent started it.
func TestTheScreenHelperDrawsAJobOnATrueColourTerminal(t *testing.T) {
	if os.Getenv(groundHelperSetting) == "" {
		t.Skip("this is the child half of the pseudo-terminal proof, and it draws a screen only when the parent starts it")
	}
	screen := New(Options{Width: proofColumns, Height: proofRows})
	screen.link = &recordingLink{}
	screen.Update(linkMessage{up: true})
	send(screen, theTaterTotsJob())
	send(screen, contract.SocketEnvelope{Type: contract.SocketReply, Text: "Where I stand: the board is up, drawing the pieces next."})
	program := tea.NewProgram(screen,
		tea.WithWindowSize(proofColumns, proofRows),
		tea.WithOutput(os.Stdout),
		tea.WithInput(os.Stdin),
	)
	if _, err := program.Run(); err != nil {
		t.Fatalf("the screen the parent started stopped on its own: %v", err)
	}
}
