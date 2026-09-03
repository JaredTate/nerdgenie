package channel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// The limits the socket runs under. Every one of them is a hard stop.
const (
	// DefaultMaxClients is how many screens may be connected at once. A machine
	// with more terminals than this open on one agent has something wrong on it.
	DefaultMaxClients = 8
	// DefaultMaxLineBytes is the longest line the socket will read. A screen
	// sends what a person typed, and nothing a person types comes near it.
	DefaultMaxLineBytes = 64 * 1024
)

// writeWait is how long one write to one screen may take before the socket gives
// up on that screen. It is measured on the operating system's clock rather than
// contract.Clock, because a deadline on a connection is the operating system's
// to keep.
const writeWait = 30 * time.Second

// firstLineBytes is the buffer the line reader starts with. It grows up to the
// cap when a line needs more.
const firstLineBytes = 4096

// Options are what the socket needs to run. The first six are required and the
// last two fall back to the limits above.
type Options struct {
	// Path is where the socket file goes, which is the home folder's SocketFile.
	Path string
	// Stream is the feed of events every attached screen reads.
	Stream *Stream
	// Queue is where the messages and commands screens send are written down.
	Queue *Queue
	// Secrets redacts every piece of text on its way out to a screen.
	Secrets contract.Secrets
	// Clock is where the time a message arrived is read from.
	Clock contract.Clock
	// AnswerDeadline is how long a preview or a masked prompt waits for an
	// answer. It is the user's own time_per_turn, because a question asked
	// inside a turn cannot usefully outlive the turn that asked it, and it is
	// never filled in here, because a deadline of this package's own choosing
	// would quietly ignore what the user set. Zero, which is the shipped
	// default, means the turn has no limit and the question waits as long as
	// the turn does; only a length below zero is refused.
	AnswerDeadline time.Duration
	// MaxClients is how many screens may be connected at once. Zero means
	// DefaultMaxClients.
	MaxClients int
	// MaxLineBytes is the longest line a screen may send. Zero means
	// DefaultMaxLineBytes.
	MaxLineBytes int
}

// Socket is the local socket the terminal and any later screen attach to: a Unix
// socket in the home folder that nobody but the agent's own user account can
// open, speaking one JSON message per line both ways. It is itself a
// contract.Channel, so the terminal is a channel exactly as Signal is.
type Socket struct {
	options  Options
	listener net.Listener
	done     chan struct{}

	guard   sync.Mutex
	clients map[*client]struct{}
	// deltaGuard and streaming hold the reply being streamed to the screens.
	deltaGuard sync.Mutex
	streaming  *replyInProgress
	watchers   map[*watcher]struct{}
	previews   map[string]chan contract.PreviewAnswerWithReason
	prompts    map[string]chan promptAnswer
	asked      int64
	closed     bool
}

// Listen opens the socket file, gives it a mode nobody else can read, and
// returns the channel it is. It clears a socket file an earlier run left behind,
// and refuses a socket another copy of the agent is already listening on.
// Nothing is accepted until Serve runs.
func Listen(options Options) (*Socket, error) {
	if err := options.check(); err != nil {
		return nil, err
	}
	options.fillIn()
	if err := clearStaleSocket(options.Path); err != nil {
		return nil, err
	}

	listener, err := net.Listen("unix", options.Path)
	if err != nil {
		return nil, fmt.Errorf("cannot open the local socket at %s, so check that the folder is there and that nothing else is using the path: %w", options.Path, err)
	}
	if err := os.Chmod(options.Path, contract.SecretFileMode); err != nil {
		return nil, errors.Join(
			fmt.Errorf("cannot give the local socket at %s a mode only this account can read, and it must not be left open to others: %w", options.Path, err),
			listener.Close())
	}

	return &Socket{
		options:  options,
		listener: listener,
		done:     make(chan struct{}),
		clients:  map[*client]struct{}{},
		watchers: map[*watcher]struct{}{},
		previews: map[string]chan contract.PreviewAnswerWithReason{},
		prompts:  map[string]chan promptAnswer{},
	}, nil
}

// Serve takes connections until the context is cancelled or the socket is
// closed, and gives each one its own reader. It returns nothing when it was
// closed on purpose.
func (socket *Socket) Serve(ctx context.Context) error {
	go func() {
		select {
		case <-ctx.Done():
			_ = socket.Close()
		case <-socket.done:
		}
	}()

	for {
		connection, err := socket.listener.Accept()
		if err != nil {
			if socket.isClosed() {
				return nil
			}
			return fmt.Errorf("cannot take a connection on the local socket at %s: %w", socket.options.Path, err)
		}
		socket.take(connection)
	}
}

// Close stops listening, hangs up on every screen, and ends every stream of
// messages Receive handed out. Closing a socket that is already closed does
// nothing and is not an error.
func (socket *Socket) Close() error {
	socket.guard.Lock()
	if socket.closed {
		socket.guard.Unlock()
		return nil
	}
	socket.closed = true
	close(socket.done)

	going := make([]*client, 0, len(socket.clients))
	for attached := range socket.clients {
		going = append(going, attached)
	}
	socket.clients = map[*client]struct{}{}
	for watching := range socket.watchers {
		delete(socket.watchers, watching)
		close(watching.messages)
	}
	socket.guard.Unlock()

	for _, attached := range going {
		attached.close()
	}
	if err := socket.listener.Close(); err != nil {
		return fmt.Errorf("cannot close the local socket at %s: %w", socket.options.Path, err)
	}
	return nil
}

// Clients is how many screens are connected, whether or not they are reading the
// event stream.
func (socket *Socket) Clients() int {
	socket.guard.Lock()
	defer socket.guard.Unlock()
	return len(socket.clients)
}

// Attached is how many screens are reading the event stream, which is what the
// status line reports and what says whether anyone is there to answer a preview.
func (socket *Socket) Attached() int {
	socket.guard.Lock()
	defer socket.guard.Unlock()
	count := 0
	for attached := range socket.clients {
		if attached.reading() {
			count++
		}
	}
	return count
}

// take gives one new connection its own reader, or turns it away when the socket
// already holds as many screens as it will.
func (socket *Socket) take(connection net.Conn) {
	attached := newClient(socket, connection)

	socket.guard.Lock()
	turnAway := socket.closed || len(socket.clients) >= socket.options.MaxClients
	if !turnAway {
		socket.clients[attached] = struct{}{}
	}
	socket.guard.Unlock()

	if turnAway {
		_ = attached.write(errorEnvelope(fmt.Sprintf(
			"the agent already has %d screens attached, which is as many as it will hold, so close one and try again",
			socket.options.MaxClients)))
		attached.close()
		return
	}
	go attached.read()
}

// forget takes one client off the socket's list, which happens when it hangs up
// or is hung up on.
func (socket *Socket) forget(attached *client) {
	socket.guard.Lock()
	defer socket.guard.Unlock()
	delete(socket.clients, attached)
}

// isClosed says whether the socket has been closed.
func (socket *Socket) isClosed() bool {
	socket.guard.Lock()
	defer socket.guard.Unlock()
	return socket.closed
}

// errorEnvelope is one thing gone wrong, in plain words, on its way to a screen.
func errorEnvelope(text string) contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketError, Text: text}
}

// check says whether the options carry everything the socket cannot run without.
func (options Options) check() error {
	switch {
	case options.Path == "":
		return errors.New("the socket needs a path to listen on, so pass the home folder's socket file")
	case options.Stream == nil:
		return errors.New("the socket needs the event stream to give attached screens, so pass the one the loop publishes to")
	case options.Queue == nil:
		return errors.New("the socket needs the queue to put messages in, so pass the one the loop drains")
	case options.Secrets == nil:
		return errors.New("the socket needs the vault to redact what it sends, so pass the secret store")
	case options.Clock == nil:
		return errors.New("the socket needs a clock to say when a message arrived, so pass the one the rest of the agent reads")
	case options.AnswerDeadline < 0:
		return errors.New("the socket was told to wait less than no time for an answer to a preview or a masked prompt, so pass the user's time_per_turn from the configuration, which is zero for no limit")
	}
	return nil
}

// fillIn puts the shipped limits into whatever the caller left at zero.
func (options *Options) fillIn() {
	if options.MaxClients <= 0 {
		options.MaxClients = DefaultMaxClients
	}
	if options.MaxLineBytes <= 0 {
		options.MaxLineBytes = DefaultMaxLineBytes
	}
}

// clearStaleSocket removes a socket file an earlier run left behind, and refuses
// a socket another copy of the agent is answering on, because two agents sharing
// one home folder would fight over the same database.
func clearStaleSocket(path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("cannot look at the socket file at %s to see whether it is in use: %w", path, err)
	}

	answering, err := net.DialTimeout("unix", path, time.Second)
	if err == nil {
		_ = answering.Close()
		return fmt.Errorf("another copy of Coeus is already listening on the socket at %s, so stop that one before starting this one", path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("cannot clear the socket file at %s that an earlier run left behind: %w", path, err)
	}
	return nil
}
