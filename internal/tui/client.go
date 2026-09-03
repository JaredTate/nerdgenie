package tui

import (
	"context"
	"errors"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/JaredTate/coeus/internal/contract"
)

// The bounds on dialling. A program that is not running must never turn into a
// tight loop, and the person must never wait more than a few seconds for the
// screen to notice that it has come back.
const (
	// firstReconnectWait is how long the client waits after the first failure.
	firstReconnectWait = 250 * time.Millisecond
	// maxReconnectWait is the longest it ever waits between tries.
	maxReconnectWait = 10 * time.Second
	// eventsWaiting is how many messages the client holds for the screen before
	// it stops reading, which is what stops a fast program from filling memory.
	eventsWaiting = 256
)

// Connection is one open link to the running program: envelopes come off it one
// at a time, and go onto it one at a time.
type Connection interface {
	// Receive hands over the next message the program sent, or says why the link
	// is gone.
	Receive() (contract.SocketEnvelope, error)
	// Send delivers one message to the program.
	Send(envelope contract.SocketEnvelope) error
	// Close ends the link.
	Close() error
}

// Dialer opens a connection to the program's local socket. The screen is given
// one rather than making its own, so that a test can drive the link.
type Dialer interface {
	// Dial opens one connection, or says in plain words why it could not.
	Dial(ctx context.Context) (Connection, error)
}

// Client keeps the screen's link to the running program. It dials, attaches,
// reads envelopes into the screen's messages, and dials again on a growing wait
// when the link drops, for as long as the screen is on the terminal.
type Client struct {
	dialer Dialer
	clock  contract.Clock
	events chan tea.Msg

	stop   context.CancelFunc
	ground context.Context

	guard sync.Mutex
	open  Connection
}

// NewClient makes a link that is not yet dialling.
func NewClient(dialer Dialer, clock contract.Clock) *Client {
	ground, stop := context.WithCancel(context.Background())
	return &Client{
		dialer: dialer,
		clock:  clock,
		events: make(chan tea.Msg, eventsWaiting),
		ground: ground,
		stop:   stop,
	}
}

// Start begins dialling and returns the stream of things the screen must know
// about: the link coming up and going down, and every message the program sends.
func (client *Client) Start() <-chan tea.Msg {
	go client.keepTrying()
	return client.events
}

// Close detaches, stops dialling, and lets everything go.
func (client *Client) Close() {
	client.tellProgram(contract.SocketEnvelope{Type: contract.SocketDetach})
	client.stop()
	client.guard.Lock()
	open := client.open
	client.open = nil
	client.guard.Unlock()
	if open != nil {
		_ = open.Close()
	}
}

// Send hands one envelope to the program, and says so plainly when there is no
// link to hand it to, so that nothing the person typed is ever lost quietly.
func (client *Client) Send(envelope contract.SocketEnvelope) error {
	return client.tellProgram(envelope)
}

// tellProgram writes one envelope on whatever link is open.
func (client *Client) tellProgram(envelope contract.SocketEnvelope) error {
	client.guard.Lock()
	open := client.open
	client.guard.Unlock()
	if open == nil {
		return errors.New("there is no link to the running program right now, so wait for the status strip to say it is back")
	}
	return open.Send(envelope)
}

// keepTrying is the whole life of the link: dial, attach, read until it breaks,
// wait, and dial again.
func (client *Client) keepTrying() {
	defer close(client.events)
	wait := firstReconnectWait
	for client.ground.Err() == nil {
		if client.oneConnection() {
			wait = firstReconnectWait
		}
		if client.clock.Sleep(client.ground, wait) != nil {
			return
		}
		wait = longerWait(wait)
	}
}

// oneConnection dials once and reads from the link until it breaks. It says
// whether the link was ever open, which is what resets the wait.
func (client *Client) oneConnection() bool {
	open, err := client.dialer.Dial(client.ground)
	if err != nil {
		client.say(linkMessage{up: false, detail: err.Error()})
		return false
	}
	client.guard.Lock()
	client.open = open
	client.guard.Unlock()

	if err := open.Send(contract.SocketEnvelope{Type: contract.SocketAttach}); err != nil {
		client.dropped(open, err)
		return false
	}
	client.say(linkMessage{up: true})
	client.readUntilItBreaks(open)
	return true
}

// readUntilItBreaks passes on every envelope the program sends until the link
// fails, and then says that it is down.
func (client *Client) readUntilItBreaks(open Connection) {
	for {
		envelope, err := open.Receive()
		if err != nil {
			client.dropped(open, err)
			return
		}
		client.say(envelopeMessage{envelope: envelope})
	}
}

// dropped forgets a link that has failed and tells the screen why.
func (client *Client) dropped(open Connection, why error) {
	client.guard.Lock()
	if client.open == open {
		client.open = nil
	}
	client.guard.Unlock()
	_ = open.Close()
	client.say(linkMessage{up: false, detail: why.Error()})
}

// say puts one message on the stream, and gives up on it when the screen has
// gone, so that nothing here can block forever.
func (client *Client) say(message tea.Msg) {
	select {
	case client.events <- message:
	case <-client.ground.Done():
	}
}

// longerWait doubles the wait between tries, up to the cap.
func longerWait(wait time.Duration) time.Duration {
	longer := wait * 2
	if longer > maxReconnectWait {
		return maxReconnectWait
	}
	return longer
}
