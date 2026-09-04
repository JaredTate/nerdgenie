package tui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The bounds on the local socket. A line past the cap is thrown away rather than
// read into memory, and a dial that hangs gives up rather than holding the first
// frame's promise open forever.
const (
	// maxSocketLineBytes is the longest line the screen will read. The program
	// sends deltas and previews, not files, so a megabyte is far more than a
	// well-behaved program ever needs.
	maxSocketLineBytes = 1 << 20
	// dialTimeout is how long one attempt to reach the program may take.
	dialTimeout = 3 * time.Second
	// readBufferBytes is how much of the socket is read at a time.
	readBufferBytes = 64 << 10
)

// errLineTooLong means a line on the socket went past the cap and the rest of it
// was thrown away.
var errLineTooLong = errors.New("a line on the socket was longer than the screen will read, so it was dropped")

// UnixDialer opens the local socket the running program listens on, which is the
// only way the terminal ever reaches it.
type UnixDialer struct {
	path string
}

// NewUnixDialer returns a dialer for the socket inside a home folder.
func NewUnixDialer(home contract.Home) *UnixDialer {
	return &UnixDialer{path: home.SocketFile()}
}

// Dial opens one link to the program, and says in plain words what to do when
// there is nothing to reach.
func (dialer *UnixDialer) Dial(ctx context.Context) (Connection, error) {
	reaching := net.Dialer{Timeout: dialTimeout}
	link, err := reaching.DialContext(ctx, "unix", dialer.path)
	if err != nil {
		return nil, fmt.Errorf("cannot reach the running program at %s, so start it with \"coeus serve\": %w", dialer.path, err)
	}
	return &socketConnection{link: link, reader: bufio.NewReaderSize(link, readBufferBytes)}, nil
}

// socketConnection is one open link over the local socket, speaking one JSON
// object per line.
type socketConnection struct {
	link    net.Conn
	reader  *bufio.Reader
	writing sync.Mutex
}

// Receive reads the next message. A line that is too long or that is not a
// message the two sides agree on comes back as an error message rather than
// closing the link, because one bad line is not a reason to lose the session.
func (connection *socketConnection) Receive() (contract.SocketEnvelope, error) {
	for {
		line, err := readCappedLine(connection.reader, maxSocketLineBytes)
		switch {
		case errors.Is(err, errLineTooLong):
			return troubleEnvelope("A line from the running program was too long to read, so it was dropped."), nil
		case err != nil:
			return contract.SocketEnvelope{}, err
		}
		envelope, err := contract.DecodeSocketEnvelope(line)
		switch {
		case errors.Is(err, contract.ErrEmptySocketLine):
			continue
		case err != nil:
			return troubleEnvelope("A line from the running program could not be read: " + err.Error()), nil
		}
		return envelope, nil
	}
}

// Send writes one message as a single line.
func (connection *socketConnection) Send(envelope contract.SocketEnvelope) error {
	connection.writing.Lock()
	defer connection.writing.Unlock()
	return contract.EncodeSocketEnvelope(connection.link, envelope)
}

// Close ends the link.
func (connection *socketConnection) Close() error {
	return connection.link.Close()
}

// troubleEnvelope is how a bad line reaches the screen: as the same error
// message the program itself would have sent, so that there is one path for
// everything that goes wrong.
func troubleEnvelope(what string) contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketError, Reason: what}
}

// readCappedLine reads one line, and refuses one longer than the cap. The rest
// of a line that was refused is thrown away, so that the line after it is read
// properly rather than as the tail of a line nobody saw.
func readCappedLine(reader *bufio.Reader, limit int) ([]byte, error) {
	line := []byte{}
	dropped := false
	for {
		piece, more, err := reader.ReadLine()
		if err != nil {
			return nil, err
		}
		if !dropped && len(line)+len(piece) > limit {
			dropped = true
			line = nil
		}
		if !dropped {
			line = append(line, piece...)
		}
		if !more {
			break
		}
	}
	if dropped {
		return nil, errLineTooLong
	}
	return line, nil
}
