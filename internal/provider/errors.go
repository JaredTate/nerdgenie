// The list of phrases that mean the prompt was longer than the model's window
// was read from Prime Agent's overflow detection at
// ~/Code/prime-agent/packages/ai/src/utils/overflow.ts. That file keeps a phrase
// for every provider its author ever met; this keeps only what the three servers
// Coeus talks to actually say, plus the one generic code, because a phrase list
// nobody can test is a list nobody can trust.

package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// overflowPhrases are the ways the three servers Coeus talks to say that the
// prompt was longer than the model can hold. They are matched against a refusal
// body that already came back with a 400 or a 413, so a phrase cannot be
// mistaken for anything else.
var overflowPhrases = []string{
	"prompt is too long",
	"request_too_large",
	"context_length_exceeded",
	"exceeds the context window",
	"exceeds the available context size",
	"reduce the length of the messages",
	"maximum context length",
}

// retryableStatuses are the refusals that a later attempt could get past,
// because they are the server's problem rather than the request's.
var retryableStatuses = []int{
	http.StatusInternalServerError,
	http.StatusBadGateway,
	http.StatusServiceUnavailable,
	http.StatusGatewayTimeout,
	529,
}

// defaultRetryAfter is how long to wait when a server says it is rate limiting
// but does not say for how long.
const defaultRetryAfter = 60 * time.Second

// providerError is a refusal from a provider, carrying what the retry wrapper
// needs to decide: which model it was, what the server said, and whether trying
// again could help.
type providerError struct {
	// modelName is the alias the user gave the model, so that an error printed
	// to the user names something the user recognises.
	modelName string
	// status is the HTTP status the server answered with, or zero when the
	// failure happened before a status arrived.
	status int
	// message is what the server said, already cut to a readable length.
	message string
	// retryable says whether the same request could succeed on a later attempt.
	retryable bool
}

// Error says which model refused, what it said, and what the status was.
func (failure providerError) Error() string {
	if failure.status == 0 {
		return fmt.Sprintf("the model %q could not be reached: %s", failure.modelName, failure.message)
	}
	return fmt.Sprintf("the model %q refused the call with status %d: %s", failure.modelName, failure.status, failure.message)
}

// failureFromStatus turns a refusal into the error the harness knows how to act
// on: the overflow sentinel when the prompt was too long, the rate-limit
// sentinel when the server said to wait, and otherwise a plain failure that says
// whether trying again could help.
func failureFromStatus(modelName string, answer *http.Response, body []byte, now time.Time) error {
	said := messageFromBody(body)
	switch {
	case answer.StatusCode == http.StatusRequestEntityTooLarge,
		answer.StatusCode == http.StatusBadRequest && looksLikeOverflow(said):
		return fmt.Errorf("the model %q refused the call because the prompt was too long, and it said %q: %w",
			modelName, said, contract.ErrContextOverflow)
	case answer.StatusCode == http.StatusTooManyRequests:
		return fmt.Errorf("the model %q is rate limiting this account: %w",
			modelName, contract.RateLimitedError{RetryAfter: retryAfterFrom(answer.Header, now)})
	default:
		return providerError{
			modelName: modelName,
			status:    answer.StatusCode,
			message:   said,
			retryable: isRetryableStatus(answer.StatusCode),
		}
	}
}

// isRetryableStatus says whether a status is one a later attempt could get past.
func isRetryableStatus(status int) bool {
	for _, retryable := range retryableStatuses {
		if status == retryable {
			return true
		}
	}
	return false
}

// looksLikeOverflow says whether a refusal's text is one of the ways the servers
// Coeus talks to report a prompt longer than the window.
func looksLikeOverflow(said string) bool {
	lowered := strings.ToLower(said)
	for _, phrase := range overflowPhrases {
		if strings.Contains(lowered, phrase) {
			return true
		}
	}
	return false
}

// messageFromBody digs the sentence out of a refusal body, which both APIs wrap
// in an "error" object, and falls back to the body itself when it is neither.
// The cap is put on the sentence that is handed on and not only on the bytes it
// was read from, because reading grows them: a byte that is not valid UTF-8
// comes back out of the JSON reader as the three-byte replacement character, so
// a body that was under the cap can decode into a message three times as long.
func messageFromBody(body []byte) string {
	if len(body) > maxErrorBodyBytes {
		body = body[:maxErrorBodyBytes]
	}
	return cutToBytes(sentenceFromBody(body), maxErrorBodyBytes)
}

// cutToBytes shortens a message to at most the given number of bytes, cutting
// between characters so that a character is never left half written.
func cutToBytes(said string, limit int) string {
	if len(said) <= limit {
		return said
	}
	cut := 0
	for boundary := range said {
		if boundary > limit {
			break
		}
		cut = boundary
	}
	return said[:cut]
}

// sentenceFromBody reads the refusal's sentence out of a body, leaving how long
// that sentence may be to messageFromBody, which is the one place that caps it.
func sentenceFromBody(body []byte) string {
	shaped := struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
		Message string `json:"message"`
	}{}
	if err := json.Unmarshal(body, &shaped); err == nil {
		joined := strings.TrimSpace(shaped.Error.Message + " " + shaped.Error.Type + " " + shaped.Error.Code)
		if joined != "" {
			return joined
		}
		if shaped.Message != "" {
			return shaped.Message
		}
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "the server sent no message with its refusal"
	}
	return trimmed
}

// retryAfterFrom reads the Retry-After header, which a server writes either as a
// number of seconds or as the moment to try again.
func retryAfterFrom(header http.Header, now time.Time) time.Duration {
	said := strings.TrimSpace(header.Get("Retry-After"))
	if said == "" {
		return defaultRetryAfter
	}
	if seconds, err := strconv.ParseFloat(said, 64); err == nil && seconds >= 0 {
		return time.Duration(seconds * float64(time.Second))
	}
	if moment, err := http.ParseTime(said); err == nil {
		if wait := moment.Sub(now); wait > 0 {
			return wait
		}
		return 0
	}
	return defaultRetryAfter
}
