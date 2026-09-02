package provider_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/provider"
	"github.com/JaredTate/coeus/internal/testkit"
)

// waitingClock is a clock every test in this package can move, together with a
// list of the lines the providers logged, which is how a test proves that a
// retry or a shortened window was written down.
type waitingClock struct {
	*testkit.FakeClock
}

// newTestClock returns a fake clock reading a fixed moment, so that a retry-after
// header given as an HTTP date lands on a time the test knows.
func newTestClock() *waitingClock {
	return &waitingClock{FakeClock: testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))}
}

// waitForSleeper moves nothing until the number of callers waiting inside Sleep
// reaches the count, which is how a test knows a retry is waiting before it
// moves the clock.
func waitForSleeper(t *testing.T, clock *waitingClock, count int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if clock.Sleepers() >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("no call is waiting on the clock after five seconds, and %d were expected", count)
}

// pushClock moves the clock forward in the background until the call under test
// finishes, so that a test never has to guess how many waits there will be. It
// returns a function the test calls to stop pushing.
func pushClock(clock *waitingClock, step time.Duration) func() {
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			select {
			case <-done:
				return
			case <-time.After(time.Millisecond):
				if clock.Sleepers() > 0 {
					clock.Advance(step)
				}
			}
		}
	}()
	return func() {
		close(done)
		<-stopped
	}
}

// noteRecorder keeps the lines a provider logged. It holds a lock because a
// retry is logged from the goroutine the call is running in.
type noteRecorder struct {
	guard sync.Mutex
	lines []string
}

// add keeps one line.
func (recorder *noteRecorder) add(line string) {
	recorder.guard.Lock()
	defer recorder.guard.Unlock()
	recorder.lines = append(recorder.lines, line)
}

// all is every line kept so far.
func (recorder *noteRecorder) all() []string {
	recorder.guard.Lock()
	defer recorder.guard.Unlock()
	kept := make([]string, len(recorder.lines))
	copy(kept, recorder.lines)
	return kept
}

// count is how many lines have been kept.
func (recorder *noteRecorder) count() int {
	return len(recorder.all())
}

// waitForNotes waits until the providers have logged at least the number of
// lines given, which is how a test knows a retry has begun without guessing.
func waitForNotes(t *testing.T, recorder *noteRecorder, count int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if recorder.count() >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("only %d lines were logged after five seconds, and %d were expected: %v",
		recorder.count(), count, recorder.all())
}

// testOptions are the options every unit test in this package starts from: the
// fake clock, a temporary home for the command-line provider's scratch folders,
// and a log the test can read back.
func testOptions(t *testing.T, clock *waitingClock) (provider.Options, *noteRecorder) {
	t.Helper()
	recorder := &noteRecorder{}
	return provider.Options{
		Clock: clock,
		Home:  testkit.NewTempHome(t),
		Log:   recorder.add,
	}, recorder
}

// requestWithEverything is a request carrying all three cache boundaries, two
// tools, and a short conversation, so that one assertion can look at the whole
// shape a provider puts on the wire.
func requestWithEverything() contract.Request {
	return contract.Request{
		SystemBlocks: []contract.SystemBlock{
			{Name: "harness rules and persona", Text: "You are the reasoning engine inside Coeus.", Boundary: contract.CacheBoundaryA},
			{Name: "tools", Text: "You have two tools.", Boundary: contract.CacheBoundaryB},
			{Name: "the record's goal and rules", Text: "Goal: post the tweet.", Boundary: contract.CacheBoundaryC},
			{Name: "the record's work and lessons", Text: "Work: nothing done yet."},
		},
		Messages: []contract.Message{
			{Role: contract.RoleUser, Text: "Post the tweet about the launch."},
			{Role: contract.RoleAssistant, Text: "Reading the notes.", ToolCalls: []contract.ToolCall{
				{ID: "call_1", Name: contract.ToolRead, Input: json.RawMessage(`{"path":"notes.md"}`)},
			}},
			{Role: contract.RoleUser, ToolResults: []contract.ToolResult{
				{CallID: "call_1", Text: "The launch is on Friday."},
			}},
		},
		Tools: []contract.ToolSpec{
			{Name: contract.ToolRead, Description: "Read a file, a directory listing, or a past result by its id.", Fields: []contract.ToolField{
				{Name: "path", Type: "string", Description: "The file to read.", Required: true},
			}},
			{Name: contract.ToolWrite, Description: "Create a file or overwrite an existing one.", Fields: []contract.ToolField{
				{Name: "path", Type: "string", Description: "The file to write.", Required: true},
				{Name: "text", Type: "string", Description: "What to put in it."},
			}},
		},
		MaxOutputTokens: 2048,
	}
}

// scriptWithTwoToolCalls is the script the streaming tests play: one reply with
// text and two tool calls, whose arguments the fake server sends in pieces.
func scriptWithTwoToolCalls() testkit.Script {
	return testkit.Script{
		Name:          "fake provider",
		ContextLength: 200000,
		Steps: []testkit.Step{{
			Text: "Reading the notes, then writing the draft.",
			ToolCalls: []contract.ToolCall{
				{ID: "call_1", Name: contract.ToolRead, Input: json.RawMessage(`{"path":"notes.md"}`)},
				{ID: "call_2", Name: contract.ToolWrite, Input: json.RawMessage(`{"path":"draft.md","text":"hello"}`)},
			},
			Finish: contract.FinishToolCalls,
			Usage:  contract.Usage{InputTokens: 6100, CachedInputTokens: 5200, OutputTokens: 400},
		}},
	}
}

// scriptSayingOneThing is the shortest useful script: one plain text reply.
func scriptSayingOneThing(text string) testkit.Script {
	return testkit.Script{
		Name:          "fake provider",
		ContextLength: 200000,
		Steps: []testkit.Step{
			{Text: text, Finish: contract.FinishEnd, Usage: contract.Usage{InputTokens: 100, OutputTokens: 20}},
			{Text: text, Finish: contract.FinishEnd, Usage: contract.Usage{InputTokens: 100, OutputTokens: 20}},
			{Text: text, Finish: contract.FinishEnd, Usage: contract.Usage{InputTokens: 100, OutputTokens: 20}},
		},
	}
}

// sendAndCollect makes one call and returns the reply together with the deltas
// joined in the order they arrived.
func sendAndCollect(ctx context.Context, model contract.Model, request contract.Request) (contract.Reply, string, error) {
	streamed := strings.Builder{}
	reply, err := model.Send(ctx, request, func(delta string) { streamed.WriteString(delta) })
	return reply, streamed.String(), err
}

// bodyOfLastCallTo returns the body of the last request the fake server received
// on one of its two API paths, which is where the cache markers are looked for.
func bodyOfLastCallTo(t *testing.T, server *testkit.FakeProviderServer, path string) map[string]any {
	t.Helper()
	requests := server.Requests()
	for at := len(requests) - 1; at >= 0; at-- {
		if requests[at].Path != path {
			continue
		}
		body := map[string]any{}
		if err := json.Unmarshal(requests[at].Body, &body); err != nil {
			t.Fatalf("the request body the provider sent to %s is not JSON: %v\n%s", path, err, requests[at].Body)
		}
		return body
	}
	t.Fatalf("the provider never called %s; it called %d other addresses", path, len(requests))
	return nil
}
