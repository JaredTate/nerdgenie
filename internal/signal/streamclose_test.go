package signal

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/testkit"
)

// The contract promises that the stream Receive hands back closes when the
// context is cancelled, so that a loop written as "for message := range" ends.
func TestTheInboundStreamClosesWhenTheContextIsCancelled(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	channel, _, _ := newTestChannel(t, baseOf(daemon), clock)
	ctx, cancel := context.WithCancel(context.Background())
	inbound, err := channel.Receive(ctx)
	if err != nil {
		t.Fatalf("receiving failed: %v", err)
	}
	cancel()
	select {
	case _, open := <-inbound:
		if open {
			t.Fatal("the stream handed back a message after the context was cancelled")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the stream stayed open two seconds after the context was cancelled")
	}
}
