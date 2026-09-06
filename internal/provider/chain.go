package provider

import (
	"context"
	"errors"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// Chain is an ordered list of models tried one after another. When the first one
// is out of tries the next one is asked, and so on, and the last failure is what
// comes back when none of them answered.
//
// A request that was longer than the window goes straight back to the caller
// instead, because no other model can fix a context that is too big: only the
// context builder can, by making it smaller.
type Chain struct {
	models  []contract.Model
	options Options
}

// NewChain returns the chain the configuration's fallback list names, in order.
func NewChain(models []contract.Model, options Options) (*Chain, error) {
	if len(models) == 0 {
		return nil, errors.New("a fallback chain was built with no models in it, so name at least one model in config.toml")
	}
	return &Chain{models: models, options: options}, nil
}

// Name is the alias of the model the chain tries first, which is what the record
// and the cost line print.
func (chain *Chain) Name() string { return chain.models[0].Name() }

// ContextLength is the window of the model the chain tries first, which is what
// the working context is sized against.
func (chain *Chain) ContextLength() int { return chain.models[0].ContextLength() }

// Send tries each model in turn until one answers. The caller sees each
// model's words as they arrive, and a reset before the next model's first word
// when the last one had streamed text, so a screen can withdraw what it drew.
func (chain *Chain) Send(ctx context.Context, request contract.Request,
	onDelta func(delta string)) (contract.Reply, error) {
	gate := newDeltaGate(onDelta, chain.options.OnReset)
	lastError := error(nil)
	for at, model := range chain.models {
		reply, err := model.Send(ctx, request, gate.forAttempt())
		if err == nil {
			// The reply names the model that answered, which is the whole point
			// of a chain: the caller asked for the first one and may have been
			// answered by the third.
			reply.Model = model.Name()
			return reply, nil
		}
		if errors.Is(err, contract.ErrContextOverflow) {
			return contract.Reply{}, err
		}
		// A call cancelled by the caller, a stop or a shutdown, is nobody's
		// failure, so no other model is asked and nothing is written down.
		if ctx.Err() != nil {
			return contract.Reply{}, err
		}
		lastError = err
		chain.noteFallback(at, err)
	}
	return contract.Reply{}, lastError
}

// noteFallback writes down that one model was given up on, naming the one that
// is being tried next when there is one.
func (chain *Chain) noteFallback(at int, err error) {
	failed := chain.models[at].Name()
	if at+1 < len(chain.models) {
		chain.options.note("the model %q is out of tries, so moving on to the model %q: %v",
			failed, chain.models[at+1].Name(), err)
		return
	}
	chain.options.note("the model %q is out of tries and it was the last one in the chain: %v", failed, err)
}
