package main

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// tokensPerWord is the rough ratio the streamed count is measured by while a
// call is still running. The provider only says what a call really cost once it
// has finished, and a person watching a model think wants a number that moves
// rather than a number that is exactly right.
const tokensPerWord = 4

// watchedModel is the model with a note taken of every call: when it began, how
// much it has written so far, what it held, and what the session has spent. It
// is what fills the header of the terminal screen, which the first human trial
// found empty.
//
// It sits between the loop and the provider chain rather than inside either,
// because the loop must not know about screens and the provider must not know
// about sessions.
type watchedModel struct {
	// under is the real model: the chain with its retries.
	under contract.Model
	// clock is where the moment a call began is read.
	clock contract.Clock
	// changed is called whenever the numbers move, so that a screen is told at
	// once rather than at the next heartbeat.
	changed func()

	guard      sync.Mutex
	callBegan  time.Time
	streamed   int
	lastHeld   int
	tokensIn   int
	cachedIn   int
	tokensOut  int
	moneySoFar float64
}

// newWatchedModel wraps one model so that what its calls cost can be seen.
func newWatchedModel(under contract.Model, clock contract.Clock, changed func()) *watchedModel {
	return &watchedModel{under: under, clock: clock, changed: changed}
}

// Name is the alias of the model in use.
func (watched *watchedModel) Name() string { return watched.model().Name() }

// ContextLength is how many tokens the model holds on one call.
func (watched *watchedModel) ContextLength() int { return watched.model().ContextLength() }

// use points this at another chain, which is what "/model" does. The session's
// own counts are kept, because they are the session's and not the model's.
func (watched *watchedModel) use(under contract.Model) {
	watched.guard.Lock()
	defer watched.guard.Unlock()
	watched.under = under
}

// model is the chain in use right now.
func (watched *watchedModel) model() contract.Model {
	watched.guard.Lock()
	defer watched.guard.Unlock()
	return watched.under
}

// costSoFar is what this session has spent, which "/status" prints.
func (watched *watchedModel) costSoFar() contract.CostLine {
	watched.guard.Lock()
	defer watched.guard.Unlock()
	return contract.CostLine{
		InputTokens:       watched.tokensIn,
		CachedInputTokens: watched.cachedIn,
		OutputTokens:      watched.tokensOut,
	}
}

// Send makes the call, counting the moment it began and what it wrote, and adds
// what it cost to the session once it has answered.
func (watched *watchedModel) Send(ctx context.Context, request contract.Request,
	onDelta func(delta string)) (contract.Reply, error) {
	watched.callBegins()
	defer watched.callEnds()

	reply, err := watched.model().Send(ctx, request, watched.counting(onDelta))
	if err == nil {
		watched.callCost(reply.Usage)
	}
	return reply, err
}

// counting wraps the caller's own delta function so that what has been written
// so far is counted whether or not anybody wanted the words.
func (watched *watchedModel) counting(onDelta func(delta string)) func(delta string) {
	return func(delta string) {
		watched.guard.Lock()
		watched.streamed += len(delta) / tokensPerWord
		watched.guard.Unlock()
		if onDelta != nil {
			onDelta(delta)
		}
	}
}

// callBegins writes down the moment this call started.
func (watched *watchedModel) callBegins() {
	watched.guard.Lock()
	watched.callBegan = watched.clock.Now()
	watched.streamed = 0
	watched.guard.Unlock()
	watched.tellSomebody()
}

// callEnds forgets the call in flight, whether it answered or not.
func (watched *watchedModel) callEnds() {
	watched.guard.Lock()
	watched.callBegan = time.Time{}
	watched.guard.Unlock()
	watched.tellSomebody()
}

// callCost adds what one call cost to the session, and remembers how much of the
// window that call held.
func (watched *watchedModel) callCost(spent contract.Usage) {
	watched.guard.Lock()
	watched.lastHeld = spent.InputTokens
	watched.tokensIn += spent.InputTokens
	watched.cachedIn += spent.CachedInputTokens
	watched.tokensOut += spent.OutputTokens
	watched.moneySoFar += spent.CostUSD
	watched.guard.Unlock()
}

// tellSomebody says the numbers moved, so that a screen is told at once.
func (watched *watchedModel) tellSomebody() {
	if watched.changed != nil {
		watched.changed()
	}
}

// fillStatus writes what is known about the model into the status a screen
// reads: the window, the context the last call held, what the session has spent,
// and, while a call is in flight, when it began and how much it has written.
func (watched *watchedModel) fillStatus(fields map[string]string) {
	watched.guard.Lock()
	defer watched.guard.Unlock()

	fields[contract.StatusFieldContextWindow] = strconv.Itoa(watched.under.ContextLength())
	if watched.lastHeld > 0 {
		fields[contract.StatusFieldContextTokens] = strconv.Itoa(watched.lastHeld)
	}
	fields[contract.StatusFieldTokensIn] = strconv.Itoa(watched.tokensIn)
	fields[contract.StatusFieldTokensOut] = strconv.Itoa(watched.tokensOut)
	if watched.moneySoFar > 0 {
		fields[contract.StatusFieldCost] = strconv.FormatFloat(watched.moneySoFar, 'f', 4, 64)
	}
	if !watched.callBegan.IsZero() {
		fields[contract.StatusFieldCallStarted] = watched.callBegan.UTC().Format(time.RFC3339)
		fields[contract.StatusFieldStreamed] = strconv.Itoa(watched.streamed)
	}
}

// calling says whether a model call is in flight, which is what the state word
// on the strip is decided from.
func (watched *watchedModel) calling() bool {
	watched.guard.Lock()
	defer watched.guard.Unlock()
	return !watched.callBegan.IsZero()
}
