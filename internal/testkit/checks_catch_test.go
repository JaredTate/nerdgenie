package testkit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// A contract check is only worth having if it catches something. Each type below
// takes a working fake and breaks exactly one promise, and each test proves the
// check notices. This is the orchestrator's rule written as code: change one
// thing the test should catch, and confirm it fails.

// namelessChannel is a channel that forgot its own name.
type namelessChannel struct{ *testkit.FakeChannel }

// Name returns nothing, which no channel may do.
func (namelessChannel) Name() string { return "" }

// confusedChannel answers a preview with a word that is not one of the three.
type confusedChannel struct{ *testkit.FakeChannel }

// ShowPreview answers with a word the harness does not understand.
func (confusedChannel) ShowPreview(context.Context, contract.Preview) (contract.PreviewAnswer, error) {
	return "maybe", nil
}

// silentlyUnwellChannel says it is broken and will not say why.
type silentlyUnwellChannel struct{ *testkit.FakeChannel }

// Health says the channel is unwell and gives no reason.
func (silentlyUnwellChannel) Health(context.Context) contract.ChannelHealth {
	return contract.ChannelHealth{Healthy: false}
}

func TestTheChannelCheckCatchesAChannelThatBreaksOnePromise(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		channel contract.Channel
	}{
		{"a channel with no name", namelessChannel{testkit.NewFakeChannel("terminal")}},
		{"a channel that answers a preview with a word nobody knows", confusedChannel{testkit.NewFakeChannel("terminal")}},
		{"a channel that is unwell and will not say why", silentlyUnwellChannel{testkit.NewFakeChannel("terminal")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := testkit.CheckChannel(ctx, test.channel); err == nil {
				t.Fatal("the channel check passed, and it was given a channel that breaks a promise")
			}
		})
	}
}

// namelessModel is a model that forgot its own name.
type namelessModel struct{ *testkit.FakeModel }

// Name returns nothing, which no model may do.
func (namelessModel) Name() string { return "" }

// windowlessModel is a model that reports no context window.
type windowlessModel struct{ *testkit.FakeModel }

// ContextLength returns nothing, and the working-context rule is sized from it.
func (windowlessModel) ContextLength() int { return 0 }

// mismatchedModel streams one thing and returns another.
type mismatchedModel struct{ *testkit.FakeModel }

// Send streams deltas that do not join to the text it returns.
func (mismatchedModel) Send(context.Context, contract.Request, func(string)) (contract.Reply, error) {
	return contract.Reply{Text: "one thing", Finish: contract.FinishEnd}, nil
}

// unfinishedModel reports a finish reason the contract does not name.
type unfinishedModel struct{ *testkit.FakeModel }

// Send returns a finish reason nobody defined.
func (unfinishedModel) Send(_ context.Context, _ contract.Request, onDelta func(string)) (contract.Reply, error) {
	if onDelta != nil {
		onDelta("hello")
	}
	return contract.Reply{Text: "hello", Finish: "gave up"}, nil
}

// overCachedModel says more of the input was cached than there was input.
type overCachedModel struct{ *testkit.FakeModel }

// Send reports a cached count larger than the input count.
func (overCachedModel) Send(_ context.Context, _ contract.Request, onDelta func(string)) (contract.Reply, error) {
	if onDelta != nil {
		onDelta("hello")
	}
	return contract.Reply{
		Text:   "hello",
		Finish: contract.FinishEnd,
		Usage:  contract.Usage{InputTokens: 10, CachedInputTokens: 100},
	}, nil
}

// workingModel is the fake model the broken ones above are built from.
func workingModel() *testkit.FakeModel {
	return testkit.NewFakeModel(testkit.Script{
		Name:          "broken",
		ContextLength: 24000,
		Steps:         []testkit.Step{{Text: "an answer", Finish: contract.FinishEnd}},
	})
}

func TestTheModelCheckCatchesAModelThatBreaksOnePromise(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		model contract.Model
	}{
		{"a model with no name", namelessModel{workingModel()}},
		{"a model with no window", windowlessModel{workingModel()}},
		{"a model whose deltas do not join to its reply", mismatchedModel{workingModel()}},
		{"a model that finished for a reason nobody defined", unfinishedModel{workingModel()}},
		{"a model that cached more than it read", overCachedModel{workingModel()}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := testkit.CheckModel(ctx, test.model); err == nil {
				t.Fatal("the model check passed, and it was given a model that breaks a promise")
			}
		})
	}
}

// forgetfulStore hands back the same sequence number every time.
type forgetfulStore struct{ *testkit.FakeStore }

// Append always says the event was numbered one.
func (forgetfulStore) Append(context.Context, contract.Event) (int64, error) { return 1, nil }

// forgivingStore never says an event is missing.
type forgivingStore struct{ *testkit.FakeStore }

// ByID makes up an event rather than saying there is none.
func (forgivingStore) ByID(context.Context, int64) (contract.Event, error) {
	return contract.Event{Sequence: 1}, nil
}

func TestTheStoreCheckCatchesALogThatBreaksOnePromise(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		store contract.Store
	}{
		{"a log whose sequence numbers do not grow", forgetfulStore{testkit.NewFakeStore()}},
		{"a log that invents an event that is not there", forgivingStore{testkit.NewFakeStore()}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := testkit.CheckStore(ctx, test.store); err == nil {
				t.Fatal("the store check passed, and it was given a log that breaks a promise")
			}
		})
	}
}

// forgetfulMemory takes a fact and never writes it down.
type forgetfulMemory struct{ *testkit.FakeMemory }

// Save pretends to write and does nothing.
func (forgetfulMemory) Save(context.Context, []contract.Fact) error { return nil }

// chattyMemory returns more hint lines than the prompt has room for.
type chattyMemory struct{ *testkit.FakeMemory }

// Hint returns four lines when the cap is three.
func (chattyMemory) Hint(context.Context, string) ([]string, error) {
	return []string{"one", "two", "three", "four"}, nil
}

func TestTheMemoryCheckCatchesAMemoryThatBreaksOnePromise(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name   string
		memory contract.Memory
	}{
		{"a memory that never writes anything down", forgetfulMemory{testkit.NewFakeMemory()}},
		{"a memory whose hint is longer than the cap", chattyMemory{testkit.NewFakeMemory()}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := testkit.CheckMemory(ctx, test.memory); err == nil {
				t.Fatal("the memory check passed, and it was given a memory that breaks a promise")
			}
		})
	}
}

// backwardsClock is a clock that runs the wrong way.
type backwardsClock struct{ *testkit.FakeClock }

// Sleep never gives up, even when the context has been cancelled.
func (backwardsClock) Sleep(context.Context, time.Duration) error { return nil }

func TestTheClockCheckCatchesAClockThatIgnoresACancelledContext(t *testing.T) {
	if err := testkit.CheckClock(backwardsClock{testkit.NewFakeClock(time.Unix(0, 0).UTC())}); err == nil {
		t.Fatal("the clock check passed, and it was given a clock that ignores a cancelled context")
	}
}

// recklessSandbox says it cannot run and runs anyway.
type recklessSandbox struct{ *testkit.FakeSandbox }

// Available always reports that bwrap is missing.
func (recklessSandbox) Available() error {
	return errors.New("bwrap is not installed, so install bubblewrap and try again")
}

// Run runs the command even though the sandbox is unavailable.
func (recklessSandbox) Run(context.Context, contract.SandboxCommand) (contract.SandboxResult, error) {
	return contract.SandboxResult{}, nil
}

func TestTheSandboxCheckCatchesAnUnavailableSandboxThatRunsAnyway(t *testing.T) {
	if err := testkit.CheckSandbox(context.Background(), recklessSandbox{testkit.NewFakeSandbox()}); err == nil {
		t.Fatal("the sandbox check passed, and an unavailable sandbox ran a command")
	}
}

// boastfulSandbox is available and reports every command as a success without
// looking at it, which is what replacing the whole body of Run with a constant
// looks like.
type boastfulSandbox struct{ *testkit.FakeSandbox }

// Run reports a success whatever it was asked to run.
func (boastfulSandbox) Run(context.Context, contract.SandboxCommand) (contract.SandboxResult, error) {
	return contract.SandboxResult{}, nil
}

func TestTheSandboxCheckCatchesASandboxThatSaysEverythingWorked(t *testing.T) {
	if err := testkit.CheckSandbox(context.Background(), boastfulSandbox{testkit.NewFakeSandbox()}); err == nil {
		t.Fatal("the sandbox check passed a sandbox that reported a program nobody has as a success")
	}
}

// leakyVault hands back a credential for anything it is asked about.
type leakyVault struct{ *testkit.FakeSecrets }

// Resolve answers every reference, whatever it says.
func (leakyVault) Resolve(context.Context, string) (contract.Credential, error) {
	return contract.Credential{Site: "anything"}, nil
}

// mangler changes text that holds no secret at all.
type mangler struct{ *testkit.FakeSecrets }

// Redact rewrites everything, secret or not.
func (mangler) Redact(string) string { return contract.RedactedMarker }

func TestTheSecretsCheckCatchesAVaultThatBreaksOnePromise(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		secrets contract.Secrets
	}{
		{"a vault that answers any reference at all", leakyVault{testkit.NewFakeSecrets()}},
		{"a redactor that rewrites text holding no secret", mangler{testkit.NewFakeSecrets()}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := testkit.CheckSecrets(ctx, test.secrets); err == nil {
				t.Fatal("the secrets check passed, and it was given a vault that breaks a promise")
			}
		})
	}
}
