package channel

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestAClientAttachesSendsAMessageAndStreamsAReplyDeltaByDelta(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)

	client.send(contract.SocketEnvelope{Type: contract.SocketMessage, ID: "m9", Text: "book me a flight"})

	taken := harness.waitForQueued(t)
	if taken.Message.Text != "book me a flight" {
		t.Errorf("the queue holds %q, want %q", taken.Message.Text, "book me a flight")
	}
	if taken.Message.Channel != contract.TerminalChannelName {
		t.Errorf("the message came from the channel %q, want %q", taken.Message.Channel, contract.TerminalChannelName)
	}
	if !taken.Message.Received.Equal(harness.clock.Now()) {
		t.Errorf("the message was received at %s, want the clock's %s", taken.Message.Received, harness.clock.Now())
	}
	if taken.Message.ID != "m9" {
		t.Errorf("the message came in as %q, want %q", taken.Message.ID, "m9")
	}

	for _, piece := range []string{"Booking ", "the ", "flight."} {
		if err := harness.stream.Publish(aDelta(piece)); err != nil {
			t.Fatalf("publishing the delta %q failed: %v", piece, err)
		}
	}
	if err := harness.stream.Publish(contract.SocketEnvelope{
		Type: contract.SocketReply, Text: "Booking the flight.",
	}); err != nil {
		t.Fatalf("publishing the reply failed: %v", err)
	}

	for _, want := range []string{"Booking ", "the ", "flight."} {
		got := client.next()
		if got.Type != contract.SocketDelta || got.Text != want {
			t.Errorf("the screen saw a %s saying %q, want a delta saying %q", got.Type, got.Text, want)
		}
	}
	if got := client.next(); got.Type != contract.SocketReply || got.Text != "Booking the flight." {
		t.Errorf("the screen saw a %s saying %q, want the finished reply", got.Type, got.Text)
	}
}

func TestTwoClientsSeeTheSameStream(t *testing.T) {
	harness := newSocketHarness(t)
	first := harness.attach(t)
	second := harness.dial(t)
	second.send(contract.SocketEnvelope{Type: contract.SocketAttach})
	harness.waitForAttached(t, 2)

	for _, text := range []string{"one", "two"} {
		if err := harness.stream.Publish(aDelta(text)); err != nil {
			t.Fatalf("publishing %q failed: %v", text, err)
		}
	}

	for _, client := range []*screen{first, second} {
		for _, want := range []string{"one", "two"} {
			if got := client.next(); got.Text != want {
				t.Errorf("a screen saw %q, want %q", got.Text, want)
			}
		}
	}
}

func TestACommandArrivesInTheQueueWithItsSlash(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.dial(t)
	client.send(contract.SocketEnvelope{Type: contract.SocketCommand, Text: "status"})

	taken := harness.waitForQueued(t)
	if taken.Message.Text != CommandPrefix+"status" {
		t.Errorf("the queue holds %q, want %q", taken.Message.Text, CommandPrefix+"status")
	}
}

func TestDetachingStopsTheStreamAndLeavesTheClientConnected(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)

	client.send(contract.SocketEnvelope{Type: contract.SocketDetach})
	harness.waitForAttached(t, 0)
	if err := harness.stream.Publish(aDelta("nobody should see this")); err != nil {
		t.Fatalf("publishing after the detach failed: %v", err)
	}

	// The client is still connected, so a message it sends still reaches the
	// queue even though the stream no longer reaches the client.
	client.send(contract.SocketEnvelope{Type: contract.SocketMessage, Text: "still here"})
	if taken := harness.waitForQueued(t); taken.Message.Text != "still here" {
		t.Errorf("the queue holds %q, want %q", taken.Message.Text, "still here")
	}
}

func TestAttachingTwiceIsNotAnError(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)
	client.send(contract.SocketEnvelope{Type: contract.SocketAttach})

	if err := harness.stream.Publish(aDelta("once only")); err != nil {
		t.Fatalf("publishing failed: %v", err)
	}
	if got := client.next(); got.Text != "once only" {
		t.Errorf("the screen saw %q, want %q", got.Text, "once only")
	}
	if attached := harness.socket.Attached(); attached != 1 {
		t.Errorf("%d screens are reading the stream, want 1", attached)
	}
}

func TestABadLineDisconnectsTheClientWithOneErrorMessage(t *testing.T) {
	harness := newSocketHarness(t)

	for what, line := range map[string]string{
		"a line that is not JSON":                   "{this is not JSON",
		"a line that is JSON but not an object":     `"just a string"`,
		"a message type neither side sends":         `{"type":"wibble"}`,
		"a message type only the program sends":     `{"type":"delta","text":"hello"}`,
		"a line the program sends back to a screen": `{"type":"reply","text":"hello"}`,
	} {
		client := harness.dial(t)
		client.sendLine(line)

		got := client.next()
		if got.Type != contract.SocketError {
			t.Errorf("%s was answered with a %s, want an error", what, got.Type)
		}
		if len(strings.Fields(got.Text)) < 4 {
			t.Errorf("%s was answered with %q, and the message has to say what to do", what, got.Text)
		}
		if !client.waitForClose() {
			t.Errorf("%s did not disconnect the client", what)
		}
	}
}

func TestALinePastTheCapDisconnectsTheClient(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.dial(t)
	client.sendLine(strings.Repeat("x", DefaultMaxLineBytes+1))

	if got := client.next(); got.Type != contract.SocketError {
		t.Errorf("a line past the cap was answered with a %s, want an error", got.Type)
	}
	if !client.waitForClose() {
		t.Error("a line past the cap did not disconnect the client")
	}
}

func TestAMessageWithNothingInItIsRefusedAndTheClientStays(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.dial(t)
	client.send(contract.SocketEnvelope{Type: contract.SocketMessage, Text: "   "})

	if got := client.next(); got.Type != contract.SocketError {
		t.Errorf("an empty message was answered with a %s, want an error", got.Type)
	}
	client.send(contract.SocketEnvelope{Type: contract.SocketMessage, Text: "something real"})
	if taken := harness.waitForQueued(t); taken.Message.Text != "something real" {
		t.Errorf("the queue holds %q, want %q", taken.Message.Text, "something real")
	}
}

func TestAFullQueueIsRefusedWithTheLineTheUserShouldSee(t *testing.T) {
	harness := newSocketHarness(t)
	ctx := context.Background()
	for range 10 {
		if _, err := harness.queue.Add(ctx, anInbound("filling the queue")); err != nil {
			t.Fatalf("filling the queue failed: %v", err)
		}
	}

	client := harness.dial(t)
	client.send(contract.SocketEnvelope{Type: contract.SocketMessage, Text: "one too many"})
	got := client.next()
	if got.Type != contract.SocketError {
		t.Fatalf("a message into a full queue was answered with a %s, want an error", got.Type)
	}
	if !strings.Contains(got.Text, "wait") {
		t.Errorf("the refusal reads %q, and it has to tell the user to wait", got.Text)
	}
}

func TestTheSocketRefusesMoreClientsThanItsCap(t *testing.T) {
	harness := newSocketHarness(t)
	for range DefaultMaxClients {
		harness.dial(t)
	}
	harness.waitForClients(t, DefaultMaxClients)

	turnedAway := harness.dial(t)
	if got := turnedAway.next(); got.Type != contract.SocketError {
		t.Errorf("the client past the cap was answered with a %s, want an error", got.Type)
	}
	if !turnedAway.waitForClose() {
		t.Error("the client past the cap was not disconnected")
	}
}

func TestTheSocketFileIsReadableByNobodyElse(t *testing.T) {
	harness := newSocketHarness(t)
	details, err := os.Stat(harness.path)
	if err != nil {
		t.Fatalf("cannot look at the socket file: %v", err)
	}
	if mode := details.Mode().Perm(); mode != contract.SecretFileMode {
		t.Errorf("the socket file's mode is %o, want %o so that nobody else can talk to the agent", mode, contract.SecretFileMode)
	}
}

func TestListeningRefusesASocketAnotherCopyIsAlreadyOn(t *testing.T) {
	harness := newSocketHarness(t)
	_, err := Listen(Options{
		Path:           harness.path,
		Stream:         NewStream(StreamOptions{}),
		Queue:          harness.queue,
		Secrets:        harness.secrets,
		Clock:          harness.clock,
		AnswerDeadline: theAnswerDeadline,
	})
	if err == nil {
		t.Fatal("a second socket was opened on a path another copy is already listening on")
	}
	if !strings.Contains(err.Error(), harness.path) {
		t.Errorf("the error reads %q, and it has to name the path", err)
	}
}

func TestListeningClearsASocketFileNobodyIsOn(t *testing.T) {
	folder, err := os.MkdirTemp("", "coeus-socket")
	if err != nil {
		t.Fatalf("cannot make a folder for the socket: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(folder) })
	path := filepath.Join(folder, "coeus.sock")
	if err := os.WriteFile(path, []byte("left behind by a crash"), contract.SecretFileMode); err != nil {
		t.Fatalf("cannot leave a stale socket file behind: %v", err)
	}

	socket, err := Listen(Options{
		Path:           path,
		Stream:         NewStream(StreamOptions{}),
		Queue:          newTestQueue(t, 10),
		Secrets:        testkit.NewFakeSecrets(),
		Clock:          testkit.NewFakeClock(arrived),
		AnswerDeadline: theAnswerDeadline,
	})
	if err != nil {
		t.Fatalf("a socket file left behind by a crash stopped the socket opening: %v", err)
	}
	if err := socket.Close(); err != nil {
		t.Fatalf("closing the socket failed: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the socket file is still there after the socket closed: %v", err)
	}
}

func TestListeningRefusesToStartWithoutItsPieces(t *testing.T) {
	whole := Options{
		Path:           filepath.Join(t.TempDir(), "coeus.sock"),
		Stream:         NewStream(StreamOptions{}),
		Queue:          newTestQueue(t, 10),
		Secrets:        testkit.NewFakeSecrets(),
		Clock:          testkit.NewFakeClock(arrived),
		AnswerDeadline: theAnswerDeadline,
	}
	missing := map[string]Options{
		"the path": {
			Stream: whole.Stream, Queue: whole.Queue, Secrets: whole.Secrets,
			Clock: whole.Clock, AnswerDeadline: whole.AnswerDeadline,
		},
		"the event stream": {
			Path: whole.Path, Queue: whole.Queue, Secrets: whole.Secrets,
			Clock: whole.Clock, AnswerDeadline: whole.AnswerDeadline,
		},
		"the queue": {
			Path: whole.Path, Stream: whole.Stream, Secrets: whole.Secrets,
			Clock: whole.Clock, AnswerDeadline: whole.AnswerDeadline,
		},
		"the vault": {
			Path: whole.Path, Stream: whole.Stream, Queue: whole.Queue,
			Clock: whole.Clock, AnswerDeadline: whole.AnswerDeadline,
		},
		"the clock": {
			Path: whole.Path, Stream: whole.Stream, Queue: whole.Queue,
			Secrets: whole.Secrets, AnswerDeadline: whole.AnswerDeadline,
		},
		// No answer deadline at all is the shipped time_per_turn and is taken;
		// one below zero is what nobody can have meant.
		"an answer deadline below zero": {
			Path: whole.Path, Stream: whole.Stream, Queue: whole.Queue,
			Secrets: whole.Secrets, Clock: whole.Clock, AnswerDeadline: -time.Second,
		},
	}
	for what, options := range missing {
		if _, err := Listen(options); err == nil {
			t.Errorf("a socket was opened without %s", what)
		}
	}
}

func TestTheSocketIsAChannelNamedTerminalAndSaysWhetherItIsWorking(t *testing.T) {
	harness := newSocketHarness(t)
	ctx := context.Background()
	if name := harness.socket.Name(); name != contract.TerminalChannelName {
		t.Errorf("the socket calls itself %q, want %q", name, contract.TerminalChannelName)
	}
	if health := harness.socket.Health(ctx); !health.Healthy {
		t.Errorf("a listening socket says it is not working: %s", health.Detail)
	}

	client := harness.attach(t)
	if err := harness.socket.Send(ctx, "a reply straight from the channel"); err != nil {
		t.Fatalf("sending through the socket failed: %v", err)
	}
	if got := client.next(); got.Type != contract.SocketReply || got.Text != "a reply straight from the channel" {
		t.Errorf("the screen saw a %s saying %q, want the reply", got.Type, got.Text)
	}

	if err := harness.socket.SendFile(ctx, "/tmp/shot.png", "the page as it looks now"); err != nil {
		t.Fatalf("sending a file through the socket failed: %v", err)
	}
	got := client.next()
	if got.Type != contract.SocketReply || len(got.Attachments) != 1 || got.Attachments[0] != "/tmp/shot.png" {
		t.Errorf("the screen saw %+v, want the file", got)
	}
	if got.Text != "the page as it looks now" {
		t.Errorf("the file arrived with the caption %q, want %q", got.Text, "the page as it looks now")
	}
}

func TestAClosedSocketSaysSoAndSendsNothing(t *testing.T) {
	harness := newSocketHarness(t)
	ctx := context.Background()
	if err := harness.socket.Close(); err != nil {
		t.Fatalf("closing the socket failed: %v", err)
	}

	health := harness.socket.Health(ctx)
	if health.Healthy {
		t.Error("a closed socket says it is working")
	}
	if health.Detail == "" {
		t.Error("a closed socket does not say why it is not working")
	}
	if err := harness.socket.Send(ctx, "nobody is there"); err == nil {
		t.Error("a closed socket sent a reply")
	}
	if err := harness.socket.SendFile(ctx, "/tmp/shot.png", "nobody is there"); err == nil {
		t.Error("a closed socket sent a file")
	}
}

func TestEverythingSentOutIsRedactedFirst(t *testing.T) {
	harness := newSocketHarness(t)
	harness.secrets.Add("x-account", contract.Credential{
		Site: "x-account", Username: "jared", Password: "hunter2the-real-one",
	})
	client := harness.attach(t)

	if err := harness.socket.Send(context.Background(), "the password is hunter2the-real-one"); err != nil {
		t.Fatalf("sending through the socket failed: %v", err)
	}
	got := client.next()
	if strings.Contains(got.Text, "hunter2the-real-one") {
		t.Errorf("the screen was sent %q, and no secret may leave the program", got.Text)
	}
	if !strings.Contains(got.Text, contract.RedactedMarker) {
		t.Errorf("the screen was sent %q, want the secret replaced by %q", got.Text, contract.RedactedMarker)
	}

	// A secret does not travel only in the body. The line above a preview, the
	// reason beside a refusal, and every value of a status all leave the program
	// too, so each one is put through the redactor and each one is pinned here.
	carried := map[string]func(contract.SocketEnvelope) string{
		"the title": func(sent contract.SocketEnvelope) string { return sent.Title },
		"the reason": func(sent contract.SocketEnvelope) string {
			return sent.Reason
		},
		"the status field": func(sent contract.SocketEnvelope) string {
			return sent.Fields[contract.StatusFieldToolLine]
		},
	}
	if err := harness.stream.Publish(contract.SocketEnvelope{
		Type:   contract.SocketPreview,
		ID:     "9",
		Title:  "sign in with hunter2the-real-one",
		Text:   "the body says nothing",
		Reason: "the last try refused hunter2the-real-one",
		Fields: map[string]string{contract.StatusFieldToolLine: "curl -u jared:hunter2the-real-one"},
	}); err != nil {
		t.Fatalf("publishing the preview failed: %v", err)
	}

	sent := client.next()
	for what, read := range carried {
		if strings.Contains(read(sent), "hunter2the-real-one") {
			t.Errorf("%s reached the screen as %q, and no secret may leave the program", what, read(sent))
		}
		if !strings.Contains(read(sent), contract.RedactedMarker) {
			t.Errorf("%s reached the screen as %q, want the secret replaced by %q", what, read(sent), contract.RedactedMarker)
		}
	}
}

func TestReceiveCarriesEveryMessageTheSocketTookAndClosesWithItsContext(t *testing.T) {
	harness := newSocketHarness(t)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	messages, err := harness.socket.Receive(ctx)
	if err != nil {
		t.Fatalf("attaching to the socket's messages failed: %v", err)
	}
	client := harness.dial(t)
	client.send(contract.SocketEnvelope{Type: contract.SocketMessage, Text: "book me a flight"})

	select {
	case message := <-messages:
		if message.Text != "book me a flight" {
			t.Errorf("the message came through as %q, want %q", message.Text, "book me a flight")
		}
	case <-time.After(aReadWait):
		t.Fatal("nothing came through the socket's own stream of messages")
	}

	stop()
	select {
	case _, open := <-messages:
		if open {
			t.Error("the stream of messages is still open after its context was cancelled")
		}
	case <-time.After(aReadWait):
		t.Fatal("the stream of messages did not close when its context was cancelled")
	}
}
