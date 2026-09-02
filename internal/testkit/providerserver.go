// The two wire protocols this server speaks were read from Prime's provider
// files at ~/Code/prime-agent/packages/ai/src/providers/anthropic.ts and
// ~/Code/prime-agent/packages/ai/src/providers/openai-completions.ts, which is
// where the event names and the usage fields come from. Nothing was copied: that
// code is TypeScript and reads these streams, and this writes them in Go.

package testkit

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// The two paths the fake provider serves.
const (
	// AnthropicPath is where the Anthropic Messages API lives.
	AnthropicPath = "/v1/messages"
	// OpenAIPath is where the OpenAI Chat Completions API lives.
	OpenAIPath = "/v1/chat/completions"
)

// Misbehaviour is one way the fake provider can go wrong on purpose, so that a
// test can prove the harness handles it.
type Misbehaviour string

const (
	// BehaveWell answers properly, which is what the server does by default.
	BehaveWell Misbehaviour = ""
	// StallTheStream sends nothing at all until the wait has passed.
	StallTheStream Misbehaviour = "stall the stream"
	// RateLimitTheCall answers 429 with a Retry-After header.
	RateLimitTheCall Misbehaviour = "rate limit the call"
	// FailTheCall answers 500.
	FailTheCall Misbehaviour = "fail the call"
	// OverflowTheContext answers the context-too-long error in each API's own
	// shape, which the provider must never retry.
	OverflowTheContext Misbehaviour = "overflow the context"
	// DropTheStream sends part of the stream and then stops without ending it.
	DropTheStream Misbehaviour = "drop the stream"
)

// ProviderRequest is one request the fake provider received, kept whole so that
// a test can look for the cache markers in it.
type ProviderRequest struct {
	// Path is which API was called.
	Path string
	// Header is what the caller sent.
	Header http.Header
	// Body is the request body, unread and unparsed.
	Body []byte
}

// FakeProviderServer is a loopback HTTP server that speaks both wire protocols
// from the same script, records every request, and can be told to misbehave for
// the next call.
type FakeProviderServer struct {
	server *httptest.Server

	guard       sync.Mutex
	script      Script
	played      int
	requests    []ProviderRequest
	nextProblem Misbehaviour
	problemWait time.Duration
}

// NewFakeProviderServer starts a server on a loopback port that answers from the
// script. Close it when the test is done.
func NewFakeProviderServer(script Script) *FakeProviderServer {
	provider := &FakeProviderServer{script: script}
	provider.server = httptest.NewServer(http.HandlerFunc(provider.handle))
	return provider
}

// Address is the base address of the server, without a path.
func (provider *FakeProviderServer) Address() string {
	return provider.server.URL
}

// AnthropicAddress is the full address of the Anthropic Messages API.
func (provider *FakeProviderServer) AnthropicAddress() string {
	return provider.server.URL + AnthropicPath
}

// OpenAIAddress is the full address of the OpenAI Chat Completions API.
func (provider *FakeProviderServer) OpenAIAddress() string {
	return provider.server.URL + OpenAIPath
}

// MisbehaveNext tells the server to go wrong on the next call and behave again
// afterwards. The wait is how long to stall, or how many seconds to put in the
// Retry-After header; it is ignored by the other misbehaviours.
func (provider *FakeProviderServer) MisbehaveNext(how Misbehaviour, wait time.Duration) {
	provider.guard.Lock()
	defer provider.guard.Unlock()
	provider.nextProblem = how
	provider.problemWait = wait
}

// Requests is every request the server received, in order.
func (provider *FakeProviderServer) Requests() []ProviderRequest {
	provider.guard.Lock()
	defer provider.guard.Unlock()
	copied := make([]ProviderRequest, len(provider.requests))
	copy(copied, provider.requests)
	return copied
}

// Close shuts the server down.
func (provider *FakeProviderServer) Close() {
	provider.server.Close()
}

// handle answers one call: record it, then either misbehave once or write the
// next step of the script in the shape the path asks for.
func (provider *FakeProviderServer) handle(writer http.ResponseWriter, request *http.Request) {
	body, _ := io.ReadAll(request.Body)
	problem, wait := provider.recordAndTakeProblem(request, body)

	if request.URL.Path != AnthropicPath && request.URL.Path != OpenAIPath {
		http.Error(writer, "this fake provider serves only "+AnthropicPath+" and "+OpenAIPath, http.StatusNotFound)
		return
	}

	if problem == StallTheStream {
		select {
		case <-time.After(wait):
		case <-request.Context().Done():
			return
		}
		problem = BehaveWell
	}
	if provider.answerProblem(writer, request.URL.Path, problem, wait) {
		return
	}

	step, err := provider.nextStep(WholeRequestBodyText(body))
	if err != nil {
		var lost lostExpectation
		if errors.As(err, &lost) {
			http.Error(writer, lost.Error(), http.StatusBadRequest)
			return
		}
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	provider.writeStream(writer, request.URL.Path, step, problem == DropTheStream)
}

// answerProblem writes the misbehaviour that needs no stream, and says whether it
// answered.
func (provider *FakeProviderServer) answerProblem(writer http.ResponseWriter, path string, problem Misbehaviour, wait time.Duration) bool {
	switch problem {
	case RateLimitTheCall:
		writer.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())))
		http.Error(writer, `{"error":{"message":"too many requests","type":"rate_limit_error"}}`, http.StatusTooManyRequests)
		return true
	case FailTheCall:
		http.Error(writer, `{"error":{"message":"the provider had an internal error","type":"api_error"}}`, http.StatusInternalServerError)
		return true
	case OverflowTheContext:
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(writer, overflowBody(path))
		return true
	default:
		return false
	}
}

// recordAndTakeProblem writes down the request and takes the misbehaviour that
// was set for it, leaving the server behaving well afterwards.
func (provider *FakeProviderServer) recordAndTakeProblem(request *http.Request, body []byte) (Misbehaviour, time.Duration) {
	provider.guard.Lock()
	defer provider.guard.Unlock()
	provider.requests = append(provider.requests, ProviderRequest{
		Path:   request.URL.Path,
		Header: request.Header.Clone(),
		Body:   body,
	})
	problem, wait := provider.nextProblem, provider.problemWait
	provider.nextProblem, provider.problemWait = BehaveWell, 0
	return problem, wait
}

// lostExpectation is the refusal of a request that dropped something the
// script's step said it must carry. It is its own type so that the server can
// answer it with a 400 the caller can read, rather than with the 500 an
// exhausted script gets.
type lostExpectation struct {
	// Message names the step, the script, and the text that went missing.
	Message string
}

// Error says what the request lost and what that means.
func (lost lostExpectation) Error() string {
	return lost.Message
}

// nextStep takes the next step off the script, refusing a request that lost
// something the step expected. This is the same check the fake model runs in
// model.go, so a test catches a dropped correction whether it drives the model
// in process or over the wire. A refused step stays on the script, so the next
// good request plays it.
func (provider *FakeProviderServer) nextStep(whole string) (Step, error) {
	provider.guard.Lock()
	defer provider.guard.Unlock()
	if provider.played >= len(provider.script.Steps) {
		return Step{}, fmt.Errorf("the script %q has %d steps and this is call %d, so add another step",
			provider.script.Name, len(provider.script.Steps), provider.played+1)
	}
	step := provider.script.Steps[provider.played]
	for _, wanted := range step.Expect {
		if !strings.Contains(whole, wanted) {
			return Step{}, lostExpectation{Message: fmt.Sprintf(
				"step %d of the script %q expects the request to carry %q, and it does not, so the harness lost it",
				provider.played+1, provider.script.Name, wanted)}
		}
	}
	provider.played++
	return step, nil
}

// overflowBody is the context-too-long error in the shape the API at that path
// uses, because each one says it differently and the provider must know both.
func overflowBody(path string) string {
	if path == AnthropicPath {
		return `{"type":"error","error":{"type":"invalid_request_error","message":"prompt is too long: 300000 tokens > 200000 maximum"}}`
	}
	return `{"error":{"message":"this model's maximum context length is 200000 tokens","type":"invalid_request_error","code":"context_length_exceeded"}}`
}

// finishReasonFor turns the step's finish reason into the word each API uses.
// The word comes from the finish reason itself rather than from whether the step
// asked for tools, so that all four of the contract's reasons can come off the
// wire and wave 1's mapping can be tested in both directions.
func finishReasonFor(step Step, forAnthropic bool) string {
	words := map[contract.FinishReason][2]string{
		contract.FinishEnd:       {"end_turn", "stop"},
		contract.FinishToolCalls: {"tool_use", "tool_calls"},
		contract.FinishLength:    {"max_tokens", "length"},
		contract.FinishStopped:   {"refusal", "content_filter"},
	}
	pair, known := words[finishOf(step)]
	if !known {
		pair = words[contract.FinishEnd]
	}
	if forAnthropic {
		return pair[0]
	}
	return pair[1]
}

// finishOf is the step's finish reason, filled in the way the fake model fills
// it when the script leaves it out: tool calls when the step asks for tools, and
// a finished answer otherwise.
func finishOf(step Step) contract.FinishReason {
	if step.Finish != "" {
		return step.Finish
	}
	if len(step.ToolCalls) > 0 {
		return contract.FinishToolCalls
	}
	return contract.FinishEnd
}
