// The two wire protocols this server speaks were read from Prime's provider
// files at ~/Code/prime-agent/packages/ai/src/providers/anthropic.ts and
// ~/Code/prime-agent/packages/ai/src/providers/openai-completions.ts, which is
// where the event names and the usage fields come from. Nothing was copied: that
// code is TypeScript and reads these streams, and this writes them in Go.

package testkit

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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

	step, err := provider.nextStep()
	if err != nil {
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

// nextStep takes the next step off the script.
func (provider *FakeProviderServer) nextStep() (Step, error) {
	provider.guard.Lock()
	defer provider.guard.Unlock()
	if provider.played >= len(provider.script.Steps) {
		return Step{}, fmt.Errorf("the script %q has %d steps and this is call %d, so add another step",
			provider.script.Name, len(provider.script.Steps), provider.played+1)
	}
	step := provider.script.Steps[provider.played]
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
func finishReasonFor(step Step, forAnthropic bool) string {
	usedTools := step.Finish == contract.FinishToolCalls || len(step.ToolCalls) > 0
	if forAnthropic {
		if usedTools {
			return "tool_use"
		}
		return "end_turn"
	}
	if usedTools {
		return "tool_calls"
	}
	return "stop"
}
