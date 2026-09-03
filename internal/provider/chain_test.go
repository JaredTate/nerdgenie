package provider_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/provider"
	"github.com/JaredTate/coeus/internal/testkit"
)

// modelOn builds a plain provider pointed at an address, under the alias given.
func modelOn(t *testing.T, name, address string, options provider.Options) contract.Model {
	t.Helper()
	alias := localAliasAt(address, 262144)
	alias.Name = name
	model, err := provider.New(alias, options)
	if err != nil {
		t.Fatalf("building the provider %q failed: %v", name, err)
	}
	return model
}

func TestTheChainMovesOnWhenTheFirstModelIsOutOfTries(t *testing.T) {
	broken := alwaysFailingServer()
	defer broken.Close()
	working := testkit.NewFakeProviderServer(scriptSayingOneThing("the second model answered"))
	defer working.Close()
	clock := newTestClock()
	options, recorder := testOptions(t, clock)
	chain, err := provider.NewChain([]contract.Model{
		provider.WithRetries(modelOn(t, "first", broken.URL, options), options),
		modelOn(t, "second", working.Address(), options),
	}, options)
	if err != nil {
		t.Fatalf("building the chain failed: %v", err)
	}

	done := sendInBackground(chain, requestWithEverything())
	for note := 1; note <= 2; note++ {
		waitForNotes(t, recorder, note)
		waitForSleeper(t, clock, 1)
		clock.Advance(30 * time.Second)
	}
	finished := waitForResult(t, done)

	if finished.err != nil {
		t.Fatalf("the chain gave up although its second model works: %v", finished.err)
	}
	if finished.reply.Text != "the second model answered" {
		t.Errorf("the reply is %q, want the second model's answer", finished.reply.Text)
	}
	if finished.reply.Model != "second" {
		t.Errorf("the reply says %q answered, want second", finished.reply.Model)
	}
	if !strings.Contains(strings.Join(recorder.all(), "\n"), "moving on to the model \"second\"") {
		t.Errorf("moving on to the next model was not written down: %v", recorder.all())
	}
}

func TestTheChainNeverReplaysTheTextTheModelItGaveUpOnStreamed(t *testing.T) {
	breaking := serverThatBreaksOffAfterSaying("Hello ", "never reached")
	defer breaking.Close()
	working := testkit.NewFakeProviderServer(scriptSayingOneThing("Hello world"))
	defer working.Close()
	options, _ := testOptions(t, newTestClock())
	chain, err := provider.NewChain([]contract.Model{
		modelOn(t, "first", breaking.URL, options),
		modelOn(t, "second", working.Address(), options),
	}, options)
	if err != nil {
		t.Fatalf("building the chain failed: %v", err)
	}

	reply, streamed, err := sendAndCollect(context.Background(), chain, requestWithEverything())

	if err != nil {
		t.Fatalf("the chain gave up although its second model works: %v", err)
	}
	if reply.Text != "Hello world" {
		t.Fatalf("the reply is %q, want the second model's whole answer", reply.Text)
	}
	if streamed != reply.Text {
		t.Errorf("the caller was streamed %q while the reply is %q, and a caller must never be handed the text of a model the chain gave up on",
			streamed, reply.Text)
	}
}

func TestTheChainStopsAtAnOverflowRatherThanMovingOn(t *testing.T) {
	first := testkit.NewFakeProviderServer(scriptSayingOneThing("never reached"))
	defer first.Close()
	second := testkit.NewFakeProviderServer(scriptSayingOneThing("never reached either"))
	defer second.Close()
	options, _ := testOptions(t, newTestClock())
	chain, err := provider.NewChain([]contract.Model{
		modelOn(t, "first", first.Address(), options),
		modelOn(t, "second", second.Address(), options),
	}, options)
	if err != nil {
		t.Fatalf("building the chain failed: %v", err)
	}
	first.MisbehaveNext(testkit.OverflowTheContext, 0)

	_, err = chain.Send(context.Background(), requestWithEverything(), nil)

	if !errors.Is(err, contract.ErrContextOverflow) {
		t.Fatalf("a prompt that was too long came back as %v, want the overflow sentinel", err)
	}
	if calls := callsTo(second, testkit.OpenAIPath); calls != 0 {
		t.Errorf("the second model was called %d times, and an overflow is the context builder's problem, not the model's", calls)
	}
}

func TestTheChainReturnsTheLastErrorWhenEveryModelFails(t *testing.T) {
	broken := alwaysFailingServer()
	defer broken.Close()
	alsoBroken := alwaysFailingServer()
	defer alsoBroken.Close()
	options, _ := testOptions(t, newTestClock())
	chain, err := provider.NewChain([]contract.Model{
		modelOn(t, "first", broken.URL, options),
		modelOn(t, "second", alsoBroken.URL, options),
	}, options)
	if err != nil {
		t.Fatalf("building the chain failed: %v", err)
	}

	_, err = chain.Send(context.Background(), requestWithEverything(), nil)

	if err == nil {
		t.Fatal("a chain whose models all failed came back as a good reply")
	}
	if !strings.Contains(err.Error(), "second") {
		t.Errorf("the error is not the last model's: %v", err)
	}
}

func TestAChainWithNoModelsIsRefused(t *testing.T) {
	options, _ := testOptions(t, newTestClock())

	_, err := provider.NewChain(nil, options)

	if err == nil {
		t.Fatal("a chain with no models was accepted, and there is nothing for it to call")
	}
}

func TestTheChainReportsTheModelItWillTryFirst(t *testing.T) {
	first := testkit.NewFakeProviderServer(scriptSayingOneThing("done"))
	defer first.Close()
	options, _ := testOptions(t, newTestClock())
	chain, err := provider.NewChain([]contract.Model{modelOn(t, "first", first.Address(), options)}, options)
	if err != nil {
		t.Fatalf("building the chain failed: %v", err)
	}

	if chain.Name() != "first" {
		t.Errorf("the chain calls itself %q, want the model it tries first", chain.Name())
	}
	if chain.ContextLength() != 262144 {
		t.Errorf("the chain reports a window of %d, want the first model's", chain.ContextLength())
	}
}

func TestTheChainPassesTheContractCheck(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("Anything at all."))
	defer server.Close()
	options, _ := testOptions(t, newTestClock())
	chain, err := provider.NewChain([]contract.Model{modelOn(t, "only", server.Address(), options)}, options)
	if err != nil {
		t.Fatalf("building the chain failed: %v", err)
	}

	if err := testkit.CheckModel(context.Background(), chain); err != nil {
		t.Fatalf("the fallback chain does not keep the model contract: %v", err)
	}

	reply, _, err := sendAndCollect(context.Background(), chain, requestWithEverything())
	if err != nil {
		t.Fatalf("one call through the chain failed: %v", err)
	}
	if reply.Model != "only" {
		t.Errorf("the reply says %q answered, want the one model in the chain", reply.Model)
	}
}
