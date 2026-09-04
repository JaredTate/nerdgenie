package channel

import (
	"context"
	"sync"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aDelta is one event of the shape the loop publishes, used wherever a test only
// needs something to send.
func aDelta(text string) contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketDelta, TaskID: "t1", Text: text}
}

func TestTwoSubscribersGetTheSameEventsInTheSameOrder(t *testing.T) {
	stream := NewStream(StreamOptions{})
	defer stream.Close()

	first, err := stream.Subscribe()
	if err != nil {
		t.Fatalf("subscribing the first reader failed: %v", err)
	}
	second, err := stream.Subscribe()
	if err != nil {
		t.Fatalf("subscribing the second reader failed: %v", err)
	}

	written := []string{"one", "two", "three"}
	for _, text := range written {
		if err := stream.Publish(aDelta(text)); err != nil {
			t.Fatalf("publishing %q failed: %v", text, err)
		}
	}

	for name, subscription := range map[string]*Subscription{"first": first, "second": second} {
		for _, want := range written {
			got := <-subscription.Events()
			if got.Text != want {
				t.Errorf("the %s reader saw %q, want %q", name, got.Text, want)
			}
		}
	}
}

func TestASlowSubscriberIsDroppedAndTheFastOneIsUnaffected(t *testing.T) {
	stream := NewStream(StreamOptions{})
	defer stream.Close()

	slow, err := stream.Subscribe()
	if err != nil {
		t.Fatalf("subscribing the slow reader failed: %v", err)
	}
	fast, err := stream.Subscribe()
	if err != nil {
		t.Fatalf("subscribing the fast reader failed: %v", err)
	}

	// The fast reader takes every event as it arrives; the slow one never reads.
	sent := SubscriberBacklog + 5
	for number := range sent {
		if err := stream.Publish(aDelta("event")); err != nil {
			t.Fatalf("publishing event %d failed: %v", number, err)
		}
		if got, open := <-fast.Events(); !open {
			t.Fatalf("the fast reader lost its stream at event %d, and it never fell behind", number)
		} else if got.Text != "event" {
			t.Fatalf("the fast reader saw %q at event %d, want %q", got.Text, number, "event")
		}
	}

	if !slow.Dropped() {
		t.Error("the slow reader was kept even though it fell past the backlog cap")
	}
	// A dropped reader keeps what it had already been given and then finds its
	// stream closed, which is how it learns it fell behind.
	held := 0
	for range slow.Events() {
		held++
	}
	if held != SubscriberBacklog {
		t.Errorf("the dropped reader held %d events, want the backlog cap of %d", held, SubscriberBacklog)
	}
	if fast.Dropped() {
		t.Error("the fast reader was dropped, and one slow reader must never cost another reader its stream")
	}
	if held := stream.Subscribers(); held != 1 {
		t.Errorf("the stream holds %d readers after the slow one was dropped, want 1", held)
	}
}

func TestTwoFakeChannelsFedFromTheStreamSeeTheSameReplies(t *testing.T) {
	ctx := context.Background()
	stream := NewStream(StreamOptions{})
	defer stream.Close()

	terminal, signal := testkit.NewFakeChannel("terminal"), testkit.NewFakeChannel("signal")
	var forwarding sync.WaitGroup
	for _, target := range []*testkit.FakeChannel{terminal, signal} {
		subscription := mustSubscribe(t, stream)
		forwarding.Add(1)
		go func() {
			defer forwarding.Done()
			for envelope := range subscription.Events() {
				if err := target.Send(ctx, envelope.Text); err != nil {
					t.Errorf("sending through %s failed: %v", target.Name(), err)
				}
			}
		}()
	}

	want := []string{"the first reply", "the second reply"}
	for _, text := range want {
		if err := stream.Publish(contract.SocketEnvelope{Type: contract.SocketReply, Text: text}); err != nil {
			t.Fatalf("publishing %q failed: %v", text, err)
		}
	}
	stream.Close()
	forwarding.Wait()

	for _, target := range []*testkit.FakeChannel{terminal, signal} {
		sent := target.Sent()
		if len(sent) != len(want) {
			t.Fatalf("%s carried %d replies, want %d", target.Name(), len(sent), len(want))
		}
		for index, text := range want {
			if sent[index] != text {
				t.Errorf("%s carried %q as reply %d, want %q", target.Name(), sent[index], index, text)
			}
		}
	}
}

func TestPublishRefusesAMessageOnlyAScreenSends(t *testing.T) {
	stream := NewStream(StreamOptions{})
	defer stream.Close()

	if err := stream.Publish(contract.SocketEnvelope{Type: contract.SocketMessage, Text: "hello"}); err == nil {
		t.Fatal("the stream carried a message type that only a screen sends, want an error naming the type")
	}
}

func TestSubscribeRefusesPastTheCap(t *testing.T) {
	stream := NewStream(StreamOptions{})
	defer stream.Close()

	for number := range MaxSubscribers {
		if _, err := stream.Subscribe(); err != nil {
			t.Fatalf("subscriber %d was refused below the cap: %v", number, err)
		}
	}
	if _, err := stream.Subscribe(); err == nil {
		t.Fatalf("subscriber %d was taken, and the cap is %d", MaxSubscribers+1, MaxSubscribers)
	}
	if held := stream.Subscribers(); held != MaxSubscribers {
		t.Errorf("the stream holds %d subscribers, want %d", held, MaxSubscribers)
	}
}

func TestClosingTheStreamClosesEverySubscription(t *testing.T) {
	stream := NewStream(StreamOptions{})
	subscription := mustSubscribe(t, stream)

	stream.Close()
	if _, open := <-subscription.Events(); open {
		t.Error("the subscription is still open after the stream closed")
	}
	if err := stream.Publish(aDelta("after the close")); err == nil {
		t.Error("a closed stream carried an event, want an error saying it is closed")
	}
	if held := stream.Subscribers(); held != 0 {
		t.Errorf("the closed stream still holds %d subscribers, want none", held)
	}
	if _, err := stream.Subscribe(); err == nil {
		t.Error("a closed stream took a new subscriber, want an error saying it is closed")
	}
	stream.Close()
}

func TestClosingOneSubscriptionLeavesTheOthers(t *testing.T) {
	stream := NewStream(StreamOptions{})
	defer stream.Close()

	going, staying := mustSubscribe(t, stream), mustSubscribe(t, stream)
	going.Close()
	going.Close()

	if held := stream.Subscribers(); held != 1 {
		t.Fatalf("the stream holds %d subscribers after one closed, want 1", held)
	}
	if err := stream.Publish(aDelta("still here")); err != nil {
		t.Fatalf("publishing after one subscriber left failed: %v", err)
	}
	if got := <-staying.Events(); got.Text != "still here" {
		t.Errorf("the remaining subscriber saw %q, want %q", got.Text, "still here")
	}
	if going.Dropped() {
		t.Error("a subscriber that closed itself is marked dropped, and only falling behind means dropped")
	}
}

// mustSubscribe subscribes to the stream and fails the test when it cannot.
func mustSubscribe(t *testing.T, stream *Stream) *Subscription {
	t.Helper()
	subscription, err := stream.Subscribe()
	if err != nil {
		t.Fatalf("subscribing failed: %v", err)
	}
	return subscription
}
