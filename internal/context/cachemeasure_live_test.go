//go:build live

package context

import (
	stdcontext "context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/clock"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/provider"
)

// This file is the measurement of the tail layout against the real daemon on
// card B. It builds two rounds of the forty-step fixture that follow each other,
// sends each pair to the daemon in the old order and in the new one, and reads
// how much of the second prompt the daemon took from its cache. It is a worker's
// measuring stick rather than part of the live suite: the suite talks to the
// daemon on port 19091, and this one to card B on 19093 so that it can run while
// another worker is using the first.
//
// The daemon holds one prompt at a time and reuses what a new prompt shares with
// the last one from the first token, so a pair of calls one after the other
// measures exactly what the layout is for. The pair is sent twice over, and the
// same prompt is sent twice first of all as a control: a control that does not
// reuse nearly everything means the daemon is not measuring what this file
// thinks it is.

// theSecondCard is the daemon this measurement talks to.
const theSecondCard = "http://127.0.0.1:19093"

// TestWhatTheDaemonReadsInEachOrder prints the daemon's own numbers for the two
// layouts, one after the other, so that the difference is measured rather than
// argued.
func TestWhatTheDaemonReadsInEachOrder(t *testing.T) {
	run := newFixtureRun(t)
	builder := newTestBuilder(t, Options{Home: liveHome(t), MaxOutputTokens: 32, Boundary: goldenBoundary})
	run.playTo(t, 20)
	earlier, err := builder.Build(t.Context(), run.input(262144))
	if err != nil {
		t.Fatalf("cannot build round twenty: %v", err)
	}
	run.playTo(t, 21)
	later, err := builder.Build(t.Context(), run.input(262144))
	if err != nil {
		t.Fatalf("cannot build round twenty-one: %v", err)
	}

	measure(t, "the control, the very same prompt twice", later, later)
	measure(t, "the old order, the record's body above the conversation", inTheOldOrder(earlier), inTheOldOrder(later))
	measure(t, "the new order, everything a turn writes anew in the tail", earlier, later)
}

// measure sends one pair of calls one after the other and prints what the second
// of them read and how much of it the daemon already had.
func measure(t *testing.T, name string, earlier contract.Request, later contract.Request) {
	t.Helper()
	send(t, name+", the first call", earlier)
	second := send(t, name+", the second call", later)
	t.Logf("%s: the second call read %d tokens, %d of them from the cache, which is %.1f per cent, in %s",
		name, second.read, second.cached, 100*float64(second.cached)/float64(second.read), second.took)
}

// asItIs is the shape of a request the builder already puts it in.
func asItIs(request contract.Request) contract.Request { return request }

// inTheOldOrder puts a built request back into the order this package used
// before the tail was widened: the record's body first, then the pins, the
// conversation, what is known, the hint, the list of results and the header.
func inTheOldOrder(request contract.Request) contract.Request {
	moved := []contract.Message{}
	for _, heading := range []string{recordSecondHalfHeading, pinnedHeading} {
		moved = append(moved, blocksOf(request, heading)...)
	}
	moved = append(moved, blocksOf(request, "")...)
	for _, heading := range []string{whatIsKnownHeading, memoryHintHeading, recordResultsHeading, recordHeaderHeading} {
		moved = append(moved, blocksOf(request, heading)...)
	}
	return contract.Request{
		SystemBlocks: request.SystemBlocks, Tools: request.Tools,
		MaxOutputTokens: request.MaxOutputTokens, Messages: moved,
	}
}

// blocksOf is every message opening with one heading, or, for an empty heading,
// every message of the conversation itself.
func blocksOf(request contract.Request, heading string) []contract.Message {
	found := []contract.Message{}
	for _, message := range request.Messages {
		if headOf(message) == heading {
			found = append(found, message)
		}
	}
	return found
}

// headOf is the heading a message opens with, or nothing when the message is
// part of the conversation rather than a block this package wrote.
func headOf(message contract.Message) string {
	for _, heading := range []string{
		recordSecondHalfHeading, recordResultsHeading, recordHeaderHeading,
		whatIsKnownHeading, pinnedHeading, memoryHintHeading,
	} {
		if len(message.Text) >= len(heading) && message.Text[:len(heading)] == heading {
			return heading
		}
	}
	return ""
}

// oneCall is what the daemon said about one real call.
type oneCall struct {
	read   int
	cached int
	took   time.Duration
	calls  int
}

// send makes one real call to card B and reads what the daemon said about it.
func send(t *testing.T, name string, request contract.Request) oneCall {
	t.Helper()
	alias := contract.ModelAlias{
		Name: contract.LocalModelAlias, Provider: contract.ProviderOpenAI,
		BaseAddress: theSecondCard + "/v1", ModelName: "local-coder", ContextLength: 262144,
	}
	model, err := provider.New(alias, provider.Options{Clock: clock.System(), Home: liveHome(t)})
	if err != nil {
		t.Fatalf("the daemon on card B could not be set up: %v", err)
	}
	ctx, giveUp := stdcontext.WithTimeout(stdcontext.Background(), 5*time.Minute)
	defer giveUp()
	started := time.Now()
	reply, err := model.Send(ctx, request, nil)
	if err != nil {
		t.Fatalf("the daemon on card B refused %s: %v", name, err)
	}
	made := oneCall{
		read: reply.Usage.InputTokens, cached: reply.Usage.CachedInputTokens,
		took: time.Since(started).Round(time.Second), calls: 1,
	}
	t.Logf("%s: %d tokens read, %d from the cache, %d written, in %s",
		name, made.read, made.cached, reply.Usage.OutputTokens, made.took)
	return made
}

// taskCounter is the number of the last task the daemon took, which counts every
// call anybody made to it. Two of these around a pair say whether the pair had
// the daemon to itself.
func taskCounter(t *testing.T) int {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	answer, err := client.Get(theSecondCard + "/slots")
	if err != nil {
		t.Fatalf("cannot read the daemon's slots: %v", err)
	}
	defer answer.Body.Close()
	slots := []struct {
		TaskID int `json:"id_task"`
	}{}
	if err := json.NewDecoder(answer.Body).Decode(&slots); err != nil {
		t.Fatalf("cannot read the daemon's slots: %v", err)
	}
	if len(slots) == 0 {
		t.Fatal("the daemon reported no slots at all")
	}
	return slots[0].TaskID
}
