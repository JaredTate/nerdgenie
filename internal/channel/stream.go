// The design in this file is borrowed from OpenCode's event bus at
// ~/Code/opencode/packages/opencode/src/bus/global.ts, which the session
// processor at ~/Code/opencode/packages/opencode/src/session/processor.ts
// publishes every turn event to, and it is written fresh in Go. There one
// emitter carries every event and any number of listeners attach to it, so the
// part producing the events never learns who is reading them. Coeus keeps that
// shape and adds the one thing a long-running agent needs: a reader that falls
// behind is dropped rather than allowed to stall the loop, because the loop must
// never wait on a screen.

package channel

import (
	"fmt"
	"sync"

	"github.com/JaredTate/coeus/internal/contract"
)

// SubscriberBacklog is how many events one subscriber may fall behind by before
// it is dropped. It is large enough for a screen that pauses for a moment and
// small enough that a screen nobody is watching cannot fill memory.
const SubscriberBacklog = 256

// MaxSubscribers is how many readers one stream carries at once. Every attached
// screen is one reader, and a machine with more screens than this attached is a
// machine with something wrong on it.
const MaxSubscribers = 32

// Stream is the one feed of events from the agent loop, which every attached
// channel subscribes to. The loop publishes deltas, replies, previews,
// questions, handoffs, status, and errors, and never learns which channels are
// listening.
type Stream struct {
	guard       sync.Mutex
	subscribers map[*Subscription]struct{}
	closed      bool
}

// NewStream returns an empty stream with nobody listening to it yet.
func NewStream() *Stream {
	return &Stream{subscribers: map[*Subscription]struct{}{}}
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
	return subscription, nil
}

// Publish hands one event to every reader. A reader that is already
// SubscriberBacklog events behind is dropped, its stream closed, and the rest go
// on untouched, because one screen nobody is watching must never stall the loop.
func (stream *Stream) Publish(envelope contract.SocketEnvelope) error {
	if !envelope.Type.FromProgram() {
		return fmt.Errorf("the event stream carries only what the program sends, and %q is not one of those, so publish a delta, a reply, a preview, a question, a handoff, a status, or an error", envelope.Type)
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
