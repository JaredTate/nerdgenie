package channel

import (
	"bufio"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// pipeToAScreen makes a client of the socket over a pair of connected ends the
// test holds both of, which is how a test can make a screen that reads nothing
// at all or one whose connection has already gone.
func pipeToAScreen(t *testing.T, harness *socketHarness) (*client, net.Conn) {
	t.Helper()
	ours, theirs := net.Pipe()
	t.Cleanup(func() {
		_ = ours.Close()
		_ = theirs.Close()
	})
	attached := newClient(harness.socket, ours)
	harness.socket.guard.Lock()
	harness.socket.clients[attached] = struct{}{}
	harness.socket.guard.Unlock()
	return attached, theirs
}

func TestAScreenThatFallsTooFarBehindIsToldAndLetGo(t *testing.T) {
	harness := newSocketHarness(t)
	attached, theirs := pipeToAScreen(t, harness)

	subscription := mustSubscribe(t, harness.stream)
	for number := range SubscriberBacklog + 1 {
		if err := harness.stream.Publish(aDelta("event")); err != nil {
			t.Fatalf("publishing event %d failed: %v", number, err)
		}
	}
	if !subscription.Dropped() {
		t.Fatal("the stream kept a reader that never read a thing, and the backlog cap is what stops that")
	}

	go attached.forward(subscription)

	if err := theirs.SetReadDeadline(time.Now().Add(aReadWait)); err != nil {
		t.Fatalf("cannot put a deadline on the pipe: %v", err)
	}
	lines := bufio.NewReader(theirs)
	last := contract.SocketEnvelope{}
	for range SubscriberBacklog + 1 {
		line, err := lines.ReadBytes('\n')
		if err != nil {
			t.Fatalf("reading what the socket sent failed: %v", err)
		}
		if last, err = contract.DecodeSocketEnvelope(line); err != nil {
			t.Fatalf("the socket sent a line that is not a message: %v", err)
		}
	}

	if last.Type != contract.SocketError {
		t.Errorf("the last thing the screen was sent is a %s, want an error saying it fell behind", last.Type)
	}
	if !strings.Contains(last.Text, "behind") {
		t.Errorf("the screen was told %q, and it has to be told it fell behind", last.Text)
	}
	if _, err := lines.ReadBytes('\n'); err == nil {
		t.Error("the screen that fell behind is still connected")
	}
}

func TestAScreenThatCannotBeWrittenToIsHungUpOn(t *testing.T) {
	harness := newSocketHarness(t)
	attached, theirs := pipeToAScreen(t, harness)
	if err := attached.attach(); err != nil {
		t.Fatalf("putting the screen on the event stream failed: %v", err)
	}
	if err := theirs.Close(); err != nil {
		t.Fatalf("closing the screen's end failed: %v", err)
	}

	if err := harness.socket.Send(context.Background(), "nobody is listening any more"); err != nil {
		t.Fatalf("sending to a screen that has gone came back as an error: %v", err)
	}
	if attached.reading() {
		t.Error("a screen that could not be written to is still on the event stream")
	}
	if held := harness.socket.Clients(); held != 0 {
		t.Errorf("the socket still holds %d clients, want none", held)
	}
}

func TestAScreenIsTurnedAwayWhenTheEventStreamIsFull(t *testing.T) {
	harness := newSocketHarness(t)
	for number := range MaxSubscribers {
		if _, err := harness.stream.Subscribe(); err != nil {
			t.Fatalf("filling the event stream failed at reader %d: %v", number, err)
		}
	}

	client := harness.dial(t)
	client.send(contract.SocketEnvelope{Type: contract.SocketAttach})
	got := client.next()
	if got.Type != contract.SocketError {
		t.Fatalf("a screen attaching to a full stream was answered with a %s, want an error", got.Type)
	}

	// The screen is still connected, so it can try again once someone leaves.
	client.send(contract.SocketEnvelope{Type: contract.SocketMessage, Text: "still here"})
	if taken := harness.waitForQueued(t); taken.Message.Text != "still here" {
		t.Errorf("the queue holds %q, want %q", taken.Message.Text, "still here")
	}
}

func TestAClosedSocketHasNothingToReceiveFrom(t *testing.T) {
	harness := newSocketHarness(t)
	if err := harness.socket.Close(); err != nil {
		t.Fatalf("closing the socket failed: %v", err)
	}
	if _, err := harness.socket.Receive(context.Background()); err == nil {
		t.Error("a closed socket handed out a stream of messages")
	}
}

func TestListeningRefusesAPathItCannotUse(t *testing.T) {
	folder := t.TempDir()
	inTheWay := filepath.Join(folder, "not-a-folder")
	if err := os.WriteFile(inTheWay, []byte("a file, not a folder"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot make the file in the way: %v", err)
	}

	for what, path := range map[string]string{
		"a folder that is not there": filepath.Join(folder, "no-such-folder", "agent.sock"),
		"a path through a file":      filepath.Join(inTheWay, "agent.sock"),
	} {
		if _, err := Listen(Options{
			Path:    path,
			Stream:  NewStream(StreamOptions{}),
			Queue:   newTestQueue(t, 10),
			Secrets: testkit.NewFakeSecrets(),
			Clock:   testkit.NewFakeClock(arrived),
		}); err == nil {
			t.Errorf("a socket was opened on %s", what)
		}
	}
}

func TestListeningRefusesASocketWithAnAnswerDeadlineBelowZero(t *testing.T) {
	// A deadline of the package's own choosing would quietly ignore the
	// time_per_turn the user set, so the caller passes theirs: zero, the
	// shipped default, is no limit, and only a length below zero is refused.
	_, err := Listen(Options{
		Path:           filepath.Join(t.TempDir(), "agent.sock"),
		Stream:         NewStream(StreamOptions{}),
		Queue:          newTestQueue(t, 10),
		Secrets:        testkit.NewFakeSecrets(),
		Clock:          testkit.NewFakeClock(arrived),
		AnswerDeadline: -time.Minute,
	})
	if err == nil {
		t.Fatal("a socket was opened with a deadline below zero for answering a preview")
	}
	if !strings.Contains(err.Error(), "time_per_turn") {
		t.Errorf("the refusal reads %q, and it has to name the setting the deadline comes from", err)
	}
}

func TestListeningTakesASocketWithNoAnswerDeadline(t *testing.T) {
	socket, err := Listen(Options{
		Path:    filepath.Join(t.TempDir(), "agent.sock"),
		Stream:  NewStream(StreamOptions{}),
		Queue:   newTestQueue(t, 10),
		Secrets: testkit.NewFakeSecrets(),
		Clock:   testkit.NewFakeClock(arrived),
	})
	if err != nil {
		t.Fatalf("a socket with no answer deadline, which is the shipped time_per_turn, was refused: %v", err)
	}
	if err := socket.Close(); err != nil {
		t.Errorf("closing the socket failed: %v", err)
	}
}

func TestReceiveRefusesMoreWatchersThanItsCap(t *testing.T) {
	harness := newSocketHarness(t)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	for number := range MaxWatchers {
		if _, err := harness.socket.Receive(ctx); err != nil {
			t.Fatalf("watcher %d was refused below the cap: %v", number, err)
		}
	}
	if _, err := harness.socket.Receive(ctx); err == nil {
		t.Errorf("watcher %d was taken, and the cap is %d", MaxWatchers+1, MaxWatchers)
	}
}

func TestWhatAnErrorRepeatsBackIsCutOnWholeLetters(t *testing.T) {
	// Half a letter is not a letter: cutting a line by bytes lands in the middle
	// of anything but plain English and reaches the screen as a question mark in
	// a box.
	cut := shortenedText(strings.Repeat("é", maxTextInAMessage+10))
	if !utf8.ValidString(cut) {
		t.Fatalf("the cut line %q is not readable text, so a letter was cut in half", cut)
	}
	if letters := utf8.RuneCountInString(strings.TrimSuffix(cut, "...")); letters != maxTextInAMessage {
		t.Errorf("the cut line holds %d letters, want %d", letters, maxTextInAMessage)
	}
	if short := strings.Repeat("é", maxTextInAMessage); shortenedText(short) != short {
		t.Errorf("a line of %d letters was cut to %q, and only a longer one is cut", maxTextInAMessage, shortenedText(short))
	}
}

func TestAMessageTypeTheReaderShouldHaveRefusedIsIgnored(t *testing.T) {
	harness := newSocketHarness(t)
	attached, _ := pipeToAScreen(t, harness)

	// The line reader refuses every type but the seven a screen sends, so this
	// can only happen if the two ever drift apart. It must do nothing rather
	// than something surprising.
	if err := harness.socket.handle(attached, contract.SocketEnvelope{Type: contract.SocketDelta}); err != nil {
		t.Errorf("a message type no screen sends came back as %v, want nothing done", err)
	}
}
