//go:build live

package provider_test

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/provider"
	"github.com/JaredTate/coeus/internal/testkit"
)

// This is the contract test for the three real models, and it never skips: a
// daemon that is down, a program that is missing, and a program that is not
// signed in are all failures, because the whole point of the live suite is to
// find out that one of them has stopped working.
//
// The two cloud models are reached through the subscriptions the user already
// pays for, by running the vendor's own program once per call. There are no API
// keys on the development machine and none are wanted, so the Anthropic API
// provider has no subtest here; it is proven against the fake provider server.

// The three models the live suite calls, as they are set up on the development
// machine.
const (
	liveLocalAddress = "http://127.0.0.1:19091/v1"
	liveLocalModel   = "local-coder"
	liveOpusModel    = "claude-opus-4-8"
	liveCodexModel   = "gpt-5.5"
)

// liveCallTimeout is how long one real call may take before the test gives up
// and says which model did not answer.
const liveCallTimeout = 5 * time.Minute

// realClock is the clock the live tests use, because a real call really does
// wait. Wave 0 ships no implementation of contract.Clock outside the fake, so
// the live suite carries the small one it needs.
type realClock struct{}

// Now is the time on this machine.
func (realClock) Now() time.Time { return time.Now() }

// Sleep waits, or comes back early when the caller gives up.
func (realClock) Sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// NewTicker starts a real ticker.
func (realClock) NewTicker(interval time.Duration) contract.Ticker {
	return &realTicker{inner: time.NewTicker(interval)}
}

// realTicker is a real ticker behind the contract's shape.
type realTicker struct {
	inner *time.Ticker
}

// Ticks is the channel the times arrive on.
func (ticking *realTicker) Ticks() <-chan time.Time { return ticking.inner.C }

// Stop ends the ticker.
func (ticking *realTicker) Stop() { ticking.inner.Stop() }

// liveOptions are the options the live suite calls with: a real clock, the real
// home folder layout under a temporary directory so that no scratch folder lands
// in the user's own home, and a log the test prints.
func liveOptions(t *testing.T) (provider.Options, *noteRecorder) {
	t.Helper()
	recorder := &noteRecorder{}
	return provider.Options{
		Clock: realClock{},
		Home:  testkit.NewTempHome(t),
		Log:   recorder.add,
	}, recorder
}

// oneToolRequest is the short request every live subtest sends: a plain question
// and one tool, so that both halves of a model's job are asked for at once.
func oneToolRequest(ask string) contract.Request {
	return contract.Request{
		SystemBlocks: []contract.SystemBlock{{
			Name:     "harness rules and persona",
			Text:     "You are the reasoning engine inside Coeus. Answer briefly and plainly.",
			Boundary: contract.CacheBoundaryA,
		}},
		Messages: []contract.Message{{Role: contract.RoleUser, Text: ask}},
		Tools: []contract.ToolSpec{{
			Name:        contract.ToolRead,
			Description: "Read a file, a directory listing, or a past result by its id.",
			Fields: []contract.ToolField{
				{Name: "path", Type: "string", Description: "The file to read.", Required: true},
			},
			Classes: []contract.PermissionClass{contract.ClassRead},
		}},
		MaxOutputTokens: 512,
	}
}

// reportUsage prints what one live call cost, so that the orchestrator can copy
// the numbers into docs/PROGRESS.md.
func reportUsage(t *testing.T, name string, reply contract.Reply, recorder *noteRecorder) {
	t.Helper()
	t.Logf("%s: %d tokens in, %d of them cached, %d out, finished with %q",
		name, reply.Usage.InputTokens, reply.Usage.CachedInputTokens, reply.Usage.OutputTokens, reply.Finish)
	for _, line := range recorder.all() {
		t.Logf("%s: %s", name, line)
	}
}

func TestTheLocalDaemonAnswersTextAndAToolCall(t *testing.T) {
	requireTheLocalDaemonIsUp(t)
	options, recorder := liveOptions(t)
	model, err := provider.New(contract.ModelAlias{
		Name:          contract.LocalModelAlias,
		Provider:      contract.ProviderOpenAI,
		BaseAddress:   liveLocalAddress,
		ModelName:     liveLocalModel,
		ContextLength: 262144,
	}, options)
	if err != nil {
		t.Fatalf("the local model at %s could not be built: %v", liveLocalAddress, err)
	}

	ctx, giveUp := context.WithTimeout(context.Background(), liveCallTimeout)
	defer giveUp()
	if err := testkit.CheckModel(ctx, model); err != nil {
		t.Fatalf("the local model at %s does not keep the model contract: %v", liveLocalAddress, err)
	}

	spoken, streamed, err := sendAndCollect(ctx, model, oneToolRequest("In one short sentence, say what a task record is for."))
	if err != nil {
		t.Fatalf("the local model at %s failed on a plain question: %v", liveLocalAddress, err)
	}
	if strings.TrimSpace(spoken.Text) == "" {
		t.Errorf("the local model answered a plain question with nothing at all")
	}
	if streamed != spoken.Text {
		t.Errorf("the local model's deltas joined to %q and its reply is %q", streamed, spoken.Text)
	}
	reportUsage(t, "local text", spoken, recorder)

	called, _, err := sendAndCollect(ctx, model, oneToolRequest("Read the file notes.txt for me. Use the read tool and nothing else."))
	if err != nil {
		t.Fatalf("the local model at %s failed when asked for a tool: %v", liveLocalAddress, err)
	}
	if len(called.ToolCalls) == 0 {
		t.Fatalf("the local model asked for no tool although it was told to, and it said %q", called.Text)
	}
	if called.ToolCalls[0].Name != contract.ToolRead {
		t.Errorf("the local model asked for the tool %q, want %q", called.ToolCalls[0].Name, contract.ToolRead)
	}
	if !strings.Contains(string(called.ToolCalls[0].Input), "notes") {
		t.Errorf("the local model's arguments came back as %s, and they should name the file", called.ToolCalls[0].Input)
	}
	reportUsage(t, "local tool call", called, recorder)
}

// requireTheLocalDaemonIsUp fails the whole subtest when the daemon is not
// answering, saying how to start it, because a live test never skips.
func requireTheLocalDaemonIsUp(t *testing.T) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	answer, err := client.Get(strings.TrimSuffix(liveLocalAddress, "/v1") + "/health")
	if err != nil {
		t.Fatalf("the local model daemon at %s is not answering, so start it with ~/llm/igo.sh and try again: %v",
			liveLocalAddress, err)
	}
	defer answer.Body.Close()
	if answer.StatusCode != http.StatusOK {
		t.Fatalf("the local model daemon at %s answered its health check with %d, want 200", liveLocalAddress, answer.StatusCode)
	}
}

func TestOpusThroughTheClaudeProgramAnswersTextAndWritesAToolCallInWords(t *testing.T) {
	checkProgramIsThere(t, contract.ClaudeProgram)
	runTheProgramSubtest(t, contract.ClaudeProgram, liveOpusModel)
}

func TestGPTThroughTheCodexProgramAnswersTextAndWritesAToolCallInWords(t *testing.T) {
	checkProgramIsThere(t, contract.CodexProgram)
	runTheProgramSubtest(t, contract.CodexProgram, liveCodexModel)
}

// runTheProgramSubtest asks one subscription program for a plain answer and then
// for a tool, and checks that the tool call came back in the one text form,
// because a program that only returns text cannot return anything else.
func runTheProgramSubtest(t *testing.T, program, modelName string) {
	t.Helper()
	options, recorder := liveOptions(t)
	model, err := provider.New(contract.ModelAlias{
		Name:          program + " on a subscription",
		Provider:      contract.ProviderCommandLine,
		Program:       program,
		ModelName:     modelName,
		ContextLength: 200000,
	}, options)
	if err != nil {
		t.Fatalf("the program %s could not be set up, so check it is installed and signed in: %v", program, err)
	}

	ctx, giveUp := context.WithTimeout(context.Background(), liveCallTimeout)
	defer giveUp()
	if err := testkit.CheckModel(ctx, model); err != nil {
		t.Fatalf("the model behind %s does not keep the model contract: %v", program, err)
	}

	spoken, streamed, err := sendAndCollect(ctx, model, oneToolRequest("In one short sentence, say what a task record is for."))
	if err != nil {
		t.Fatalf("the program %s failed on a plain question, so check it is signed in: %v", program, err)
	}
	if strings.TrimSpace(spoken.Text) == "" {
		t.Errorf("the program %s answered a plain question with nothing at all", program)
	}
	if streamed != spoken.Text {
		t.Errorf("the deltas from %s joined to %q and its reply is %q", program, streamed, spoken.Text)
	}
	reportUsage(t, program+" text", spoken, recorder)

	called, _, err := sendAndCollect(ctx, model,
		oneToolRequest("Read the file notes.txt for me. Write the tool call and nothing else."))
	if err != nil {
		t.Fatalf("the program %s failed when asked for a tool: %v", program, err)
	}
	if len(called.ToolCalls) != 0 {
		t.Errorf("the program %s returned %d structured tool calls, and a program that only returns text returns none",
			program, len(called.ToolCalls))
	}
	if !strings.Contains(called.Text, contract.ToolCallOpenTag) {
		t.Fatalf("the program %s did not write its tool call in the one text form, and it said:\n%s", program, called.Text)
	}
	if !strings.Contains(called.Text, contract.ToolRead) {
		t.Errorf("the program %s wrote a tool call that does not name the read tool:\n%s", program, called.Text)
	}
	reportUsage(t, program+" tool call", called, recorder)
	reportCost(t, program, model)
}

// checkProgramIsThere fails with the program's name when it is not installed,
// which is the message the orchestrator needs to fix it.
func checkProgramIsThere(t *testing.T, program string) {
	t.Helper()
	if _, err := exec.LookPath(program); err != nil {
		t.Fatalf("the program %s is not on the path, so install it, sign in, and put its folder and node's folder on PATH: %v",
			program, err)
	}
}

// reportCost prints what the program said the last call cost, when it said
// anything, so that the orchestrator can record it.
func reportCost(t *testing.T, program string, model contract.Model) {
	t.Helper()
	teller, tells := model.(interface{ LastCostUSD() float64 })
	if !tells {
		return
	}
	t.Logf("%s: the last call cost %s dollars", program, fmt.Sprintf("%.5f", teller.LastCostUSD()))
}
