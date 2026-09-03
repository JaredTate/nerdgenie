package reliability_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/reliability"
	"github.com/JaredTate/coeus/internal/testkit"
)

// aGuardThatSends builds a guard whose sender is the function given, so that a
// test can fail the send or look at the log from inside it.
func aGuardThatSends(t *testing.T, home contract.Home, store *testkit.FakeStore, send func(ctx context.Context, channel string, text string) error) *reliability.Guard {
	t.Helper()
	guard, err := reliability.New(reliability.Settings{
		Home:  home,
		Clock: testkit.NewFakeClock(startOfTime),
		Store: store,
		Caps:  contract.DefaultConfig().Caps,
		Send:  send,
	})
	if err != nil {
		t.Fatalf("building the guard failed: %v", err)
	}
	return guard
}

func TestAReplyIsWrittenDownBeforeTheGuardSendsItAndMarkedAfter(t *testing.T) {
	store := testkit.NewFakeStore()
	waitingWhenItWasSent := -1
	guard := aGuardThatSends(t, testkit.NewTempHome(t), store, func(context.Context, string, string) error {
		waiting, err := reliability.NewLedger(store, testkit.NewFakeClock(startOfTime)).Undelivered(context.Background())
		if err != nil {
			t.Errorf("reading the waiting replies from inside the send failed: %v", err)
		}
		waitingWhenItWasSent = len(waiting)
		return nil
	})

	if err := guard.Deliver(context.Background(), "17", "signal", "the post is up"); err != nil {
		t.Fatalf("delivering the reply failed: %v", err)
	}

	if waitingWhenItWasSent != 1 {
		t.Errorf("%d replies were written down when the send ran, want the one being sent: nothing is sent that was not written down first", waitingWhenItWasSent)
	}
	waiting, err := reliability.NewLedger(store, testkit.NewFakeClock(startOfTime)).Undelivered(context.Background())
	if err != nil {
		t.Fatalf("reading the waiting replies failed: %v", err)
	}
	if len(waiting) != 0 {
		t.Errorf("%d replies are still waiting after one was delivered, so the next start would send it again", len(waiting))
	}
}

func TestAReplyTheGuardCouldNotSendIsSentAgainAfterTheRestart(t *testing.T) {
	home := testkit.NewTempHome(t)
	store := testkit.NewFakeStore()
	nobodyIsListening := errors.New("no screen is attached, so the reply reached nobody")
	guard := aGuardThatSends(t, home, store, func(context.Context, string, string) error { return nobodyIsListening })
	if _, err := guard.Start(context.Background()); err != nil {
		t.Fatalf("the first start failed: %v", err)
	}

	err := guard.Deliver(context.Background(), "17", "signal", "the post is up")

	if !errors.Is(err, nobodyIsListening) {
		t.Fatalf("delivering to nobody came back with %v, want the reason the send failed", err)
	}
	// The program is killed here. The reply was written down and never marked
	// delivered, which is what the next start finds and sends again.
	afterTheCrash, told := aGuard(t, home, store)
	found, err := afterTheCrash.Start(context.Background())
	if err != nil {
		t.Fatalf("the start after the crash failed: %v", err)
	}
	if found.RepliesSentAgain != 1 {
		t.Fatalf("%d replies were sent again, want the one that reached nobody", found.RepliesSentAgain)
	}
	if len(told.sent) != 1 || !strings.HasPrefix(told.sent[0].text, reliability.DuplicateMarker) {
		t.Errorf("the reply was sent again as %q, want it marked as a possible duplicate", told.sent)
	}
}
