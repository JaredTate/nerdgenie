package provider_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/provider"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// The tests in this file are the ways a call to the Codex backend goes wrong
// and what the provider does about each: a sign-in that is missing, expired,
// unreadable, or not the codex program's; a refusal; a rate limit and an
// overload tried again on the fake clock; and a stream that goes quiet.

func TestTheCodexProviderFailsPlainlyWhenTheSignInFileIsMissing(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{events: codexTextStream("never reached", "")})
	t.Setenv("CODEX_HOME", t.TempDir())
	options, _ := testOptions(t, newTestClock())
	model, err := provider.New(codexAliasAt(backend.address(), contract.ThinkDefault), options)
	if err != nil {
		t.Fatalf("building the Codex provider failed: %v", err)
	}

	_, err = model.Send(context.Background(), requestWithEverything(), nil)

	if err == nil {
		t.Fatal("a call with no sign-in file came back as a good reply")
	}
	if !strings.Contains(err.Error(), "auth.json") || !strings.Contains(err.Error(), "run codex once") {
		t.Errorf("the error does not name the sign-in file and say to run codex once to sign in: %v", err)
	}
	if backend.callCount() != 0 {
		t.Errorf("the backend was called %d times with no sign-in at all", backend.callCount())
	}
}

func TestTheCodexProviderFailsPlainlyWhenTheTokenHasExpired(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{events: codexTextStream("never reached", "")})
	writeCodexLogin(t, codexLoginFile(codexTokenExpiring(testClockNow().Add(-time.Hour), "account-from-the-token")))
	options, lines := testOptions(t, newTestClock())
	model, err := provider.New(codexAliasAt(backend.address(), contract.ThinkDefault), options)
	if err != nil {
		t.Fatalf("building the Codex provider failed: %v", err)
	}

	_, err = model.Send(context.Background(), requestWithEverything(), nil)

	if err == nil {
		t.Fatal("a call with an expired token came back as a good reply")
	}
	if !strings.Contains(err.Error(), "expired") || !strings.Contains(err.Error(), "run codex once") {
		t.Errorf("the error does not say the sign-in has expired and to run codex once to refresh it: %v", err)
	}
	if backend.callCount() != 0 {
		t.Errorf("the backend was called %d times with an expired token", backend.callCount())
	}
	checkNoTokenIn(t, err, lines)
}

func TestTheCodexProviderFailsPlainlyWhenTheSignInFileCannotBeRead(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{events: codexTextStream("never reached", "")})
	folder := t.TempDir()
	t.Setenv("CODEX_HOME", folder)
	if err := os.Mkdir(filepath.Join(folder, "auth.json"), 0o700); err != nil {
		t.Fatalf("the folder standing in for an unreadable file could not be made: %v", err)
	}
	options, _ := testOptions(t, newTestClock())
	model, err := provider.New(codexAliasAt(backend.address(), contract.ThinkDefault), options)
	if err != nil {
		t.Fatalf("building the Codex provider failed: %v", err)
	}

	_, err = model.Send(context.Background(), requestWithEverything(), nil)

	if err == nil {
		t.Fatal("a call whose sign-in file cannot be read came back as a good reply")
	}
	if !strings.Contains(err.Error(), "could not be read") {
		t.Errorf("the error does not say the sign-in file could not be read: %v", err)
	}
}

func TestTheCodexProviderFailsPlainlyWhenTheSignInFileIsNotWhatCodexWrites(t *testing.T) {
	tests := []struct {
		name     string
		contents string
	}{
		{"not JSON", "this is not JSON {"},
		{"no tokens at all", `{"auth_mode":"apikey","OPENAI_API_KEY":null}`},
		{"an empty token", `{"tokens":{"access_token":"","account_id":"x"}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newCodexBackend(t, codexAnswer{events: codexTextStream("never reached", "")})
			writeCodexLogin(t, test.contents)
			options, _ := testOptions(t, newTestClock())
			model, err := provider.New(codexAliasAt(backend.address(), contract.ThinkDefault), options)
			if err != nil {
				t.Fatalf("building the Codex provider failed: %v", err)
			}

			_, err = model.Send(context.Background(), requestWithEverything(), nil)

			if err == nil {
				t.Fatal("a call with a sign-in file holding no token came back as a good reply")
			}
			if !strings.Contains(err.Error(), "run codex once") {
				t.Errorf("the error does not say to run codex once to sign in: %v", err)
			}
			if strings.Contains(err.Error(), test.contents) {
				t.Errorf("the error repeats the file's contents, and a sign-in file is never printed: %v", err)
			}
		})
	}
}

func TestTheCodexProviderRefusesATokenWhoseClaimsItCannotReadWithoutPrintingIt(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{events: codexTextStream("never reached", "")})
	writeCodexLogin(t, codexLoginFile("not.a.token."+codexSecretToken))
	options, lines := testOptions(t, newTestClock())
	model, err := provider.New(codexAliasAt(backend.address(), contract.ThinkDefault), options)
	if err != nil {
		t.Fatalf("building the Codex provider failed: %v", err)
	}

	_, err = model.Send(context.Background(), requestWithEverything(), nil)

	if err == nil {
		t.Fatal("a call with a token whose claims cannot be read came back as a good reply")
	}
	if !strings.Contains(err.Error(), "run codex once") {
		t.Errorf("the error does not say to run codex once to sign in again: %v", err)
	}
	if backend.callCount() != 0 {
		t.Errorf("the backend was called %d times with a token the provider could not read", backend.callCount())
	}
	checkNoTokenIn(t, err, lines)
}

func TestTheCodexProviderTellsTheUserToSignInAgainOnAnUnauthorizedAnswer(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{status: http.StatusUnauthorized, body: `{"detail":"Unauthorized"}`})
	model, lines := codexAgainst(t, backend)

	_, err := model.Send(context.Background(), requestWithEverything(), nil)

	if err == nil {
		t.Fatal("a call the backend refused as unauthorized came back as a good reply")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "run codex once") {
		t.Errorf("the error does not carry the status and say to run codex once to sign in again: %v", err)
	}
	checkNoTokenIn(t, err, lines)
}

// checkNoTokenIn fails when the token from the fake sign-in file appears in an
// error or in any line the provider logged, because a token is a secret and
// the program never prints one.
func checkNoTokenIn(t *testing.T, err error, lines *noteRecorder) {
	t.Helper()
	if err != nil && strings.Contains(err.Error(), codexSecretToken) {
		t.Errorf("the error carries the access token, and a token is never printed: %v", err)
	}
	for _, line := range lines.all() {
		if strings.Contains(line, codexSecretToken) {
			t.Errorf("a log line carries the access token, and a token is never printed: %s", line)
		}
	}
}

func TestTheCodexProviderRetriesARateLimitedCallAfterTheHeadersWait(t *testing.T) {
	backend := newCodexBackend(t,
		codexAnswer{status: http.StatusTooManyRequests, retryAfter: "9", body: `{"error":{"message":"slow down"}}`},
		codexAnswer{events: codexTextStream("after the wait", "")},
	)
	goodCodexLogin(t)
	clock := newTestClock()
	options, recorder := testOptions(t, clock)
	base, err := provider.New(codexAliasAt(backend.address(), contract.ThinkDefault), options)
	if err != nil {
		t.Fatalf("building the Codex provider failed: %v", err)
	}
	model := provider.WithRetries(base, options)

	done := sendInBackground(model, requestWithEverything())
	waitForNotes(t, recorder, 1)
	waitForSleeper(t, clock, 1)
	clock.Advance(9 * time.Second)
	finished := waitForResult(t, done)

	if finished.err != nil {
		t.Fatalf("a rate-limited call that was tried again came back as %v", finished.err)
	}
	if finished.reply.Text != "after the wait" {
		t.Errorf("the reply is %q, want the second attempt's answer", finished.reply.Text)
	}
	if backend.callCount() != 2 {
		t.Errorf("the backend was called %d times, want two: the refusal and the retry", backend.callCount())
	}
	if !strings.Contains(recorder.all()[0], "9s") {
		t.Errorf("the retry line does not say the wait the header asked for: %v", recorder.all())
	}
	checkNoTokenIn(t, finished.err, recorder)
}

func TestTheCodexProviderRetriesAnOverloadedAnswer(t *testing.T) {
	backend := newCodexBackend(t,
		codexAnswer{status: http.StatusServiceUnavailable, body: `{"error":{"message":"overloaded"}}`},
		codexAnswer{events: codexTextStream("after the wait", "")},
	)
	goodCodexLogin(t)
	clock := newTestClock()
	options, recorder := testOptions(t, clock)
	base, err := provider.New(codexAliasAt(backend.address(), contract.ThinkDefault), options)
	if err != nil {
		t.Fatalf("building the Codex provider failed: %v", err)
	}
	model := provider.WithRetries(base, options)

	done := sendInBackground(model, requestWithEverything())
	waitForNotes(t, recorder, 1)
	waitForSleeper(t, clock, 1)
	clock.Advance(2 * time.Second)
	finished := waitForResult(t, done)

	if finished.err != nil {
		t.Fatalf("an overloaded call that was tried again came back as %v", finished.err)
	}
	if backend.callCount() != 2 {
		t.Errorf("the backend was called %d times, want two: the refusal and the retry", backend.callCount())
	}
}

func TestTheCodexProviderGivesUpOnAStreamThatGoesQuiet(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{stall: true})
	goodCodexLogin(t)
	clock := newTestClock()
	options, _ := testOptions(t, clock)
	model, err := provider.New(codexAliasAt(backend.address(), contract.ThinkDefault), options)
	if err != nil {
		t.Fatalf("building the Codex provider failed: %v", err)
	}

	done := sendInBackground(model, requestWithEverything())
	waitForSleeper(t, clock, 1)
	clock.Advance(90 * time.Second)
	finished := waitForResult(t, done)

	if !errors.Is(finished.err, contract.ErrStalledStream) {
		t.Fatalf("a stream that sent nothing came back as %v, want the stalled-stream sentinel", finished.err)
	}
	if !strings.Contains(finished.err.Error(), "gpt") {
		t.Errorf("the error does not name the model whose stream went quiet: %v", finished.err)
	}
}

func TestTheCodexProviderReturnsTheOverflowSentinel(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{status: http.StatusBadRequest,
		body: `{"error":{"message":"Your input exceeds the context window of this model.","code":"context_length_exceeded"}}`})
	model, _ := codexAgainst(t, backend)

	_, err := model.Send(context.Background(), requestWithEverything(), nil)

	if !errors.Is(err, contract.ErrContextOverflow) {
		t.Fatalf("a prompt that was too long came back as %v, want the overflow sentinel", err)
	}
}

func TestTheCodexProviderPassesTheContractCheck(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{events: codexTextStream("Anything ", "at all.")})
	model, _ := codexAgainst(t, backend)

	if err := testkit.CheckModel(context.Background(), model); err != nil {
		t.Fatalf("the Codex provider does not keep the model contract: %v", err)
	}
}

func TestTheCodexProviderNeedsNoBaseAddressAndNoKey(t *testing.T) {
	goodCodexLogin(t)
	options, _ := testOptions(t, newTestClock())
	alias := codexAliasAt("", contract.ThinkDefault)

	model, err := provider.New(alias, options)

	if err != nil {
		t.Fatalf("building the Codex provider with no base address failed, and the backend has one address: %v", err)
	}
	if model.Name() != "gpt" || model.ContextLength() != 400000 {
		t.Errorf("the model calls itself %q with a window of %d, want the alias's gpt and 400000", model.Name(), model.ContextLength())
	}
}

func TestTheCodexProviderRefusesAThinkLevelNobodyOffers(t *testing.T) {
	backend := newCodexBackend(t, codexAnswer{events: codexTextStream("ok", "")})
	model, _ := codexAgainst(t, backend)
	request := requestWithEverything()
	request.Think = contract.Think("harder")

	_, err := model.Send(context.Background(), request, nil)

	if err == nil || !strings.Contains(err.Error(), "harder") {
		t.Fatalf("a level nobody offers came back as %v, want a refusal naming it", err)
	}
	if backend.callCount() != 0 {
		t.Errorf("the backend was called %d times with a level nobody offers", backend.callCount())
	}
}
