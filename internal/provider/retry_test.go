package provider_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/provider"
	"github.com/JaredTate/coeus/internal/testkit"
)

// callResult is what one background call produced, so that a test can move the
// clock while the call is waiting on it.
type callResult struct {
	reply contract.Reply
	err   error
}

// sendInBackground starts one call and hands back the channel its result will
// arrive on.
func sendInBackground(model contract.Model, request contract.Request) <-chan callResult {
	done := make(chan callResult, 1)
	go func() {
		reply, err := model.Send(context.Background(), request, nil)
		done <- callResult{reply: reply, err: err}
	}()
	return done
}

// sendInBackgroundCollecting starts one call, keeping every delta it streams as
// it arrives, and hands back the channel its result will arrive on. The text it
// gathers is only safe to read once that result has come back.
func sendInBackgroundCollecting(model contract.Model, request contract.Request,
	streamed *strings.Builder) <-chan callResult {
	done := make(chan callResult, 1)
	go func() {
		reply, err := model.Send(context.Background(), request, func(delta string) { streamed.WriteString(delta) })
		done <- callResult{reply: reply, err: err}
	}()
	return done
}

// waitForResult reads the result of a background call, failing the test if it
// never arrives.
func waitForResult(t *testing.T, done <-chan callResult) callResult {
	t.Helper()
	select {
	case finished := <-done:
		return finished
	case <-time.After(10 * time.Second):
		t.Fatal("the call never finished, and every wait in the provider is measured on the fake clock")
		return callResult{}
	}
}

// retryingAgainst builds the OpenAI-compatible provider with the retry wrapper
// around it, and hands back the clock and the log the test drives it with.
func retryingAgainst(t *testing.T, server *testkit.FakeProviderServer) (contract.Model, *waitingClock, *noteRecorder) {
	t.Helper()
	clock := newTestClock()
	options, recorder := testOptions(t, clock)
	base, err := provider.New(localAliasAt(server.Address(), 262144), options)
	if err != nil {
		t.Fatalf("building the provider failed: %v", err)
	}
	return provider.WithRetries(base, options), clock, recorder
}

func TestAServerErrorIsTriedAgainAndTheSecondAttemptSucceeds(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("second time lucky"))
	defer server.Close()
	model, clock, recorder := retryingAgainst(t, server)
	server.MisbehaveNext(testkit.FailTheCall, 0)

	done := sendInBackground(model, requestWithEverything())
	waitForNotes(t, recorder, 1)
	waitForSleeper(t, clock, 1)
	clock.Advance(30 * time.Second)
	finished := waitForResult(t, done)

	if finished.err != nil {
		t.Fatalf("a call that failed once and then worked came back as %v", finished.err)
	}
	if finished.reply.Text != "second time lucky" {
		t.Errorf("the reply is %q, want the second attempt's answer", finished.reply.Text)
	}
	if recorder.count() != 1 || !strings.Contains(recorder.all()[0], "attempt 1") {
		t.Errorf("the retry was not written down as one line saying which attempt failed: %v", recorder.all())
	}
}

func TestARateLimitWaitsForExactlyAsLongAsTheServerAsked(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("after the wait"))
	defer server.Close()
	model, clock, recorder := retryingAgainst(t, server)
	server.MisbehaveNext(testkit.RateLimitTheCall, 17*time.Second)

	done := sendInBackground(model, requestWithEverything())
	waitForNotes(t, recorder, 1)
	waitForSleeper(t, clock, 1)
	clock.Advance(17*time.Second - time.Nanosecond)
	if clock.Sleepers() != 1 {
		t.Fatal("the retry stopped waiting a moment early, and the header said seventeen seconds")
	}
	clock.Advance(time.Nanosecond)
	finished := waitForResult(t, done)

	if finished.err != nil {
		t.Fatalf("a rate-limited call that waited and then worked came back as %v", finished.err)
	}
	if finished.reply.Text != "after the wait" {
		t.Errorf("the reply is %q, want the second attempt's answer", finished.reply.Text)
	}
}

func TestAStalledStreamIsGivenUpOnAndTriedAgain(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("the stream came back"))
	defer server.Close()
	model, clock, recorder := retryingAgainst(t, server)
	server.MisbehaveNext(testkit.StallTheStream, time.Minute)

	done := sendInBackground(model, requestWithEverything())
	waitForSleeper(t, clock, 1)
	clock.Advance(90 * time.Second)
	waitForNotes(t, recorder, 1)
	waitForSleeper(t, clock, 1)
	clock.Advance(2 * time.Second)
	finished := waitForResult(t, done)

	if finished.err != nil {
		t.Fatalf("a stalled call that was tried again came back as %v", finished.err)
	}
	if finished.reply.Text != "the stream came back" {
		t.Errorf("the reply is %q, want the second attempt's answer", finished.reply.Text)
	}
	if !strings.Contains(recorder.all()[0], "nothing") {
		t.Errorf("the line about the stall does not say what went wrong: %v", recorder.all())
	}
}

func TestAnOverflowIsNeverTriedAgain(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("never reached"))
	defer server.Close()
	model, _, recorder := retryingAgainst(t, server)
	server.MisbehaveNext(testkit.OverflowTheContext, 0)

	_, err := model.Send(context.Background(), requestWithEverything(), nil)

	if !errors.Is(err, contract.ErrContextOverflow) {
		t.Fatalf("a prompt that was too long came back as %v, want the overflow sentinel", err)
	}
	if recorder.count() != 0 {
		t.Errorf("an overflow was retried, and a retry would fail the same way: %v", recorder.all())
	}
	if calls := callsTo(server, testkit.OpenAIPath); calls != 1 {
		t.Errorf("the model was called %d times, and an overflow is never sent twice", calls)
	}
}

func TestThreeAttemptsIsAllTheRetriesThereAre(t *testing.T) {
	server := alwaysFailingServer()
	defer server.Close()
	clock := newTestClock()
	options, recorder := testOptions(t, clock)
	base, err := provider.New(localAliasAt(server.URL, 262144), options)
	if err != nil {
		t.Fatalf("building the provider failed: %v", err)
	}
	model := provider.WithRetries(base, options)

	done := sendInBackground(model, requestWithEverything())
	for note := 1; note <= 2; note++ {
		waitForNotes(t, recorder, note)
		waitForSleeper(t, clock, 1)
		clock.Advance(30 * time.Second)
	}
	finished := waitForResult(t, done)

	if finished.err == nil {
		t.Fatal("a call that failed every time came back as a good reply")
	}
	if recorder.count() != 2 {
		t.Errorf("there were %d waits, and three attempts means two of them: %v", recorder.count(), recorder.all())
	}
	if !strings.Contains(finished.err.Error(), contract.LocalModelAlias) {
		t.Errorf("the last error does not name the model: %v", finished.err)
	}
}

// alwaysFailingServer answers every call with a server error, which is how a
// test reaches the end of the attempts without racing the fake provider's
// one-call-at-a-time misbehaviour.
func alwaysFailingServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, `{"error":{"message":"the provider had an internal error"}}`, http.StatusBadGateway)
	}))
}

func TestARefusalThatIsNotWorthRetryingComesStraightBack(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("never reached"))
	defer server.Close()
	options, recorder := testOptions(t, newTestClock())
	base, err := provider.New(localAliasAt(server.Address()+"/nowhere", 4096), options)
	if err != nil {
		t.Fatalf("building the provider failed: %v", err)
	}
	model := provider.WithRetries(base, options)

	_, err = model.Send(context.Background(), requestWithEverything(), nil)

	if err == nil {
		t.Fatal("a call to an address the server does not serve came back as a good reply")
	}
	if recorder.count() != 0 {
		t.Errorf("a refusal that a second attempt cannot fix was retried anyway: %v", recorder.all())
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("the error does not say what the server answered: %v", err)
	}
}

// serverThatBreaksOffAfterSaying answers the Chat Completions path with a stream
// of its own. Its first call streams the first text and then stops without ever
// saying why it stopped, which is what a connection that broke half way through
// an answer looks like; every call after that one streams the whole text
// properly. It is how a test makes a model fail after the caller has already
// been given some of the model's words.
func serverThatBreaksOffAfterSaying(first, whole string) *httptest.Server {
	calls := &atomic.Int64{}
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != testkit.OpenAIPath {
			http.Error(writer, "this server answers only "+testkit.OpenAIPath, http.StatusNotFound)
			return
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.WriteHeader(http.StatusOK)
		if calls.Add(1) == 1 {
			fmt.Fprint(writer, openAITextChunk(first))
			return
		}
		fmt.Fprint(writer, openAITextChunk(whole))
		fmt.Fprint(writer, `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n")
		fmt.Fprint(writer, "data: [DONE]\n\n")
	}))
}

// openAITextChunk is one line of a Chat Completions stream carrying a piece of
// the answer's text.
func openAITextChunk(text string) string {
	return fmt.Sprintf(`data: {"choices":[{"index":0,"delta":{"content":%q}}]}`+"\n\n", text)
}

func TestARetryNeverReplaysTheTextTheFailedAttemptStreamed(t *testing.T) {
	server := serverThatBreaksOffAfterSaying("Hello ", "Hello world")
	defer server.Close()
	clock := newTestClock()
	options, recorder := testOptions(t, clock)
	base, err := provider.New(localAliasAt(server.URL, 262144), options)
	if err != nil {
		t.Fatalf("building the provider failed: %v", err)
	}
	model := provider.WithRetries(base, options)

	streamed := &strings.Builder{}
	done := sendInBackgroundCollecting(model, requestWithEverything(), streamed)
	waitForNotes(t, recorder, 1)
	waitForSleeper(t, clock, 1)
	clock.Advance(30 * time.Second)
	finished := waitForResult(t, done)

	if finished.err != nil {
		t.Fatalf("a call whose stream broke and was then tried again came back as %v", finished.err)
	}
	if finished.reply.Text != "Hello world" {
		t.Fatalf("the reply is %q, want the whole answer the second attempt streamed", finished.reply.Text)
	}
	if streamed.String() != finished.reply.Text {
		t.Errorf("the caller was streamed %q while the reply is %q, and a caller must never be handed the text of an attempt that was thrown away",
			streamed.String(), finished.reply.Text)
	}
}

func TestTheRetryWrapperKeepsTheModelsNameAndWindow(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("done"))
	defer server.Close()
	model, _, _ := retryingAgainst(t, server)

	if model.Name() != contract.LocalModelAlias {
		t.Errorf("the wrapped model calls itself %q, want the alias underneath", model.Name())
	}
	if model.ContextLength() != 262144 {
		t.Errorf("the wrapped model reports a window of %d, want the one underneath", model.ContextLength())
	}
}
