package channel

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// arrived is the moment the tests hand to the queue, with nanoseconds in it so
// that a time losing precision on the way to the file would show up.
var arrived = time.Date(2026, time.April, 9, 10, 11, 12, 130456789, time.UTC)

// newTestQueue opens a queue in a folder the test framework removes afterwards
// and closes it when the test ends.
func newTestQueue(t *testing.T, capacity int) *Queue {
	t.Helper()
	opened, err := OpenQueue(context.Background(), filepath.Join(t.TempDir(), "coeus.db"), capacity)
	if err != nil {
		t.Fatalf("cannot open a queue for the test: %v", err)
	}
	t.Cleanup(func() {
		if err := opened.Close(); err != nil {
			t.Errorf("closing the queue failed: %v", err)
		}
	})
	return opened
}

// anInbound is one message as a channel hands it over, with everything filled in
// but the text.
func anInbound(text string) contract.Inbound {
	return contract.Inbound{
		ID:       "m1",
		Sender:   "jared",
		Text:     text,
		Received: arrived,
		Channel:  contract.TerminalChannelName,
	}
}

func TestAMessageComesBackOutOfTheQueueExactlyAsItWentIn(t *testing.T) {
	queue := newTestQueue(t, 10)
	ctx := context.Background()
	sent := anInbound("post the weekly note")
	sent.Attachments = []string{"/home/jared/.coeus/inbox/photo.jpg", "/home/jared/.coeus/inbox/note.txt"}

	sequence, err := queue.Add(ctx, sent)
	if err != nil {
		t.Fatalf("adding the message failed: %v", err)
	}
	if sequence != 1 {
		t.Errorf("the queue numbered the first message %d, want 1", sequence)
	}

	taken, held, err := queue.Take(ctx)
	if err != nil {
		t.Fatalf("taking the message failed: %v", err)
	}
	if !held {
		t.Fatal("the queue said it was empty right after a message was added")
	}
	if taken.Sequence != sequence {
		t.Errorf("the message came back as number %d, want %d", taken.Sequence, sequence)
	}
	if taken.Duplicate {
		t.Error("a message handed out for the first time is marked a duplicate")
	}
	if taken.Message.ID != sent.ID || taken.Message.Sender != sent.Sender || taken.Message.Text != sent.Text {
		t.Errorf("the message came back as %+v, want %+v", taken.Message, sent)
	}
	if taken.Message.Channel != sent.Channel {
		t.Errorf("the message came back from the channel %q, want %q", taken.Message.Channel, sent.Channel)
	}
	if !taken.Message.Received.Equal(sent.Received) {
		t.Errorf("the message came back received at %s, want %s", taken.Message.Received, sent.Received)
	}
	if taken.Message.Received.Location() != time.UTC {
		t.Errorf("the message came back in the zone %s, and the queue keeps every time in UTC", taken.Message.Received.Location())
	}
	if strings.Join(taken.Message.Attachments, ",") != strings.Join(sent.Attachments, ",") {
		t.Errorf("the attachments came back as %v, want %v", taken.Message.Attachments, sent.Attachments)
	}
}

func TestTheQueueHandsMessagesOutInTheOrderTheyArrived(t *testing.T) {
	queue := newTestQueue(t, 10)
	ctx := context.Background()
	for _, text := range []string{"first", "second", "third"} {
		if _, err := queue.Add(ctx, anInbound(text)); err != nil {
			t.Fatalf("adding %q failed: %v", text, err)
		}
	}

	for _, want := range []string{"first", "second", "third"} {
		taken, held, err := queue.Take(ctx)
		if err != nil || !held {
			t.Fatalf("taking %q failed: held %t, error %v", want, held, err)
		}
		if taken.Message.Text != want {
			t.Errorf("the queue handed out %q, want %q", taken.Message.Text, want)
		}
		if err := queue.Done(ctx, taken.Sequence); err != nil {
			t.Fatalf("marking %q done failed: %v", want, err)
		}
	}
}

func TestAMessageTakenButNotFinishedIsHandedOutAgainAfterARestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "coeus.db")

	queue, err := OpenQueue(ctx, path, 10)
	if err != nil {
		t.Fatalf("opening the queue failed: %v", err)
	}
	if _, err := queue.Add(ctx, anInbound("book the flight")); err != nil {
		t.Fatalf("adding the message failed: %v", err)
	}
	taken, held, err := queue.Take(ctx)
	if err != nil || !held {
		t.Fatalf("taking the message failed: held %t, error %v", held, err)
	}
	if err := queue.Close(); err != nil {
		t.Fatalf("closing the queue failed: %v", err)
	}

	reopened, err := OpenQueue(ctx, path, 10)
	if err != nil {
		t.Fatalf("reopening the queue failed: %v", err)
	}
	defer func() { _ = reopened.Close() }()

	again, held, err := reopened.Take(ctx)
	if err != nil || !held {
		t.Fatalf("the message was not handed out again after the restart: held %t, error %v", held, err)
	}
	if again.Sequence != taken.Sequence {
		t.Errorf("the message came back as number %d, want the same number %d", again.Sequence, taken.Sequence)
	}
	if again.Message.Text != "book the flight" {
		t.Errorf("the message came back as %q, want %q", again.Message.Text, "book the flight")
	}
	if !again.Duplicate {
		t.Error("the message came back without the duplicate marker, and the task it started may already have run")
	}
}

func TestAMessageMarkedDoneIsGoneAfterARestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "coeus.db")

	queue, err := OpenQueue(ctx, path, 10)
	if err != nil {
		t.Fatalf("opening the queue failed: %v", err)
	}
	if _, err := queue.Add(ctx, anInbound("water the plants")); err != nil {
		t.Fatalf("adding the message failed: %v", err)
	}
	taken, _, err := queue.Take(ctx)
	if err != nil {
		t.Fatalf("taking the message failed: %v", err)
	}
	if err := queue.Done(ctx, taken.Sequence); err != nil {
		t.Fatalf("marking the message done failed: %v", err)
	}
	if err := queue.Close(); err != nil {
		t.Fatalf("closing the queue failed: %v", err)
	}

	reopened, err := OpenQueue(ctx, path, 10)
	if err != nil {
		t.Fatalf("reopening the queue failed: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	if _, held, err := reopened.Take(ctx); held || err != nil {
		t.Errorf("a finished message was handed out again after the restart: held %t, error %v", held, err)
	}
}

func TestAFullQueueRefusesWithALineTheChannelCanSendBack(t *testing.T) {
	queue := newTestQueue(t, 2)
	ctx := context.Background()
	for _, text := range []string{"first", "second"} {
		if _, err := queue.Add(ctx, anInbound(text)); err != nil {
			t.Fatalf("adding %q failed: %v", text, err)
		}
	}

	_, err := queue.Add(ctx, anInbound("third"))
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("the third message was refused with %v, want the full-queue error", err)
	}
	if words := len(strings.Fields(err.Error())); words < 6 {
		t.Errorf("the refusal reads %q, and it is sent to the user, so it has to say what to do", err)
	}

	waiting, err := queue.Held(ctx)
	if err != nil {
		t.Fatalf("counting what the queue holds failed: %v", err)
	}
	if waiting != 2 {
		t.Errorf("the queue holds %d messages, want 2", waiting)
	}
}

func TestAMessageInHandStillCountsAgainstTheCap(t *testing.T) {
	queue := newTestQueue(t, 1)
	ctx := context.Background()
	if _, err := queue.Add(ctx, anInbound("first")); err != nil {
		t.Fatalf("adding the first message failed: %v", err)
	}
	taken, _, err := queue.Take(ctx)
	if err != nil {
		t.Fatalf("taking the first message failed: %v", err)
	}

	if _, err := queue.Add(ctx, anInbound("second")); !errors.Is(err, ErrQueueFull) {
		t.Errorf("a message was taken while one was still in hand, and the error was %v", err)
	}
	if err := queue.Done(ctx, taken.Sequence); err != nil {
		t.Fatalf("marking the first message done failed: %v", err)
	}
	if _, err := queue.Add(ctx, anInbound("second")); err != nil {
		t.Errorf("the queue stayed full after the message in hand was finished: %v", err)
	}
}

func TestTakingFromAnEmptyQueueSaysThereIsNothing(t *testing.T) {
	queue := newTestQueue(t, 10)
	taken, held, err := queue.Take(context.Background())
	if err != nil {
		t.Fatalf("taking from an empty queue failed: %v", err)
	}
	if held {
		t.Errorf("an empty queue handed out %+v", taken)
	}
}

func TestAddingWakesWhoeverIsWaitingOnTheQueue(t *testing.T) {
	queue := newTestQueue(t, 10)
	select {
	case <-queue.Arrived():
		t.Fatal("the queue said something had arrived before anything was added")
	default:
	}

	if _, err := queue.Add(context.Background(), anInbound("wake up")); err != nil {
		t.Fatalf("adding the message failed: %v", err)
	}
	select {
	case <-queue.Arrived():
	case <-time.After(time.Second):
		t.Fatal("nobody was woken when a message arrived")
	}
}

func TestTheQueueRefusesWhatItCannotHandOut(t *testing.T) {
	queue := newTestQueue(t, 10)
	ctx := context.Background()
	empty := anInbound("")
	noChannel := anInbound("hello")
	noChannel.Channel = ""

	if _, err := queue.Add(ctx, empty); err == nil {
		t.Error("a message with no text and no attachment was queued, want an error saying there is nothing in it")
	}
	if _, err := queue.Add(ctx, noChannel); err == nil {
		t.Error("a message with no channel was queued, and a reply would have nowhere to go")
	}
}

func TestDoneOnANumberTheQueueDoesNotHoldIsAnError(t *testing.T) {
	queue := newTestQueue(t, 10)
	if err := queue.Done(context.Background(), 404); err == nil {
		t.Error("the queue finished a message it never held, want an error naming the number")
	}
}

func TestOpeningTheQueueRefusesACapacityBelowOneAndAPathWithAQuestionMark(t *testing.T) {
	ctx := context.Background()
	folder := t.TempDir()

	if _, err := OpenQueue(ctx, filepath.Join(folder, "coeus.db"), 0); err == nil {
		t.Error("a queue with room for no messages was opened, want an error saying the cap has to be at least one")
	}
	if _, err := OpenQueue(ctx, filepath.Join(folder, "what?.db"), 10); err == nil {
		t.Error("a path with a question mark in it was opened, and the database driver reads that as its own options")
	}
	if _, err := OpenQueue(ctx, folder, 10); err == nil {
		t.Error("a folder was opened as a queue, want an error naming the path")
	}
}

func TestEveryCallOnAClosedQueueSaysItIsClosed(t *testing.T) {
	ctx := context.Background()
	queue, err := OpenQueue(ctx, filepath.Join(t.TempDir(), "coeus.db"), 10)
	if err != nil {
		t.Fatalf("opening the queue failed: %v", err)
	}
	if err := queue.Close(); err != nil {
		t.Fatalf("closing the queue failed: %v", err)
	}
	if err := queue.Close(); err != nil {
		t.Errorf("closing a closed queue failed: %v", err)
	}

	if _, err := queue.Add(ctx, anInbound("hello")); err == nil {
		t.Error("a closed queue took a message")
	}
	if _, _, err := queue.Take(ctx); err == nil {
		t.Error("a closed queue handed out a message")
	}
	if err := queue.Done(ctx, 1); err == nil {
		t.Error("a closed queue finished a message")
	}
	if _, err := queue.Held(ctx); err == nil {
		t.Error("a closed queue counted what it holds")
	}
}
