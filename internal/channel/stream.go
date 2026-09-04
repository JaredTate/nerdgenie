// The design in this file is borrowed from OpenCode's event bus at
// ~/Code/opencode/packages/opencode/src/bus/global.ts, which the session
// processor at ~/Code/opencode/packages/opencode/src/session/processor.ts
// publishes every turn event to, and it is written fresh in Go. There one
// emitter carries every event and any number of listeners attach to it, so the
// part producing the events never learns who is reading them. Nerd Genie keeps that
// shape and adds the one thing a long-running agent needs: a reader that falls
// behind is dropped rather than allowed to stall the loop, because the loop must
// never wait on a screen.

package channel

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// SubscriberBacklog is how many events one subscriber may fall behind by before
// it is dropped. It is large enough for a screen that pauses for a moment and
// small enough that a screen nobody is watching cannot fill memory.
const SubscriberBacklog = 256

// MaxSubscribers is how many readers one stream carries at once. Every attached
// screen is one reader, and a machine with more screens than this attached is a
// machine with something wrong on it.
const MaxSubscribers = 32

// StatusHeartbeat is how often the stream says how the agent is doing when
// nothing else is happening. A screen calls the program gone when no status has
// reached it for ten seconds, so this is half of that: often enough to keep the
// health mark lit, far enough apart to cost nothing.
const StatusHeartbeat = 5 * time.Second

// StreamOptions are what the stream needs beyond the events the loop publishes.
// The two go together: a stream given neither carries only what is published to
// it, which is all a caller that has nothing to say about the agent needs.
type StreamOptions struct {
	// Clock is where the heartbeat counts its five seconds.
	Clock contract.Clock
	// Status returns what a screen has to be told about the agent right now:
	// the state word, the command list the palette is filled from, and whatever
	// else the program knows, under the contract.StatusField names.
	// cmd/nerdgenie/serve.go fills it in.
	Status func() map[string]string
}

// Stream is the one feed of events from the agent loop, which every attached
// channel subscribes to. The loop publishes deltas, replies, previews,
// questions, handoffs, status, and errors, and never learns which channels are
// listening.
//
// The stream adds one message of its own: a status, sent to a screen the moment
// it attaches and again on every heartbeat, so that a screen's palette is filled
// and its health mark stays lit without the loop having to remember to say so.
type Stream struct {
	options     StreamOptions
	stopBeating context.CancelFunc

	guard       sync.Mutex
	subscribers map[*Subscription]struct{}
	closed      bool
}

// NewStream returns an empty stream with nobody listening to it yet. A stream
// given both a clock and something to say about the agent starts its heartbeat
// at once, and Close stops it.
func NewStream(options StreamOptions) *Stream {
	stream := &Stream{options: options, subscribers: map[*Subscription]struct{}{}}
	if options.Clock == nil || options.Status == nil {
		return stream
	}

	beating, stopBeating := context.WithCancel(context.Background())
	stream.stopBeating = stopBeating
	go stream.beat(beating)
	return stream
}

// beat sends one status every StatusHeartbeat until the stream is closed, so
// that a screen watching a quiet agent still knows the agent is there.
func (stream *Stream) beat(ctx context.Context) {
	for {
		if err := stream.options.Clock.Sleep(ctx, StatusHeartbeat); err != nil {
			return
		}
		status, worth := stream.statusToSend()
		if !worth {
			continue
		}
		if err := stream.Publish(status); err != nil {
			// The only thing Publish refuses an already checked status for is a
			// closed stream, and a closed stream is the end of the heartbeat.
			return
		}
	}
}

// statusToSend is what the stream has to say about the agent right now, and
// whether that is anything at all. A status whose state word is not one a screen
// knows is held back, because Publish refuses such a status and the two paths
// must agree.
func (stream *Stream) statusToSend() (contract.SocketEnvelope, bool) {
	if stream.options.Status == nil {
		return contract.SocketEnvelope{}, false
	}
	fields := stream.options.Status()
	if len(fields) == 0 {
		return contract.SocketEnvelope{}, false
	}
	status := contract.SocketEnvelope{Type: contract.SocketStatus, Fields: fields}
	if err := checkStatusWords(status); err != nil {
		return contract.SocketEnvelope{}, false
	}
	return status, true
}

// Subscribe adds one reader and returns its subscription. It refuses once the
// stream holds MaxSubscribers readers, and refuses on a closed stream.
func (stream *Stream) Subscribe() (*Subscription, error) {
	stream.guard.Lock()
	defer stream.guard.Unlock()
	if stream.closed {
		return nil, fmt.Errorf("the event stream is closed, so nothing more can be read from it")
	}
	if len(stream.subscribers) >= MaxSubscribers {
		return nil, fmt.Errorf("the event stream already carries %d readers, which is the limit, so detach a screen before attaching another", MaxSubscribers)
	}
	subscription := &Subscription{
		stream: stream,
		events: make(chan contract.SocketEnvelope, SubscriberBacklog),
	}
	stream.subscribers[subscription] = struct{}{}

	// A screen that has just attached knows nothing about the agent, so it is
	// told at once rather than left with an empty palette and a hollow health
	// mark until the loop next publishes something of its own. The subscription
	// is new, so there is always room for this one message.
	if status, worth := stream.statusToSend(); worth {
		subscription.events <- status
	}
	return subscription, nil
}

// Publish hands one event to every reader. A reader that is already
// SubscriberBacklog events behind is dropped, its stream closed, and the rest go
// on untouched, because one screen nobody is watching must never stall the loop.
func (stream *Stream) Publish(envelope contract.SocketEnvelope) error {
	if !envelope.Type.FromProgram() {
		return fmt.Errorf("the event stream carries only what the program sends, and %q is not one of those, so publish a delta, a reply, a preview, a question, a handoff, a status, or an error", envelope.Type)
	}
	if err := checkStatusWords(envelope); err != nil {
		return err
	}

	stream.guard.Lock()
	defer stream.guard.Unlock()
	if stream.closed {
		return fmt.Errorf("the event stream is closed, so the %q event has nowhere to go", envelope.Type)
	}
	for subscription := range stream.subscribers {
		select {
		case subscription.events <- envelope:
		default:
			delete(stream.subscribers, subscription)
			subscription.finish(true)
		}
	}
	return nil
}

// checkStatusWords holds the one thing the stream knows about what a status
// says: its state field carries one of the five words contract.State names, so
// that the program and every screen read the same spelling. The fields
// themselves are the contract.StatusField names, and a field a screen does not
// know is a field it ignores, so a status may carry as many as the program has.
func checkStatusWords(envelope contract.SocketEnvelope) error {
	if envelope.Type != contract.SocketStatus {
		return nil
	}
	state, given := envelope.Fields[contract.StatusFieldState]
	if !given || contract.KnownScreenState(state) {
		return nil
	}
	return fmt.Errorf("a status says the agent is %q, which is not one of the words a screen knows, so use one of %s, %s, %s, %s, or %s",
		state, contract.StateIdle, contract.StateThinking, contract.StateUsingTool,
		contract.StateWaitingForYou, contract.StatePaused)
}

// Subscribers is how many readers the stream carries, which is what the socket
// counts its attached screens with.
func (stream *Stream) Subscribers() int {
	stream.guard.Lock()
	defer stream.guard.Unlock()
	return len(stream.subscribers)
}

// Close ends every subscription and refuses anything published afterwards.
// Closing a stream that is already closed does nothing and is not an error.
func (stream *Stream) Close() {
	stream.guard.Lock()
	defer stream.guard.Unlock()
	if stream.closed {
		return
	}
	stream.closed = true
	if stream.stopBeating != nil {
		stream.stopBeating()
	}
	for subscription := range stream.subscribers {
		delete(stream.subscribers, subscription)
		subscription.finish(false)
	}
}

// Subscription is one reader's place in the stream. Its events arrive on the
// channel Events returns, and that channel closes when the reader leaves, when
// the stream closes, or when the reader falls too far behind.
type Subscription struct {
	stream  *Stream
	events  chan contract.SocketEnvelope
	guard   sync.Mutex
	dropped bool
	ended   bool
}

// Events is where this reader's events arrive. It is closed when the
// subscription ends, for whatever reason.
func (subscription *Subscription) Events() <-chan contract.SocketEnvelope {
	return subscription.events
}

// Dropped says whether this reader was dropped for falling past the backlog cap,
// which is what tells a screen it fell behind rather than being sent away.
func (subscription *Subscription) Dropped() bool {
	subscription.guard.Lock()
	defer subscription.guard.Unlock()
	return subscription.dropped
}

// Close takes this reader off the stream. Closing a subscription that has
// already ended does nothing and is not an error.
func (subscription *Subscription) Close() {
	subscription.stream.guard.Lock()
	delete(subscription.stream.subscribers, subscription)
	subscription.stream.guard.Unlock()
	subscription.finish(false)
}

// finish closes the reader's channel once and records why. The caller holds the
// stream's lock whenever the stream itself is doing the finishing.
func (subscription *Subscription) finish(dropped bool) {
	subscription.guard.Lock()
	defer subscription.guard.Unlock()
	if subscription.ended {
		return
	}
	subscription.ended, subscription.dropped = true, dropped
	close(subscription.events)
}
