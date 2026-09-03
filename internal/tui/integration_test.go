//go:build integration

package tui

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/JaredTate/coeus/internal/contract"
)

// pump takes the next thing the link has to say and gives it to the screen,
// which is what Bubble Tea does for itself when the screen is on a terminal.
func pump(t *testing.T, screen *Screen) tea.Msg {
	t.Helper()
	select {
	case message := <-screen.events:
		screen.Update(message)
		return message
	case <-time.After(waitingLimit):
		t.Fatal("the link said nothing, and the program on the other end had spoken")
		return nil
	}
}

// readOneLine reads one message on the program's side of the socket.
func readOneLine(t *testing.T, reader *bufio.Reader) contract.SocketEnvelope {
	t.Helper()
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("nothing arrived on the program's side of the socket: %v", err)
	}
	envelope, err := contract.DecodeSocketEnvelope(line)
	if err != nil {
		t.Fatalf("the program's side could not read %q: %v", line, err)
	}
	return envelope
}

func TestTheScreenTalksToAProgramOverTheRealSocket(t *testing.T) {
	home, accepted, _ := listenOnATempSocket(t)
	clock := testkitClock()
	screen := New(Options{
		Clock:       clock,
		Environment: plainEnvironment,
		Dialer:      NewUnixDialer(home),
		Width:       80,
		Height:      24,
	})
	defer screen.Close()

	if strings.Contains(screen.frame(), "Ask me first") {
		t.Fatal("the first frame already holds a card, and it is drawn before anything is connected")
	}
	screen.Init()

	link := acceptOne(t, accepted)
	fromScreen := bufio.NewReader(link)
	if first := readOneLine(t, fromScreen); first.Type != contract.SocketAttach {
		t.Fatalf("the screen said %+v first, and it attaches first", first)
	}
	pump(t, screen)
	if !strings.Contains(screen.frame(), "idle") {
		t.Errorf("the status strip is %q after attaching, and the screen is idle", statusStrip(screen))
	}

	for _, piece := range []string{"Nine years ", "ago today."} {
		if err := contract.EncodeSocketEnvelope(link, contract.SocketEnvelope{Type: contract.SocketDelta, Text: piece}); err != nil {
			t.Fatalf("the program could not stream a delta: %v", err)
		}
		pump(t, screen)
	}
	advance(screen, clock, heartbeatInterval)
	if !strings.Contains(screen.frame(), "Nine years ago today.") {
		t.Errorf("the streamed reply is not on the frame:\n%s", screen.frame())
	}

	preview := contract.SocketEnvelope{Type: contract.SocketPreview, ID: "3", Text: "browser_click e7 \"Post\""}
	if err := contract.EncodeSocketEnvelope(link, preview); err != nil {
		t.Fatalf("the program could not send a preview: %v", err)
	}
	pump(t, screen)
	if !strings.Contains(screen.frame(), previewTitle) {
		t.Fatalf("the preview card is not on the frame:\n%s", screen.frame())
	}

	press(screen, 'a')
	answer := readOneLine(t, fromScreen)
	if answer.Type != contract.SocketApprove || answer.ID != "3" || answer.Text != "" {
		t.Errorf("the program was told %+v, and pressing a approves preview 3 for this one call", answer)
	}
}

func TestTheScreenSaysSoWhenTheProgramGoesAwayAndAttachesAgain(t *testing.T) {
	home, accepted, listener := listenOnATempSocket(t)
	clock := testkitClock()
	screen := New(Options{
		Clock:       clock,
		Environment: plainEnvironment,
		Dialer:      NewUnixDialer(home),
		Width:       80,
		Height:      24,
	})
	defer screen.Close()
	screen.Init()

	link := acceptOne(t, accepted)
	pump(t, screen)
	if err := link.Close(); err != nil {
		t.Fatalf("the program could not close its side: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("the program could not stop listening: %v", err)
	}
	pump(t, screen)

	if !strings.Contains(statusStrip(screen), "disconnected, reconnecting") {
		t.Errorf("the status strip is %q after the program went away", statusStrip(screen))
	}
	if !strings.Contains(headerOf(screen), "offline") {
		t.Errorf("the header is %q after the program went away", headerOf(screen))
	}

	waitForSleeper(t, clock)
	secondListener := listenAgain(t, home)
	clock.Advance(firstReconnectWait)
	if _, err := secondListener.Accept(); err != nil {
		t.Fatalf("the screen did not come back to the socket: %v", err)
	}
	pump(t, screen)
	if strings.Contains(statusStrip(screen), "disconnected") {
		t.Errorf("the status strip is %q after the program came back", statusStrip(screen))
	}
}

// listenAgain stands the program's side of the socket back up, the way a
// restarted program would.
func listenAgain(t *testing.T, home contract.Home) net.Listener {
	t.Helper()
	listener, err := net.Listen("unix", home.SocketFile())
	if err != nil {
		t.Fatalf("cannot listen on the socket again: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}
