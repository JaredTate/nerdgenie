package channel

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// aReadWait is how long a test waits for something to arrive on a socket before
// it gives up and says the socket is stuck.
const aReadWait = 5 * time.Second

// theAnswerDeadline is how long the socket in these tests waits for a screen to
// answer, which is the shipped time_per_turn: what cmd/coeus/serve.go passes
// from the user's own configuration.
var theAnswerDeadline = contract.DefaultConfig().Caps.TimePerTurn

// socketHarness is a listening socket with everything around it a test needs.
type socketHarness struct {
	socket  *Socket
	path    string
	queue   *Queue
	stream  *Stream
	clock   *testkit.FakeClock
	secrets *testkit.FakeSecrets
	served  chan error
}

// newSocketHarness starts a socket on a short path under a folder the test
// framework removes afterwards, because a Unix socket path is far shorter than a
// file path may be.
func newSocketHarness(t *testing.T) *socketHarness {
	t.Helper()
	folder, err := os.MkdirTemp("", "coeus-socket")
	if err != nil {
		t.Fatalf("cannot make a folder for the socket: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(folder) })

	harness := &socketHarness{
		path:    filepath.Join(folder, "coeus.sock"),
		queue:   newTestQueue(t, 10),
		stream:  NewStream(StreamOptions{}),
		clock:   testkit.NewFakeClock(arrived),
		secrets: testkit.NewFakeSecrets(),
		served:  make(chan error, 1),
	}
	socket, err := Listen(Options{
		Path:           harness.path,
		Stream:         harness.stream,
		Queue:          harness.queue,
		Secrets:        harness.secrets,
		Clock:          harness.clock,
		AnswerDeadline: theAnswerDeadline,
	})
	if err != nil {
		t.Fatalf("listening on the socket failed: %v", err)
	}
	harness.socket = socket

	ctx, stop := context.WithCancel(context.Background())
	go func() { harness.served <- socket.Serve(ctx) }()
	t.Cleanup(func() {
		stop()
		if err := socket.Close(); err != nil {
			t.Errorf("closing the socket failed: %v", err)
		}
		harness.stream.Close()
		if err := <-harness.served; err != nil {
			t.Errorf("serving the socket ended with %v", err)
		}
	})
	return harness
}

// screen is a test's stand-in for the terminal: one client of the socket.
type screen struct {
	t      *testing.T
	socket net.Conn
	lines  *bufio.Reader
}

// dial opens one client connection to the socket and closes it when the test
// ends.
func (harness *socketHarness) dial(t *testing.T) *screen {
	t.Helper()
	connection, err := net.Dial("unix", harness.path)
	if err != nil {
		t.Fatalf("cannot attach to the socket at %s: %v", harness.path, err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return &screen{t: t, socket: connection, lines: bufio.NewReader(connection)}
}

// attach opens a client connection and asks for the event stream.
func (harness *socketHarness) attach(t *testing.T) *screen {
	t.Helper()
	client := harness.dial(t)
	client.send(contract.SocketEnvelope{Type: contract.SocketAttach})
	harness.waitForAttached(t, 1)
	return client
}

// waitForAttached waits until the socket says the wanted number of screens are
// reading the event stream.
func (harness *socketHarness) waitForAttached(t *testing.T, wanted int) {
	t.Helper()
	deadline := time.Now().Add(aReadWait)
	for time.Now().Before(deadline) {
		if harness.socket.Attached() >= wanted {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("%d screens are reading the event stream after %s, want %d", harness.socket.Attached(), aReadWait, wanted)
}

// send writes one message to the socket the way a screen does.
func (client *screen) send(envelope contract.SocketEnvelope) {
	client.t.Helper()
	if err := contract.EncodeSocketEnvelope(client.socket, envelope); err != nil {
		client.t.Fatalf("cannot send a %s message: %v", envelope.Type, err)
	}
}

// sendLine writes raw bytes and a newline, which is how a test sends something
// no screen should ever send.
func (client *screen) sendLine(line string) {
	client.t.Helper()
	if _, err := client.socket.Write([]byte(line + "\n")); err != nil {
		client.t.Fatalf("cannot send a line of %d bytes: %v", len(line), err)
	}
}

// next reads the next message the program sent, and fails the test when nothing
// arrives.
func (client *screen) next() contract.SocketEnvelope {
	client.t.Helper()
	envelope, err := client.tryNext()
	if err != nil {
		client.t.Fatalf("reading what the program sent failed: %v", err)
	}
	return envelope
}

// tryNext reads the next message the program sent and gives back the trouble
// rather than failing the test, which is what a helper running on a goroutine of
// its own has to do.
func (client *screen) tryNext() (contract.SocketEnvelope, error) {
	if err := client.socket.SetReadDeadline(time.Now().Add(aReadWait)); err != nil {
		return contract.SocketEnvelope{}, fmt.Errorf("cannot put a deadline on the socket: %w", err)
	}
	line, err := client.lines.ReadBytes('\n')
	if err != nil {
		return contract.SocketEnvelope{}, fmt.Errorf("nothing arrived on the socket: %w", err)
	}
	envelope, err := contract.DecodeSocketEnvelope(line)
	if err != nil {
		return contract.SocketEnvelope{}, fmt.Errorf("the program sent a line that is not a message: %w", err)
	}
	return envelope, nil
}

// waitForClose says whether the program hung up, which is what a bad line earns.
func (client *screen) waitForClose() bool {
	client.t.Helper()
	if err := client.socket.SetReadDeadline(time.Now().Add(aReadWait)); err != nil {
		client.t.Fatalf("cannot put a deadline on the socket: %v", err)
	}
	_, err := client.lines.ReadBytes('\n')
	return err != nil
}

// waitForQueued waits for one message to reach the queue and hands it back.
func (harness *socketHarness) waitForQueued(t *testing.T) Queued {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(aReadWait)
	for time.Now().Before(deadline) {
		taken, held, err := harness.queue.Take(ctx)
		if err != nil {
			t.Fatalf("reading the queue failed: %v", err)
		}
		if held {
			return taken
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("nothing reached the queue in %s", aReadWait)
	return Queued{}
}

// waitForClients waits until the socket is holding the wanted number of
// connections.
func (harness *socketHarness) waitForClients(t *testing.T, wanted int) {
	t.Helper()
	deadline := time.Now().Add(aReadWait)
	for time.Now().Before(deadline) {
		if harness.socket.Clients() >= wanted {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("the socket holds %d clients after %s, want %d", harness.socket.Clients(), aReadWait, wanted)
}
