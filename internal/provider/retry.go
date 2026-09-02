// The shape of the wait between attempts was read from OpenCode's retry policy
// at ~/Code/opencode/packages/opencode/src/session/retry.ts: a first delay that
// doubles, a quarter of it added as jitter so that many callers do not come back
// at the same moment, a cap when the server said nothing, and the retry-after
// header honoured ahead of the arithmetic when the server did say something.
// OpenCode reads the header in two forms and so does this; the rest is written
// fresh, and the waiting happens on the harness clock rather than on a timer.

package provider

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// The retry policy, from the brief: three attempts in all, a wait of one second
// that doubles, a quarter of it added as jitter, and a cap on both the
// arithmetic and on what a server may ask for.
const (
	// maxAttempts is how many times one call is made in all, counting the first.
	maxAttempts = 3
	// firstBackoff is the wait before the second attempt.
	firstBackoff = time.Second
	// backoffJitter is the largest fraction of a wait that is added to it.
	backoffJitter = 0.25
	// maxBackoff caps the wait this package works out for itself.
	maxBackoff = 30 * time.Second
	// maxHonouredRetryAfter caps the wait a server may ask for, so that one
	// unreasonable header cannot stop the agent for an afternoon.
	maxHonouredRetryAfter = 5 * time.Minute
)

// retryingModel tries a call again when the reason it failed is one another
// attempt could get past.
type retryingModel struct {
	inner   contract.Model
	options Options
}

// WithRetries wraps a model so that a call is tried again when trying again
// could help: a rate limit, a server error, a connection that broke, or a stream
// that went quiet. A request that was simply too long is never tried again,
// because the second attempt would fail the same way.
func WithRetries(model contract.Model, options Options) contract.Model {
	return &retryingModel{inner: model, options: options}
}

// Name is the name of the model underneath.
func (model *retryingModel) Name() string { return model.inner.Name() }

// ContextLength is the window of the model underneath.
func (model *retryingModel) ContextLength() int { return model.inner.ContextLength() }

// Send makes the call, and makes it again while there are attempts left and the
// failure is one another attempt could get past.
func (model *retryingModel) Send(ctx context.Context, request contract.Request,
	onDelta func(delta string)) (contract.Reply, error) {
	lastError := error(nil)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		reply, err := model.inner.Send(ctx, request, onDelta)
		if err == nil {
			return reply, nil
		}
		lastError = err
		wait, again := retryWait(err, attempt, rand.Float64())
		if !again || attempt == maxAttempts {
			return contract.Reply{}, err
		}
		model.options.note("the model %q failed on attempt %d of %d and is being tried again in %s: %v",
			model.inner.Name(), attempt, maxAttempts, wait.Round(time.Millisecond), err)
		if slept := model.options.Clock.Sleep(ctx, wait); slept != nil {
			return contract.Reply{}, slept
		}
	}
	return contract.Reply{}, lastError
}

// retryWait says how long to wait before the next attempt, and whether there is
// any point in making one.
func retryWait(err error, attempt int, jitter float64) (time.Duration, bool) {
	if errors.Is(err, contract.ErrContextOverflow) {
		return 0, false
	}
	limited := contract.RateLimitedError{}
	if errors.As(err, &limited) {
		return min(max(limited.RetryAfter, 0), maxHonouredRetryAfter), true
	}
	if errors.Is(err, contract.ErrStalledStream) {
		return backoffDelay(attempt, jitter), true
	}
	failure := providerError{}
	if errors.As(err, &failure) && failure.retryable {
		return backoffDelay(attempt, jitter), true
	}
	return 0, false
}

// backoffDelay is the wait before the attempt after this one: one second
// doubling each time, with up to a quarter of it added, and never more than the
// cap. The jitter is a number from zero up to one.
func backoffDelay(attempt int, jitter float64) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 16 {
		attempt = 16
	}
	if jitter < 0 || jitter > 1 || math.IsNaN(jitter) {
		jitter = 0
	}
	base := float64(firstBackoff) * math.Pow(2, float64(attempt-1))
	return min(time.Duration(base+base*backoffJitter*jitter), maxBackoff)
}
