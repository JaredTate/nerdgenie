package main

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestAWaitingPreviewIsListedAndThenAnswered(t *testing.T) {
	held := newWaitingPreviews()
	ctx := context.Background()

	answer, err := held.begin(contract.Preview{ID: "3", Title: "run the command"})
	if err != nil {
		t.Fatalf("beginning to wait on a preview failed: %v", err)
	}

	listed, err := held.list(ctx)
	if err != nil || len(listed) != 1 || listed[0].ID != "3" {
		t.Fatalf("the waiting list is %+v, err %v, want the one question numbered 3", listed, err)
	}

	if err := held.answer(ctx, "3", contract.AnswerOnce, "go ahead"); err != nil {
		t.Fatalf("answering the waiting question failed: %v", err)
	}
	// A second answer while the first is still waiting to be taken is refused,
	// because the one place that holds an answer is full.
	if err := held.answer(ctx, "3", contract.AnswerOnce, ""); err == nil {
		t.Error("a question that has already been answered was answered a second time")
	}

	select {
	case given := <-answer:
		if given.Answer != contract.AnswerOnce || given.Reason != "go ahead" {
			t.Errorf("the answer that reached the question is %+v, want an approval with the reason", given)
		}
	default:
		t.Error("the answer was accepted but never reached the question that was waiting")
	}

	held.end("3")
	if listed, _ := held.list(ctx); len(listed) != 0 {
		t.Errorf("the question is still listed after it was forgotten: %+v", listed)
	}
}

func TestAnsweringAQuestionThatIsNotWaitingSaysSo(t *testing.T) {
	held := newWaitingPreviews()
	if err := held.answer(context.Background(), "99", contract.AnswerOnce, ""); err == nil {
		t.Error("answering under a number nothing is waiting on said nothing was wrong")
	}
}

func TestTheWaitingListIsBounded(t *testing.T) {
	held := newWaitingPreviews()
	for at := 0; at < maxPreviewsWaiting; at++ {
		if _, err := held.begin(contract.Preview{ID: strconv.Itoa(at)}); err != nil {
			t.Fatalf("the %dth question could not begin waiting: %v", at, err)
		}
	}
	if _, err := held.begin(contract.Preview{ID: "one too many"}); err == nil {
		t.Errorf("a %dth question began waiting, and the list holds only %d", maxPreviewsWaiting+1, maxPreviewsWaiting)
	}
}

func TestShowPreviewTakesTheScreensAnswerWhenTheScreenAnswersFirst(t *testing.T) {
	screen := testkit.NewFakeChannel(contract.TerminalChannelName)
	screen.AnswerPreviewsWith(contract.AnswerOnce)
	watched := watchedChannel{Channel: screen, waiting: newWaitingPreviews()}

	given, err := watched.ShowPreview(context.Background(), contract.Preview{ID: "5", Title: "delete the file"})
	if err != nil {
		t.Fatalf("showing the preview failed: %v", err)
	}
	if given.Answer != contract.AnswerOnce {
		t.Errorf("the answer is %+v, want the screen's own approval", given)
	}
}

func TestShowPreviewTakesTheTypedAnswerWhenItLandsFirst(t *testing.T) {
	waiting := newWaitingPreviews()
	watched := watchedChannel{Channel: &blockingChannel{}, waiting: waiting}
	preview := contract.Preview{ID: "6", Title: "push the branch"}

	answered := make(chan contract.PreviewAnswerWithReason, 1)
	go func() {
		given, _ := watched.ShowPreview(context.Background(), preview)
		answered <- given
	}()

	// The screen never answers, so once the question is on the waiting list a
	// slash command's answer is the one that reaches it.
	waitUntilWaiting(t, waiting, "6")
	if err := waiting.answer(context.Background(), "6", contract.AnswerReject, "not yet"); err != nil {
		t.Fatalf("typing the answer failed: %v", err)
	}
	select {
	case given := <-answered:
		if given.Answer != contract.AnswerReject || given.Reason != "not yet" {
			t.Errorf("ShowPreview returned %+v, want the typed rejection", given)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ShowPreview never returned after the typed answer landed")
	}
}

func TestShowPreviewRefusesWhenTheWaitingListIsFull(t *testing.T) {
	waiting := newWaitingPreviews()
	for at := 0; at < maxPreviewsWaiting; at++ {
		if _, err := waiting.begin(contract.Preview{ID: "held" + strconv.Itoa(at)}); err != nil {
			t.Fatalf("filling the waiting list failed: %v", err)
		}
	}
	watched := watchedChannel{Channel: testkit.NewFakeChannel(contract.TerminalChannelName), waiting: waiting}

	given, err := watched.ShowPreview(context.Background(), contract.Preview{ID: "overflow"})
	if err != nil {
		t.Fatalf("ShowPreview returned an error rather than a plain refusal: %v", err)
	}
	if given.Answer != contract.AnswerReject {
		t.Errorf("a question that could not be held was answered %+v, want a refusal", given)
	}
}

// waitUntilWaiting waits until the given preview is on the waiting list, so that
// a test does not answer a question before ShowPreview has written it down.
func waitUntilWaiting(t *testing.T, held *waitingPreviews, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		listed, _ := held.list(context.Background())
		for _, one := range listed {
			if one.ID == id {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("the question %q never reached the waiting list", id)
}

// blockingChannel is a channel whose preview never answers on its own, so that a
// test can prove the typed answer is the one taken.
type blockingChannel struct {
	testkit.FakeChannel
}

// ShowPreview waits until the caller gives up on it, which is what a screen that
// is showing a question but has not been answered looks like from inside.
func (blocking *blockingChannel) ShowPreview(ctx context.Context, _ contract.Preview) (contract.PreviewAnswerWithReason, error) {
	<-ctx.Done()
	return contract.PreviewAnswerWithReason{Answer: contract.AnswerReject, Reason: "the screen was given up on"}, ctx.Err()
}
