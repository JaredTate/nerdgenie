package provider

// deltaGate hands the caller the pieces of text an attempt streams, as they
// arrive, and tells the caller to withdraw them when that attempt is given up
// on and another begins.
//
// The retry wrapper and the fallback chain both make more than one attempt at
// the same request, and a stream can fail after it has already sent some of the
// model's words. A screen showing those words as they arrive would otherwise
// draw "Hello " from an attempt that was thrown away and then "Hello world"
// from the one that worked. So the gate remembers whether the attempt in
// progress has streamed anything, and before the next attempt's first word it
// calls the reset, which is the caller's chance to take the partial reply off
// the screen. The words still arrive as they are written, which is what the
// design promises the person watching.
type deltaGate struct {
	// onDelta is the caller's own delta function, or nil when the caller asked
	// for no deltas at all.
	onDelta func(delta string)
	// onReset is called before a new attempt's first word when the last one
	// streamed text, or nil when nobody shows the text as it arrives.
	onReset func()
	// streamed says whether the attempt in progress has sent any text on.
	streamed bool
}

// newDeltaGate returns the gate that one call's attempts stream through.
func newDeltaGate(onDelta func(delta string), onReset func()) *deltaGate {
	return &deltaGate{onDelta: onDelta, onReset: onReset}
}

// forAttempt returns the delta function to hand to one attempt, first
// withdrawing whatever an earlier attempt streamed. It is nil when the caller
// wants no deltas, so that a provider gathers no text nobody asked for.
func (gate *deltaGate) forAttempt() func(delta string) {
	if gate.streamed && gate.onReset != nil {
		gate.onReset()
	}
	gate.streamed = false
	if gate.onDelta == nil {
		return nil
	}
	return gate.pass
}

// pass hands one piece of the attempt's text straight to the caller.
func (gate *deltaGate) pass(delta string) {
	gate.streamed = true
	gate.onDelta(delta)
}
