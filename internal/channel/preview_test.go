package channel

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// aPreview is what the permission function puts in front of the user before
// anything on the ask-me-first list runs.
var aPreview = contract.Preview{
	ID:    "3",
	Title: "run a command with sudo",
	Body:  "sudo systemctl restart nginx",
}

// previewResult is what one call to ShowPreview came back with.
type previewResult struct {
	answer contract.PreviewAnswer
	reason string
	err    error
}

// showPreview starts one preview in the background and hands back where its
// answer will arrive.
func (harness *socketHarness) showPreview(ctx context.Context) chan previewResult {
	answers := make(chan previewResult, 1)
	go func() {
		answer, err := harness.socket.ShowPreview(ctx, aPreview)
		answers <- previewResult{answer: answer.Answer, reason: answer.Reason, err: err}
	}()
	return answers
}

// waitForSleepers waits until the wanted number of callers are waiting on the
// fake clock, so that a test can move the clock knowing the waiter is there.
func (harness *socketHarness) waitForSleepers(t *testing.T, wanted int) {
	t.Helper()
	deadline := time.Now().Add(aReadWait)
	for time.Now().Before(deadline) {
		if harness.clock.Sleepers() >= wanted {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("%d callers are waiting on the clock after %s, want %d", harness.clock.Sleepers(), aReadWait, wanted)
}

func TestAPreviewIsShownOnEveryScreenAndApprovedOverTheSocket(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)
	answers := harness.showPreview(context.Background())

	shown := client.next()
	if shown.Type != contract.SocketPreview {
		t.Fatalf("the screen saw a %s, want a preview", shown.Type)
	}
	if shown.ID != aPreview.ID {
		t.Errorf("the preview arrived under the number %q, want %q", shown.ID, aPreview.ID)
	}
	if shown.Title != aPreview.Title {
		t.Errorf("the preview's title is %q, want %q", shown.Title, aPreview.Title)
	}
	if shown.Text != aPreview.Body {
		t.Errorf("the preview shows %q, want exactly what is about to happen: %q", shown.Text, aPreview.Body)
	}

	client.send(contract.SocketEnvelope{Type: contract.SocketApprove, ID: shown.ID})
	got := <-answers
	if got.err != nil {
		t.Fatalf("showing the preview failed: %v", got.err)
	}
	if got.answer != contract.AnswerOnce {
		t.Errorf("the preview was answered %q, want %q", got.answer, contract.AnswerOnce)
	}
}

func TestApprovingWithAlwaysIsTheAnswerForTheWholeSession(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)
	answers := harness.showPreview(context.Background())

	shown := client.next()
	client.send(contract.SocketEnvelope{
		Type: contract.SocketApprove,
		ID:   shown.ID,
		Text: contract.ApproveAlwaysText,
	})

	got := <-answers
	if got.err != nil {
		t.Fatalf("showing the preview failed: %v", got.err)
	}
	if got.answer != contract.AnswerAlways {
		t.Errorf("the preview was answered %q, want %q", got.answer, contract.AnswerAlways)
	}
}

func TestAPreviewIsDeniedOverTheSocket(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)
	answers := harness.showPreview(context.Background())

	shown := client.next()
	client.send(contract.SocketEnvelope{
		Type:   contract.SocketDeny,
		ID:     shown.ID,
		Reason: "not on the live server",
	})

	got := <-answers
	if got.err != nil {
		t.Fatalf("showing the preview failed: %v", got.err)
	}
	if got.answer != contract.AnswerReject {
		t.Errorf("the preview was answered %q, want %q", got.answer, contract.AnswerReject)
	}
	if got.reason != "not on the live server" {
		t.Errorf("the refusal carried the reason %q, want the words the person typed", got.reason)
	}
}

func TestTheReasonTypedWithARefusalReachesTheCallerAndAnApprovalCarriesNone(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)

	// The reason is the whole point of a refusal: the model is told why in the
	// user's own words, so that it tries another way rather than the same way.
	refused := harness.showPreview(context.Background())
	shown := client.next()
	client.send(contract.SocketEnvelope{
		Type:   contract.SocketDeny,
		ID:     shown.ID,
		Reason: "never touch the web server while the shop is open",
	})
	got := <-refused
	if got.err != nil {
		t.Fatalf("showing the preview failed: %v", got.err)
	}
	if got.answer != contract.AnswerReject {
		t.Fatalf("the preview was answered %q, want %q", got.answer, contract.AnswerReject)
	}
	if got.reason != "never touch the web server while the shop is open" {
		t.Errorf("the refusal reached the caller with the reason %q, want the words the person typed", got.reason)
	}

	approved := harness.showPreview(context.Background())
	shown = client.next()
	client.send(contract.SocketEnvelope{Type: contract.SocketApprove, ID: shown.ID})
	if answer := <-approved; answer.reason != "" {
		t.Errorf("an approval carried the reason %q, and only a refusal has one", answer.reason)
	}
}

func TestAPreviewNobodyAnswersInTimeIsRefused(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)
	answers := harness.showPreview(context.Background())

	if shown := client.next(); shown.Type != contract.SocketPreview {
		t.Fatalf("the screen saw a %s, want a preview", shown.Type)
	}
	harness.waitForSleepers(t, 1)
	harness.clock.Advance(DefaultAnswerDeadline + time.Second)

	got := <-answers
	if got.err != nil {
		t.Fatalf("a preview nobody answered came back with an error rather than a no: %v", got.err)
	}
	if got.answer != contract.AnswerReject {
		t.Errorf("a preview nobody answered came back as %q, want %q, because nothing happens without a yes", got.answer, contract.AnswerReject)
	}
}

func TestAPreviewWithNoScreenAttachedIsRefusedAtOnce(t *testing.T) {
	harness := newSocketHarness(t)
	harness.dial(t)

	answer, err := harness.socket.ShowPreview(context.Background(), aPreview)
	if err != nil {
		t.Fatalf("showing a preview with nobody attached failed: %v", err)
	}
	if answer.Answer != contract.AnswerReject {
		t.Errorf("a preview nobody could see came back as %q, want %q", answer.Answer, contract.AnswerReject)
	}
}

func TestAPreviewWhoseCallerGivesUpIsRefusedAndSaysWhy(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)
	ctx, stop := context.WithCancel(context.Background())
	answers := harness.showPreview(ctx)

	if shown := client.next(); shown.Type != contract.SocketPreview {
		t.Fatalf("the screen saw a %s, want a preview", shown.Type)
	}
	stop()

	got := <-answers
	if got.answer != contract.AnswerReject {
		t.Errorf("a preview the caller gave up on came back as %q, want %q", got.answer, contract.AnswerReject)
	}
	if got.err == nil {
		t.Error("a preview the caller gave up on came back with no error, and the caller has to know nobody answered")
	}
}

func TestAPreviewWithNoNumberOfItsOwnIsGivenOne(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)
	answers := make(chan previewResult, 1)
	go func() {
		answer, err := harness.socket.ShowPreview(context.Background(), contract.Preview{
			Title: "post to X",
			Body:  "Coeus can now book flights.",
		})
		answers <- previewResult{answer: answer.Answer, reason: answer.Reason, err: err}
	}()

	shown := client.next()
	if shown.ID == "" {
		t.Fatal("a preview with no number of its own arrived without one, and there is no way to answer it")
	}
	client.send(contract.SocketEnvelope{Type: contract.SocketApprove, ID: shown.ID})
	if got := <-answers; got.answer != contract.AnswerOnce {
		t.Errorf("the preview was answered %q, want %q", got.answer, contract.AnswerOnce)
	}
}

func TestTwoPreviewsUnderTheSameNumberAreRefusedRatherThanLeftHanging(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)
	first := harness.showPreview(context.Background())
	shown := client.next()

	answer, err := harness.socket.ShowPreview(context.Background(), aPreview)
	if err == nil {
		t.Fatal("a second preview took the number the first is waiting on, and one of them could never be answered")
	}
	if answer.Answer != contract.AnswerReject {
		t.Errorf("the second preview came back as %q, want %q", answer.Answer, contract.AnswerReject)
	}

	// The first is still waiting and is still the one that number answers.
	client.send(contract.SocketEnvelope{Type: contract.SocketApprove, ID: shown.ID})
	if got := <-first; got.answer != contract.AnswerOnce {
		t.Errorf("the first preview was answered %q, want %q", got.answer, contract.AnswerOnce)
	}
}

func TestAnAnswerToANumberNothingIsWaitingOnIsRefusedAndTheClientStays(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)

	for _, answer := range []contract.SocketMessageType{
		contract.SocketApprove, contract.SocketDeny, contract.SocketSecret,
	} {
		client.send(contract.SocketEnvelope{Type: answer, ID: "404"})
		got := client.next()
		if got.Type != contract.SocketError {
			t.Errorf("a %s for a number nothing is waiting on was answered with a %s, want an error", answer, got.Type)
		}
		if !strings.Contains(got.Text, "404") {
			t.Errorf("the error reads %q, and it has to name the number that was sent", got.Text)
		}
	}
}
