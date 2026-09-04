package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// theStatusOnAttach is what cmd/nerdgenie/serve.go sends a screen the moment it
// attaches and on every heartbeat after that: the model in use, that nothing is
// running, that the health check answered, and the command list the palette is
// filled from. Each command name carries the leading slash, because that is how
// the running program writes it on the wire.
func theStatusOnAttach() contract.SocketEnvelope {
	listed := []string{
		"/help" + contract.StatusCommandSeparator + "Lists the commands.",
		"/status" + contract.StatusCommandSeparator + "Says what the agent is doing.",
		"/tasks" + contract.StatusCommandSeparator + "Lists the tasks.",
	}
	return contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldModel:    "local",
		contract.StatusFieldState:    contract.StateIdle,
		contract.StatusFieldHealthy:  "true",
		contract.StatusFieldCommands: strings.Join(listed, "\n"),
	}}
}

// attachedScreen is a screen in the state the running program leaves it in a
// moment after it starts: the link is up, the status heartbeat has arrived with
// the command list, and the transcript is still empty so the banner is showing.
func attachedScreen(t *testing.T) (*Screen, *recordingLink) {
	t.Helper()
	screen, link := screenWithLink()
	screen.Update(linkMessage{up: true})
	screen.Update(envelopeMessage{envelope: theStatusOnAttach()})
	if len(screen.blocks) != 0 {
		t.Fatalf("the transcript holds %d blocks after attaching, and the banner shows while it is empty", len(screen.blocks))
	}
	if !strings.Contains(screen.frame(), "the agent that does not forget") {
		t.Fatal("the banner is not on the frame after attaching")
	}
	return screen, link
}

func TestTypingReachesTheInputBoxAfterTheProgramHasReportedItself(t *testing.T) {
	screen, link := attachedScreen(t)

	typeWord(screen, "Say hello in five words.")
	if screen.input.text() != "Say hello in five words." {
		t.Fatalf("the input box holds %q after typing, and it should hold what was typed", screen.input.text())
	}
	if !strings.Contains(screen.frame(), "Say hello in five words.") {
		t.Error("the typed words are not on the frame, and the input box draws what is typed")
	}

	pressKey(screen, tea.KeyEnter)
	if screen.input.text() != "" {
		t.Errorf("Enter left %q in the input box, and sending empties it", screen.input.text())
	}
	if len(link.sent) != 1 || link.sent[0].Type != contract.SocketMessage || link.sent[0].Text != "Say hello in five words." {
		t.Fatalf("Enter sent %+v, and it sends what was typed as a message", link.sent)
	}
	if !strings.Contains(screen.frame(), "Say hello in five words.") {
		t.Error("what was sent is not in the transcript, and the person's own words go there at once")
	}
}

func TestASlashCommandTypedInFullReachesTheProgramTheProgramNamedIt(t *testing.T) {
	screen, link := attachedScreen(t)

	typeWord(screen, "/status")
	if screen.input.text() != "/status" {
		t.Fatalf("the input box holds %q, and the palette must not eat what is typed", screen.input.text())
	}
	// The palette is open on a slash command, so the first Enter completes what
	// is typed and the next one sends it, which is what the palette tests pin.
	pressKey(screen, tea.KeyEnter)
	pressKey(screen, tea.KeyEnter)
	if len(link.sent) != 1 || link.sent[0].Type != contract.SocketCommand || link.sent[0].Text != "status" {
		t.Fatalf("a typed command reached the program as %+v, and it goes as the command the program named", link.sent)
	}
}

func TestThePaletteFiltersTheCommandsTheProgramReported(t *testing.T) {
	screen, _ := attachedScreen(t)

	press(screen, '/')
	if !screen.paletteOpen {
		t.Fatal("a slash in an empty box did not open the command palette")
	}
	listed := strings.Join(screen.paletteRows(), "\n")
	for _, name := range []string{"/help", "/status", "/tasks"} {
		if !strings.Contains(listed, name) {
			t.Errorf("the palette does not list %s, and the program reported it:\n%s", name, listed)
		}
	}
	if strings.Contains(listed, "//") {
		t.Errorf("the palette drew a command with two slashes, because the name the program sent already had one:\n%s", listed)
	}

	typeWord(screen, "sta")
	if screen.input.text() != "/sta" {
		t.Fatalf("the input box holds %q while the palette is open, and it should hold what was typed", screen.input.text())
	}
	filtered := strings.Join(screen.paletteRows(), "\n")
	if !strings.Contains(filtered, "/status") || strings.Contains(filtered, "/help") {
		t.Errorf("the palette filtered to\n%s\nand /sta matches only /status", filtered)
	}

	pressKey(screen, tea.KeyTab)
	if screen.input.text() != "/status " {
		t.Errorf("Tab completed to %q, and it completes the command that matches", screen.input.text())
	}
}

// typedOnATerminal drives the whole screen the way cmd/nerdgenie does: through Run,
// with Bubble Tea reading real bytes off a pipe and drawing to another. It gives
// back everything that was drawn, so that a key that never reaches the input box
// is caught at the layer the person types into rather than only at Update.
func typedOnATerminal(t *testing.T, typed string) string {
	t.Helper()
	keys, keyboard, err := os.Pipe()
	if err != nil {
		t.Fatalf("there is no pipe to type into: %v", err)
	}
	frames, painted, err := os.Pipe()
	if err != nil {
		t.Fatalf("there is no pipe to draw on: %v", err)
	}

	drawn := make(chan string, 1)
	go func() {
		buffer := make([]byte, 64*1024)
		seen := strings.Builder{}
		for {
			read, err := frames.Read(buffer)
			seen.Write(buffer[:read])
			if err != nil {
				drawn <- seen.String()
				return
			}
		}
	}()

	stopped := make(chan error, 1)
	go func() {
		stopped <- Run(Options{
			Environment: plainEnvironment,
			Width:       80,
			Height:      24,
			Input:       keys,
			Output:      painted,
		})
	}()

	if _, err := keyboard.WriteString(typed); err != nil {
		t.Fatalf("the keys could not be typed: %v", err)
	}
	time.Sleep(keyRoundTrip)
	if _, err := keyboard.WriteString("\x03\x03"); err != nil {
		t.Fatalf("the quit could not be typed: %v", err)
	}
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("the screen stopped badly: %v", err)
		}
	case <-time.After(waitingLimit):
		t.Fatal("the screen did not quit on two Ctrl+C presses")
	}
	keyboard.Close()
	keys.Close()
	painted.Close()
	frames.Close()
	return <-drawn
}

// keyRoundTrip is how long the whole program is given to read a key off the
// pipe, change the screen, and draw the frame again.
const keyRoundTrip = 300 * time.Millisecond

func TestTypedLettersReachTheInputBoxThroughTheWholeProgram(t *testing.T) {
	painted := typedOnATerminal(t, "hello there")
	if !strings.Contains(painted, "hello there") {
		t.Errorf("the letters typed into the running screen never reached the input box; the frame held:\n%s", painted)
	}
}
