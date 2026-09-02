package tui

import (
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// tickMessage is the screen's own heartbeat. It carries the time from
// contract.Clock, which is what moves the spinner on and what flushes the deltas
// that have arrived since the last one.
type tickMessage struct {
	// at is the moment the clock had reached when the heartbeat was made.
	at time.Time
}

// envelopeMessage carries one message the running program sent over the socket
// into the screen's update function.
type envelopeMessage struct {
	// envelope is the message itself, in the shape internal/contract defines.
	envelope contract.SocketEnvelope
}
