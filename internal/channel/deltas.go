package channel

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// deltaCoalesce is how long the pieces of a reply are gathered before they go
// to the screens, which the design sets at thirty milliseconds so that a
// terminal is not repainted for every word. The first piece goes out at once.
const deltaCoalesce = 30 * time.Millisecond

// deltaHoldBackRunes is how much of the end of the streamed text waits before it
// is shown, so that a secret cannot leave the program in pieces: a secret that
// has not finished arriving cannot be recognised yet, and holding back this
// much keeps every secret up to this length off the screen until the redactor
// has seen all of it. Sixty-four covers the keys the vault usually holds; a
// longer secret can show its first runes for a moment, and is withdrawn the
// instant it is recognised. A reply shorter than this streams no piece at all
// and arrives whole, which for a reply that short is no slower.
const deltaHoldBackRunes = 64

// replyInProgress is one reply as it streams: everything the model has written
// so far, what the screens have been shown of it, and what is waiting to go.
type replyInProgress struct {
	raw       strings.Builder
	shown     string
	gathered  string
	lastFlush time.Time
}

// SendDelta hands every attached screen one more piece of the reply the model
// is writing. The piece is redacted together with everything before it, the end
// of the text is held back, and what the screens have already seen is checked
// against the new redaction: a secret that has just been recognised inside the
// shown text withdraws it and shows the redacted text over.
func (socket *Socket) SendDelta(ctx context.Context, text string) error {
	socket.deltaGuard.Lock()
	defer socket.deltaGuard.Unlock()
	reply := socket.replyInProgress()
	reply.raw.WriteString(text)
	visible := withoutTail(socket.options.Secrets.Redact(reply.raw.String()), deltaHoldBackRunes)
	if !strings.HasPrefix(visible, reply.shown) {
		if err := socket.toEveryScreen(ctx, contract.SocketEnvelope{Type: contract.SocketDelta, Reset: true}); err != nil {
			return err
		}
		reply.shown, reply.gathered = "", ""
	}
	reply.gathered += visible[len(reply.shown):]
	reply.shown = visible
	now := socket.options.Clock.Now()
	if now.Sub(reply.lastFlush) < deltaCoalesce {
		return nil
	}
	return socket.flushGathered(ctx, reply, now)
}

// FinishDelta sends the end of the reply the screens were shown as it streamed:
// the runes held back for the redactor, redacted now that the whole text is
// known, and anything gathered inside the last window. Nothing sent them once,
// so they reached the screen only when the next round's words pushed them
// out, after the line for the reply's tool call had landed, and every sentence
// on the screen was cut a few words short and finished after the pill.
func (socket *Socket) FinishDelta(ctx context.Context) error {
	socket.deltaGuard.Lock()
	defer socket.deltaGuard.Unlock()
	reply := socket.streaming
	if reply == nil {
		return nil
	}
	socket.streaming = nil
	// The finished reply the screens are handed is trimmed, and the pieces
	// have to be the start of it, so the tail goes out without its trailing
	// blank space.
	whole := strings.TrimRight(socket.options.Secrets.Redact(reply.raw.String()), " \t\r\n")
	if !strings.HasPrefix(whole, reply.shown) {
		if err := socket.toEveryScreen(ctx, contract.SocketEnvelope{Type: contract.SocketDelta, Reset: true}); err != nil {
			return err
		}
		reply.shown, reply.gathered = "", ""
	}
	reply.gathered += whole[len(reply.shown):]
	return socket.flushGathered(ctx, reply, socket.options.Clock.Now())
}

// ResetDelta tells every attached screen to take the partial reply down, because
// the call behind it failed and is being tried again; the pieces that follow
// start the reply over.
func (socket *Socket) ResetDelta(ctx context.Context) error {
	socket.deltaGuard.Lock()
	defer socket.deltaGuard.Unlock()
	socket.streaming = nil
	return socket.toEveryScreen(ctx, contract.SocketEnvelope{Type: contract.SocketDelta, Reset: true})
}

// endStreaming forgets the reply in progress, because the finished reply that
// is about to go out carries the whole text and is the authority over the
// pieces.
func (socket *Socket) endStreaming() {
	socket.deltaGuard.Lock()
	defer socket.deltaGuard.Unlock()
	socket.streaming = nil
}

// replyInProgress returns the reply being streamed, starting one when none is.
func (socket *Socket) replyInProgress() *replyInProgress {
	if socket.streaming == nil {
		socket.streaming = &replyInProgress{}
	}
	return socket.streaming
}

// flushGathered sends what has gathered since the last flush, as one piece.
func (socket *Socket) flushGathered(ctx context.Context, reply *replyInProgress, now time.Time) error {
	if reply.gathered == "" {
		return nil
	}
	piece := reply.gathered
	reply.gathered = ""
	reply.lastFlush = now
	return socket.toEveryScreen(ctx, contract.SocketEnvelope{Type: contract.SocketDelta, Text: piece})
}

// withoutTail returns the text with its last runes taken off.
func withoutTail(text string, runes int) string {
	if utf8.RuneCountInString(text) <= runes {
		return ""
	}
	kept := text
	for taken := 0; taken < runes; taken++ {
		_, size := utf8.DecodeLastRuneInString(kept)
		kept = kept[:len(kept)-size]
	}
	return kept
}
