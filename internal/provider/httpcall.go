package provider

import (
	"bytes"
	"context"
	"io"
	"net/http"
)

// openStream is one answer whose body is still arriving: the reader to walk, the
// watch that will cancel it if it goes quiet, and the one function that ends it.
type openStream struct {
	// reader is the answer's body, with the stall watch listening to it.
	reader io.Reader
	// watch says afterwards whether the call was cut short for going quiet.
	watch *stallWatch
	// finish closes the body and releases the call's context.
	finish func()
}

// openStreamedCall sends one request and returns its answer's body, already
// under the stall watch. A refusal is turned into the error the harness acts on
// before the body is handed back, so a caller only ever reads a good stream.
func openStreamedCall(ctx context.Context, options Options, modelName, address string,
	header http.Header, body []byte) (*openStream, error) {
	ctx, releaseDeadline := withCallDeadline(ctx)
	callCtx, giveUp := context.WithCancel(ctx)
	watch := &stallWatch{}
	go watch.watchFor(callCtx, options.Clock, giveUp)

	stop := func() {
		giveUp()
		releaseDeadline()
	}

	request, err := http.NewRequestWithContext(callCtx, http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		stop()
		return nil, providerError{modelName: modelName, message: err.Error()}
	}
	for name, values := range header {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}

	answer, err := options.client().Do(request)
	if err != nil {
		stop()
		return nil, watch.explain(modelName, err)
	}
	if answer.StatusCode != http.StatusOK {
		refusal := readRefusal(answer)
		answer.Body.Close()
		stop()
		return nil, failureFromStatus(modelName, answer, refusal, options.Clock.Now())
	}
	return &openStream{
		reader: watchingReader{inner: answer.Body, watch: watch},
		watch:  watch,
		finish: func() { answer.Body.Close(); stop() },
	}, nil
}

// withCallDeadline gives the call a deadline of its own when the caller brought
// none, so that no call can hang for ever.
func withCallDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, has := ctx.Deadline(); has {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, callTimeout())
}

// readRefusal reads as much of a refusal's body as is worth printing.
func readRefusal(answer *http.Response) []byte {
	body, err := io.ReadAll(io.LimitReader(answer.Body, maxErrorBodyBytes))
	if err != nil {
		return nil
	}
	return body
}
