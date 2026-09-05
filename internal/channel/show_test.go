package channel

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// showHook is the shape of the function the socket calls to answer a show,
// which is Options.Show.
type showHook = func(ctx context.Context, fields map[string]string) (contract.SocketEnvelope, error)

// aShowRequest is the show a test's screen sends: the result r27 of task 6,
// named the way the contract says a screen names one.
func aShowRequest() contract.SocketEnvelope {
	return contract.SocketEnvelope{
		Type:   contract.SocketShow,
		Fields: map[string]string{"id": "r27", "task": "6"},
	}
}

// recordingShow is a show hook that writes down what it was asked and answers
// with a text made from the id, so a test can see both halves of the exchange.
type recordingShow struct {
	guard sync.Mutex
	asked []map[string]string
}

// answer is the hook itself.
func (hook *recordingShow) answer(_ context.Context, fields map[string]string) (contract.SocketEnvelope, error) {
	hook.guard.Lock()
	defer hook.guard.Unlock()
	hook.asked = append(hook.asked, fields)
	return contract.SocketEnvelope{Type: contract.SocketShown, Text: "the whole of " + fields["id"], Fields: fields}, nil
}

// askedOnce is the one request the hook was given, failing the test when it was
// given none or more than one.
func (hook *recordingShow) askedOnce(t *testing.T) map[string]string {
	t.Helper()
	hook.guard.Lock()
	defer hook.guard.Unlock()
	if len(hook.asked) != 1 {
		t.Fatalf("the show hook was asked %d times, want once", len(hook.asked))
	}
	return hook.asked[0]
}

// TestAShowIsAnsweredToTheAskingScreenAlone holds the whole of the show
// exchange: the socket hands the request's fields to the hook, writes the
// hook's answer back to the screen that asked, and sends nothing to the other
// screens, because a result one person opened is not a reply to everyone.
func TestAShowIsAnsweredToTheAskingScreenAlone(t *testing.T) {
	hook := &recordingShow{}
	harness := newSocketHarnessAnswering(t, theAnswerDeadline, hook.answer)
	asking := harness.attach(t)
	watching := harness.dial(t)
	watching.send(contract.SocketEnvelope{Type: contract.SocketAttach})
	harness.waitForAttached(t, 2)

	asking.send(aShowRequest())

	got := asking.next()
	if got.Type != contract.SocketShown {
		t.Fatalf("the show was answered with a %s, want a shown", got.Type)
	}
	if got.Text != "the whole of r27" {
		t.Errorf("the shown carries %q, want the hook's own text", got.Text)
	}
	if got.Fields["id"] != "r27" || got.Fields["task"] != "6" {
		t.Errorf("the shown carries the fields %v, want the id and the task it was asked with", got.Fields)
	}
	asked := hook.askedOnce(t)
	if asked["id"] != "r27" || asked["task"] != "6" {
		t.Errorf("the hook was asked %v, want the id r27 of task 6", asked)
	}
	if !watching.nothingArrives() {
		t.Error("the other screen was sent the answer to a show it never asked")
	}
}

// TestAShowWithNoHookIsAnsweredWithAnErrorThatSaysSo holds that a program with
// nothing wired to answer a show says so in plain words and keeps the screen
// connected, because asking for a result is not a fault worth hanging up over.
func TestAShowWithNoHookIsAnsweredWithAnErrorThatSaysSo(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.dial(t)

	client.send(aShowRequest())

	got := client.next()
	if got.Type != contract.SocketError {
		t.Fatalf("a show with no hook was answered with a %s, want an error", got.Type)
	}
	if !strings.Contains(got.Text, "show") {
		t.Errorf("the error reads %q, and it has to say that a show cannot be answered", got.Text)
	}
	client.send(aShowRequest())
	if again := client.next(); again.Type != contract.SocketError {
		t.Errorf("the second show was answered with a %s, so the first one hung up on the screen", again.Type)
	}
}

// TestAShowWhoseHookFailsIsAnsweredWithTheFailure holds that the hook's own
// words reach the screen, with the request's fields beside them so a screen
// with several shows out knows which one failed.
func TestAShowWhoseHookFailsIsAnsweredWithTheFailure(t *testing.T) {
	failing := func(context.Context, map[string]string) (contract.SocketEnvelope, error) {
		return contract.SocketEnvelope{}, errors.New("there is no result r27 in task 6, so ask for one the record lists")
	}
	harness := newSocketHarnessAnswering(t, theAnswerDeadline, failing)
	client := harness.dial(t)

	client.send(aShowRequest())

	got := client.next()
	if got.Type != contract.SocketError {
		t.Fatalf("a show the hook refused was answered with a %s, want an error", got.Type)
	}
	if got.Text != "there is no result r27 in task 6, so ask for one the record lists" {
		t.Errorf("the error reads %q, want the hook's own words", got.Text)
	}
	if got.Fields["id"] != "r27" {
		t.Errorf("the error carries the fields %v, want the id it was asked with", got.Fields)
	}
}

// TestAShowThatOutlivesTheDeadlineIsCutOff holds that a hook which takes longer
// than the socket's answer deadline is given up on: the screen is told, the
// hook's context is cancelled, and its late answer never arrives.
func TestAShowThatOutlivesTheDeadlineIsCutOff(t *testing.T) {
	cutOff := make(chan struct{})
	slow := func(ctx context.Context, _ map[string]string) (contract.SocketEnvelope, error) {
		<-ctx.Done()
		close(cutOff)
		return contract.SocketEnvelope{Type: contract.SocketShown, Text: "too late"}, nil
	}
	harness := newSocketHarnessAnswering(t, 50*time.Millisecond, slow)
	client := harness.dial(t)

	client.send(aShowRequest())

	got := client.next()
	if got.Type != contract.SocketError {
		t.Fatalf("a show past the deadline was answered with a %s, want an error", got.Type)
	}
	if !strings.Contains(got.Text, "50ms") {
		t.Errorf("the error reads %q, and it has to say how long the socket waited", got.Text)
	}
	select {
	case <-cutOff:
	case <-time.After(aReadWait):
		t.Fatal("the hook's context was never cancelled after the deadline, so a slow hook runs on forever")
	}
	if !client.nothingArrives() {
		t.Error("the hook's late answer reached the screen after the error")
	}
}

// TestTheShowDeadlineIsTenSecondsWhenTheSocketHasNone holds the fallback: a
// socket with no answer deadline of its own, which is what the shipped caps
// give it, still cuts a show off after ten seconds.
func TestTheShowDeadlineIsTenSecondsWhenTheSocketHasNone(t *testing.T) {
	if DefaultShowDeadline != 10*time.Second {
		t.Errorf("the default show deadline is %s, want ten seconds", DefaultShowDeadline)
	}
	if got := showDeadlineFor(0); got != DefaultShowDeadline {
		t.Errorf("a socket with no deadline cuts a show off after %s, want %s", got, DefaultShowDeadline)
	}
	if got := showDeadlineFor(theAnswerDeadline); got != theAnswerDeadline {
		t.Errorf("a socket with a deadline cuts a show off after %s, want its own %s", got, theAnswerDeadline)
	}
}

// TestAScreenMayNotHaveMoreShowsWaitingThanTheCap holds the bound: one screen
// may have MaxShowsInFlight shows unanswered at once, the next is refused with
// a word about the cap, and the ones in flight still come back once the hook
// lets them go.
func TestAScreenMayNotHaveMoreShowsWaitingThanTheCap(t *testing.T) {
	release := make(chan struct{})
	var running atomic.Int32
	waiting := func(ctx context.Context, _ map[string]string) (contract.SocketEnvelope, error) {
		running.Add(1)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return contract.SocketEnvelope{Type: contract.SocketShown, Text: "done"}, nil
	}
	harness := newSocketHarnessAnswering(t, theAnswerDeadline, waiting)
	client := harness.dial(t)

	for range MaxShowsInFlight {
		client.send(aShowRequest())
	}
	waitUntil(t, func() bool { return int(running.Load()) == MaxShowsInFlight }, "every show up to the cap is running")
	client.send(aShowRequest())

	got := client.next()
	if got.Type != contract.SocketError {
		t.Fatalf("the show past the cap was answered with a %s, want an error", got.Type)
	}
	if !strings.Contains(got.Text, "wait") {
		t.Errorf("the error reads %q, and it has to tell the screen to wait", got.Text)
	}
	close(release)
	for range MaxShowsInFlight {
		if answered := client.next(); answered.Type != contract.SocketShown {
			t.Errorf("a show in flight was answered with a %s after the hook let go, want a shown", answered.Type)
		}
	}
}

// TestAShowAnsweredWithNoTypeIsSentAsAShown holds that a hook which fills in
// only the text still answers with a shown, because that is the one type a
// show is answered with.
func TestAShowAnsweredWithNoTypeIsSentAsAShown(t *testing.T) {
	bare := func(context.Context, map[string]string) (contract.SocketEnvelope, error) {
		return contract.SocketEnvelope{Text: "the text"}, nil
	}
	harness := newSocketHarnessAnswering(t, theAnswerDeadline, bare)
	client := harness.dial(t)

	client.send(aShowRequest())

	if got := client.next(); got.Type != contract.SocketShown || got.Text != "the text" {
		t.Errorf("the show was answered with a %s carrying %q, want a shown carrying the text", got.Type, got.Text)
	}
}

// TestAScreenThatHangsUpEndsItsShow holds that a hook still running when the
// screen hangs up is cancelled rather than left to run for the whole deadline
// answering nobody.
func TestAScreenThatHangsUpEndsItsShow(t *testing.T) {
	started := make(chan struct{})
	cutOff := make(chan struct{})
	slow := func(ctx context.Context, _ map[string]string) (contract.SocketEnvelope, error) {
		close(started)
		<-ctx.Done()
		close(cutOff)
		return contract.SocketEnvelope{Type: contract.SocketShown, Text: "nobody is listening"}, nil
	}
	harness := newSocketHarnessAnswering(t, theAnswerDeadline, slow)
	client := harness.dial(t)

	client.send(aShowRequest())
	select {
	case <-started:
	case <-time.After(aReadWait):
		t.Fatal("the show hook never started")
	}
	_ = client.socket.Close()

	select {
	case <-cutOff:
	case <-time.After(aReadWait):
		t.Fatal("the hook's context was never cancelled after the screen hung up")
	}
}

// TestAShownTextIsRedactedLikeEverythingElse holds that a result read back out
// of the log goes through the vault's redactor on its way to the screen, the
// way every reply does, because a stored result may hold a secret a tool saw.
func TestAShownTextIsRedactedLikeEverythingElse(t *testing.T) {
	leaking := func(context.Context, map[string]string) (contract.SocketEnvelope, error) {
		return contract.SocketEnvelope{Type: contract.SocketShown, Text: "the token is hunter2-secret"}, nil
	}
	harness := newSocketHarnessAnswering(t, theAnswerDeadline, leaking)
	harness.secrets.Add("token", contract.Credential{Site: "token", Password: "hunter2-secret"})
	client := harness.dial(t)

	client.send(aShowRequest())

	if got := client.next(); strings.Contains(got.Text, "hunter2-secret") {
		t.Errorf("the shown carries %q, and the secret in it was not redacted", got.Text)
	}
}

// waitUntil polls a condition for the test's read wait and fails when it never
// comes true.
func waitUntil(t *testing.T, condition func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(aReadWait)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("after %s it is still not true that %s", aReadWait, what)
}
