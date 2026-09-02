package tui

import "time"

// tickMessage is the screen's own heartbeat. It carries the time from
// contract.Clock, which is what moves the spinner on and what flushes the deltas
// that have arrived since the last one.
type tickMessage struct {
	// at is the moment the clock had reached when the heartbeat was made.
	at time.Time
}
