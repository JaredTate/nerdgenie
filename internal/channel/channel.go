// The design in this file is borrowed from ZeroClaw's channel trait at
// ~/Code/zeroclaw/crates/zeroclaw-api/src/channel.rs and written fresh in Go.
// There every way of talking to the agent is one small trait: a name, a send, a
// listen that pushes what arrives into one shared sender, an approval prompt
// that comes back as one of a fixed set of answers, and a health check, so the
// runtime never learns which surface it is talking to. Nerd Genie keeps that shape in
// contract.Channel and makes the local socket the first thing to implement it,
// so the terminal is a channel exactly as Signal is.

package channel

import (
	"context"
	"fmt"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The socket is the first real channel, so the compiler is asked to say at once
// if it ever stops matching the contract every other package writes against.
var _ contract.Channel = (*Socket)(nil)

// watcherBacklog is how many messages one caller of Receive may fall behind by.
// The queue is where a message is kept safe, so a watcher that stops reading
// loses its copy rather than holding the socket up.
const watcherBacklog = 64

// MaxWatchers is how many callers of Receive the socket carries at once. Every
// watcher costs a buffer of its own, and a program with more of them than this
// watching one socket is a program that has forgotten to let one go.
const MaxWatchers = 8

// watcher is one caller of Receive and the messages going to it.
type watcher struct {
	// messages is where the caller reads what the socket took in.
	messages chan contract.Inbound
}

// Name is the channel's name, which every message from a screen arrives under.
// It is contract.TerminalChannelName and nothing of this package's own, because
// a command that may only run in the terminal is checked against that one name
// and two spellings of it would let such a command run over Signal.
func (socket *Socket) Name() string {
	return contract.TerminalChannelName
}

// Receive is the live copy of everything the socket takes in. Every message a
// screen sends is written into the queue first, and the queue is the one path to
// the loop and the only one that survives a restart, so this stream is for
// anything that wants to watch rather than the way work is handed on. It closes
// when the context is cancelled or the socket closes, and a caller that stops
// reading loses its copies rather than holding a screen up.
func (socket *Socket) Receive(ctx context.Context) (<-chan contract.Inbound, error) {
	socket.guard.Lock()
	if socket.closed {
		socket.guard.Unlock()
		return nil, fmt.Errorf("the local socket at %s is closed, so there is nothing to receive from it", socket.options.Path)
	}
	if len(socket.watchers) >= MaxWatchers {
		socket.guard.Unlock()
		return nil, fmt.Errorf("the local socket at %s already carries %d watchers of what it receives, which is the limit, so let one go before starting another", socket.options.Path, MaxWatchers)
	}
	watching := &watcher{messages: make(chan contract.Inbound, watcherBacklog)}
	socket.watchers[watching] = struct{}{}
	socket.guard.Unlock()

	go func() {
		select {
		case <-ctx.Done():
		case <-socket.done:
		}
		socket.forgetWatcher(watching)
	}()
	return watching.messages, nil
}

// Send delivers one reply to every attached screen. With no screen attached it
// does nothing and says so with no error, because the reply is in the event log
// either way and the loop must not stop for want of an audience.
func (socket *Socket) Send(ctx context.Context, text string) error {
	socket.endStreaming()
	return socket.toEveryScreen(ctx, contract.SocketEnvelope{Type: contract.SocketReply, Text: text})
}

// SendFile delivers one file with its caption. The screen is given the path,
// which it can read because it runs as the same user account.
func (socket *Socket) SendFile(ctx context.Context, path string, caption string) error {
	return socket.toEveryScreen(ctx, contract.SocketEnvelope{
		Type:        contract.SocketReply,
		Text:        caption,
		Attachments: []string{path},
	})
}

// Health says whether the socket is still listening, and why not when it is not.
func (socket *Socket) Health(_ context.Context) contract.ChannelHealth {
	if socket.isClosed() {
		return contract.ChannelHealth{
			Healthy: false,
			Detail:  fmt.Sprintf("the local socket at %s is closed, so no screen can attach; start the agent again", socket.options.Path),
		}
	}
	return contract.ChannelHealth{Healthy: true}
}

// toEveryScreen writes one message to every attached screen.
func (socket *Socket) toEveryScreen(ctx context.Context, envelope contract.SocketEnvelope) error {
	_, err := socket.writeToScreens(ctx, envelope)
	return err
}

// writeToScreens writes one message to every attached screen and says how many
// screens took it, which is how a question knows whether anyone could have
// answered it. A screen that cannot be written to is hung up on rather than
// allowed to hold up the rest.
func (socket *Socket) writeToScreens(ctx context.Context, envelope contract.SocketEnvelope) (int, error) {
	if socket.isClosed() {
		return 0, fmt.Errorf("the local socket at %s is closed, so the %s has nowhere to go", socket.options.Path, envelope.Type)
	}
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("the %s was not sent to the attached screens: %w", envelope.Type, err)
	}

	written := 0
	for _, attached := range socket.attachedClients() {
		if err := attached.write(envelope); err != nil {
			attached.close()
			continue
		}
		written++
	}
	return written, nil
}

// attachedClients is every screen reading the event stream, copied out from
// under the lock so that writing to a slow screen cannot hold the socket up.
func (socket *Socket) attachedClients() []*client {
	socket.guard.Lock()
	defer socket.guard.Unlock()
	reading := make([]*client, 0, len(socket.clients))
	for attached := range socket.clients {
		if attached.reading() {
			reading = append(reading, attached)
		}
	}
	return reading
}

// deliver hands one message the socket took in to everyone watching what this
// channel receives.
func (socket *Socket) deliver(message contract.Inbound) {
	socket.guard.Lock()
	defer socket.guard.Unlock()
	for watching := range socket.watchers {
		select {
		case watching.messages <- message:
		default:
		}
	}
}

// forgetWatcher ends one caller's stream of messages, once and once only.
func (socket *Socket) forgetWatcher(watching *watcher) {
	socket.guard.Lock()
	defer socket.guard.Unlock()
	if _, held := socket.watchers[watching]; !held {
		return
	}
	delete(socket.watchers, watching)
	close(watching.messages)
}

// redact puts every piece of text in a message on its way to a screen through
// the vault's redactor, and empties the field a secret would ride in, because
// nothing the program sends out ever carries a secret.
func (socket *Socket) redact(envelope contract.SocketEnvelope) contract.SocketEnvelope {
	envelope.Text = socket.options.Secrets.Redact(envelope.Text)
	envelope.Title = socket.options.Secrets.Redact(envelope.Title)
	envelope.Reason = socket.options.Secrets.Redact(envelope.Reason)
	envelope.Secret = ""
	if len(envelope.Fields) > 0 {
		fields := make(map[string]string, len(envelope.Fields))
		for name, value := range envelope.Fields {
			fields[name] = socket.options.Secrets.Redact(value)
		}
		envelope.Fields = fields
	}
	return envelope
}
