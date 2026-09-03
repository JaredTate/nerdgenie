package provider

import "strings"

// maxHeldDeltaPieces is how many pieces of one attempt's text are kept apart
// from one another. Past that they are joined into one piece, so that the memory
// a gate uses follows the size of the text rather than the number of pieces a
// server chose to send it in.
const maxHeldDeltaPieces = 4096

// deltaGate hands the caller the pieces of text one attempt streamed, and hands
// them on only once that attempt has succeeded.
//
// The retry wrapper and the fallback chain both make more than one attempt at
// the same request, and a stream can fail after it has already sent some of the
// model's words. Passing those words straight on would leave a caller that had
// seen "Hello " from an attempt which was thrown away and then "Hello world"
// from the attempt that worked, while the reply itself says only "Hello world":
// the deltas and the reply would no longer be the same text. So an attempt's
// pieces wait here until that attempt is known to be the one whose reply is
// going back to the caller. The cost is that the words arrive when the attempt
// finishes rather than as they are written, and it is the price of never showing
// anyone text that was discarded.
type deltaGate struct {
	// onDelta is the caller's own delta function, or nil when the caller asked
	// for no deltas at all.
	onDelta func(delta string)
	// held is what the attempt in progress has streamed so far.
	held []string
}

// newDeltaGate returns the gate that one call's attempts stream through.
func newDeltaGate(onDelta func(delta string)) *deltaGate {
	return &deltaGate{onDelta: onDelta}
}

// forAttempt returns the delta function to hand to one attempt, forgetting
// whatever an earlier attempt streamed. It is nil when the caller wants no
// deltas, so that a provider gathers no text nobody asked for.
func (gate *deltaGate) forAttempt() func(delta string) {
	gate.held = nil
	if gate.onDelta == nil {
		return nil
	}
	return gate.hold
}

// hold keeps one piece of the text the attempt in progress is streaming.
func (gate *deltaGate) hold(delta string) {
	if len(gate.held) >= maxHeldDeltaPieces {
		gate.held = []string{strings.Join(gate.held, "")}
	}
	gate.held = append(gate.held, delta)
}

// deliver hands the caller everything the attempt that has just succeeded
// streamed, in the order it arrived.
func (gate *deltaGate) deliver() {
	for _, piece := range gate.held {
		gate.onDelta(piece)
	}
	gate.held = nil
}
