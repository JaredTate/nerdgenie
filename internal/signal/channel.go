// The shape of this channel was borrowed from ZeroClaw's Signal channel at
// ~/Code/zeroclaw/crates/zeroclaw-channels/src/signal.rs, which is where the
// idea of one channel over a daemon somebody else may be running comes from, and
// from Hermes' Signal platform at
// ~/Code/hermes-agent/gateway/platforms/signal.py, which is where the typing
// indicator around a reply and the pairing answer to a stranger come from. The
// Go here is written fresh.

package signal

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

const (
	// SignalChannelName is what this channel is called everywhere it is named.
	SignalChannelName = "signal"
	// InboundBacklog is how many inbound messages may wait for the agent before
	// the reader stops taking more off the daemon.
	InboundBacklog = 64
	// StillWorkingAfter is how long a task may run without a word before the
	// user is told it is still going. It is said once per task.
	StillWorkingAfter = 5 * time.Minute
	// StillWorkingNote is what that one message says.
	StillWorkingNote = "I am still working on that. I will send the result when it is done."
	// DefaultDaemonHost is where signal-cli serves when nothing else says.
	DefaultDaemonHost = "127.0.0.1"
	// DefaultDaemonPort is the port signal-cli serves on when nothing else says.
	DefaultDaemonPort = 8420
	// MaxAttachmentsPerMessage is how many files from one message are saved.
	MaxAttachmentsPerMessage = 10
)

// Channel is the Signal channel: everything a user says over Signal comes in
// through it and everything the agent says goes out through it.
type Channel struct {
	client  *Client
	stream  *Stream
	pairing *Pairing
	daemon  *Daemon
	clock   contract.Clock
	secrets contract.Secrets
	account string
	inbox   string

	inbound   chan contract.Inbound
	delivered atomic.Int64

	guard        sync.Mutex
	running      bool
	stop         context.CancelFunc
	lastSender   string
	waiting      chan contract.PreviewAnswer
	working      bool
	workingSince time.Time
	noted        bool
}

// Channel is a channel in the sense internal/contract means, and this line
// fails to compile the day it stops being one.
var _ contract.Channel = (*Channel)(nil)

// ChannelOptions is everything the Signal channel needs to be built.
type ChannelOptions struct {
	// Account is the phone number the daemon is linked to, which is what "nerdgenie
	// signal link" writes into the configuration.
	Account string
	// Program is the signal-cli to run. When it is empty, Coeus starts no
	// daemon and talks to one somebody else is already running at the host and
	// port below, which is what a machine running signal-cli in a container
	// wants.
	Program string
	// Host is where the daemon serves its HTTP interface. Empty means
	// 127.0.0.1.
	Host string
	// Port is the port it serves on. Zero means 8420.
	Port int
	// Home is the agent's home folder, which holds the pairing files, the
	// attachment cache, and the inbox.
	Home contract.Home
	// Clock is where every wait is measured.
	Clock contract.Clock
	// Secrets is the vault, whose redactor every outbound message goes through.
	Secrets contract.Secrets
}

// NewChannel builds the Signal channel. It talks to nothing until Receive is
// called, and the orchestrator wires it in serve.go.
func NewChannel(options ChannelOptions) (*Channel, error) {
	if options.Account == "" {
		return nil, errors.New("the Signal channel has no account, so run \"nerdgenie signal link\" before switching Signal on")
	}
	if options.Clock == nil {
		return nil, errors.New("the Signal channel has no clock, and every wait it makes is measured on one")
	}
	host, port := options.Host, options.Port
	if host == "" {
		host = DefaultDaemonHost
	}
	if port == 0 {
		port = DefaultDaemonPort
	}

	client, err := NewClient(ClientOptions{
		BaseAddress: fmt.Sprintf("http://%s:%d", host, port),
		Account:     options.Account,
		Home:        options.Home,
		Secrets:     options.Secrets,
	})
	if err != nil {
		return nil, err
	}
	pairing, err := NewPairing(options.Home, options.Clock)
	if err != nil {
		return nil, err
	}
	channel := &Channel{
		client:  client,
		stream:  NewStream(client, options.Clock),
		pairing: pairing,
		clock:   options.Clock,
		secrets: options.Secrets,
		account: options.Account,
		inbox:   options.Home.InboxFolder(),
		inbound: make(chan contract.Inbound, InboundBacklog),
	}
	return channel, channel.superviseWith(options, host, port)
}

// superviseWith gives the channel a daemon to look after, unless somebody else
// is running one.
func (channel *Channel) superviseWith(options ChannelOptions, host string, port int) error {
	if options.Program == "" {
		return nil
	}
	daemon, err := NewDaemon(DaemonOptions{
		Program: options.Program,
		Account: options.Account,
		Address: fmt.Sprintf("%s:%d", host, port),
		Clock:   options.Clock,
		Healthy: func(ctx context.Context) bool { return channel.client.Health(ctx).Healthy },
	})
	if err != nil {
		return err
	}
	channel.daemon = daemon
	return nil
}

// Name is what this channel is called.
func (channel *Channel) Name() string { return SignalChannelName }

// Pairing is the store of codes and approved senders, which the orchestrator
// hands to the pairing command so that both work on the same one.
func (channel *Channel) Pairing() *Pairing { return channel.pairing }

// Connections is how many times the event stream has been opened, which is how a
// caller or a test can see that it dropped and came back.
func (channel *Channel) Connections() int { return channel.stream.Connections() }

// Delivered is how many messages have been handed to the agent.
func (channel *Channel) Delivered() int { return int(channel.delivered.Load()) }

// Receive starts the daemon if this Coeus is the one running it, opens the event
// stream, and hands back the messages from paired senders. Calling it again
// hands back the same stream rather than opening a second one.
func (channel *Channel) Receive(ctx context.Context) (<-chan contract.Inbound, error) {
	channel.guard.Lock()
	already := channel.running
	channel.guard.Unlock()
	if already {
		return channel.streamFor(ctx), nil
	}

	if channel.daemon != nil {
		if err := channel.daemon.Start(ctx); err != nil {
			return nil, err
		}
	}
	// The daemon, its event stream, and the quiet watcher live as long as the
	// channel, not as long as one caller's attach: a reader that goes away
	// must not take the stream that every other reader and the preview answer
	// ride on. Close ends them.
	running, stop := context.WithCancel(context.Background())

	channel.guard.Lock()
	channel.running = true
	channel.stop = stop
	channel.guard.Unlock()

	// The ticker that watches for a quiet task is started here rather than
	// inside the watcher, so that by the time Receive has returned the clock is
	// already being watched and no tick can be missed.
	quiet := channel.clock.NewTicker(StillWorkingAfter)
	go func() { _ = channel.stream.Run(running, func(event Event) { channel.route(running, event) }) }()
	go channel.watchForQuiet(running, quiet)
	return channel.streamFor(ctx), nil
}

// streamFor hands one caller its own stream of the inbound messages, fed from
// the channel's one queue, and closes it when that caller's context ends, which
// is what the channel contract promises a reader. The queue itself outlives any
// one reader, so a second attach after the first was cancelled still works.
func (channel *Channel) streamFor(ctx context.Context) <-chan contract.Inbound {
	stream := make(chan contract.Inbound, InboundBacklog)
	go func() {
		defer close(stream)
		for {
			select {
			case <-ctx.Done():
				return
			case message, open := <-channel.inbound:
				if !open {
					return
				}
				select {
				case stream <- message:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return stream
}

// Send delivers one reply, split into as few messages as Signal's length allows,
// with a typing indicator while it is being written.
//
// The whole reply goes through the redactor before it is split, never each piece
// after: the redactor looks for whole values, so a secret that falls across a
// split matches neither half and would leave in two messages a reader joins back
// up. The client redacts every message again on its way out, which costs nothing
// and covers the messages that are not replies.
func (channel *Channel) Send(ctx context.Context, text string) error {
	pieces := SplitReply(channel.secrets.Redact(text))
	if len(pieces) == 0 {
		return nil
	}
	recipient := channel.recipient()
	_ = channel.client.Typing(ctx, recipient, false)
	defer func() { _ = channel.client.Typing(ctx, recipient, true) }()

	for _, piece := range pieces {
		if err := channel.client.Send(ctx, recipient, piece, nil); err != nil {
			return err
		}
	}
	channel.stopWorking()
	return nil
}

// SendFile delivers one file with a line about it, which is how a handoff
// arrives with its screenshot.
func (channel *Channel) SendFile(ctx context.Context, path string, caption string) error {
	return channel.client.Send(ctx, channel.recipient(), caption, []string{path})
}

// AskSecret always refuses, because Signal shows what is typed to anybody
// looking at the phone and keeps it in the conversation afterwards.
func (channel *Channel) AskSecret(context.Context, string) (string, error) {
	return "", contract.ErrNoMaskedPrompt
}

// Health says whether signal-cli is answering.
func (channel *Channel) Health(ctx context.Context) contract.ChannelHealth {
	return channel.client.Health(ctx)
}

// Close stops the event stream and, when this Coeus started it, signal-cli.
func (channel *Channel) Close() error {
	channel.guard.Lock()
	stop := channel.stop
	channel.stop = nil
	channel.running = false
	channel.guard.Unlock()

	if stop != nil {
		stop()
	}
	if channel.daemon != nil {
		return channel.daemon.Stop()
	}
	return nil
}

// route decides what to do with one message the daemon reported: a group message
// is left alone, a stranger gets a pairing code and nothing else, an answer to a
// waiting preview is that answer, and everything else goes to the agent.
func (channel *Channel) route(ctx context.Context, event Event) {
	if event.Sender == "" || event.GroupID != "" {
		return
	}
	if !channel.pairing.IsApproved(event.Sender) {
		channel.offerPairing(ctx, event.Sender)
		return
	}
	channel.remember(event.Sender)
	if channel.answerPreview(event.Text) {
		return
	}

	message := contract.Inbound{
		ID:          strconv.FormatInt(event.Timestamp, 10),
		Sender:      event.Sender,
		Text:        event.Text,
		Attachments: channel.saveIntoInbox(ctx, event),
		Received:    receivedAt(event, channel.clock),
		Channel:     SignalChannelName,
	}
	channel.startWorking()
	select {
	case channel.inbound <- message:
		channel.delivered.Add(1)
	case <-ctx.Done():
	}
}

// offerPairing gives a sender the agent does not know a code and nothing else. A
// sender who has already asked within the last ten minutes is told nothing at
// all, because answering every message would be a reply anybody could ask for
// again and again.
func (channel *Channel) offerPairing(ctx context.Context, sender string) {
	code, offered, err := channel.pairing.Offer(sender)
	if err != nil || !offered {
		return
	}
	_ = channel.client.Send(ctx, sender, PairingMessage(code), nil)
}

// recipient is who a reply goes to: the last person who wrote, and the account
// itself when nobody has written yet, which is the note a person sends
// themselves.
func (channel *Channel) recipient() string {
	channel.guard.Lock()
	defer channel.guard.Unlock()
	if channel.lastSender != "" {
		return channel.lastSender
	}
	return channel.account
}

// remember writes down who last wrote, so that a reply goes back to them.
func (channel *Channel) remember(sender string) {
	channel.guard.Lock()
	defer channel.guard.Unlock()
	channel.lastSender = sender
}

// startWorking marks the moment a task began, which is what the quiet note is
// measured from.
func (channel *Channel) startWorking() {
	channel.guard.Lock()
	defer channel.guard.Unlock()
	channel.working = true
	channel.workingSince = channel.clock.Now()
	channel.noted = false
}

// stopWorking marks the task as answered, so no quiet note follows it.
func (channel *Channel) stopWorking() {
	channel.guard.Lock()
	defer channel.guard.Unlock()
	channel.working = false
}

// watchForQuiet says once, five minutes into a task that has said nothing, that
// the agent is still working.
func (channel *Channel) watchForQuiet(ctx context.Context, ticking contract.Ticker) {
	defer ticking.Stop()
	for {
		select {
		case <-ticking.Ticks():
			channel.noteIfQuiet(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// noteIfQuiet sends the one note when a task has been running quietly for long
// enough and has not been noted yet.
func (channel *Channel) noteIfQuiet(ctx context.Context) {
	channel.guard.Lock()
	quietFor := channel.clock.Now().Sub(channel.workingSince)
	due := channel.working && !channel.noted && quietFor >= StillWorkingAfter
	if due {
		channel.noted = true
	}
	recipient := channel.lastSender
	if recipient == "" {
		recipient = channel.account
	}
	channel.guard.Unlock()

	if due {
		_ = channel.client.Send(ctx, recipient, StillWorkingNote, nil)
	}
}

// receivedAt is when Signal says the message was sent, and the machine's own
// time when the daemon reported none.
func receivedAt(event Event, clock contract.Clock) time.Time {
	if event.Timestamp > 0 {
		return time.UnixMilli(event.Timestamp).UTC()
	}
	return clock.Now()
}
