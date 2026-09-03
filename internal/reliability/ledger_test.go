package reliability_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/reliability"
	"github.com/JaredTate/coeus/internal/testkit"
)

// sentReply is one reply a test's sender was asked to deliver.
type sentReply struct {
	channel string
	text    string
}

// sender collects what it was asked to send, and can be told to refuse.
type sender struct {
	sent   []sentReply
	refuse error
}

// send is the function the ledger is given to deliver a reply with.
func (collector *sender) send(_ context.Context, channel string, text string) error {
	collector.sent = append(collector.sent, sentReply{channel: channel, text: text})
	return collector.refuse
}

// aLedger builds a ledger over a log kept in memory and the clock the test moves.
func aLedger(t *testing.T) (*reliability.Ledger, *testkit.FakeStore, *testkit.FakeClock) {
	t.Helper()
	store := testkit.NewFakeStore()
	clock := testkit.NewFakeClock(startOfTime)
	return reliability.NewLedger(store, clock), store, clock
}

func TestAReplyIsWrittenDownBeforeItIsSentAndMarkedAfter(t *testing.T) {
	ledger, store, _ := aLedger(t)

	reply, err := ledger.Record(context.Background(), "17", "signal", "the post is up")
	if err != nil {
		t.Fatalf("writing the reply down failed: %v", err)
	}
	if reply.ID == "" {
		t.Errorf("the reply was written down without an identifier, so nothing could mark it delivered")
	}

	waiting, err := ledger.Undelivered(context.Background())
	if err != nil {
		t.Fatalf("reading the undelivered replies failed: %v", err)
	}
	if len(waiting) != 1 || waiting[0].Text != "the post is up" {
		t.Fatalf("the log holds %d replies waiting to be sent, want the one just written: %+v", len(waiting), waiting)
	}
	if waiting[0].Channel != "signal" || waiting[0].TaskID != "17" {
		t.Errorf("the waiting reply is %+v, want the channel and task it was written for", waiting[0])
	}

	if err := ledger.MarkDelivered(context.Background(), reply); err != nil {
		t.Fatalf("marking the reply delivered failed: %v", err)
	}

	waiting, err = ledger.Undelivered(context.Background())
	if err != nil {
		t.Fatalf("reading the undelivered replies again failed: %v", err)
	}
	if len(waiting) != 0 {
		t.Errorf("a delivered reply is still waiting to be sent: %+v", waiting)
	}
	if store.Count() != 2 {
		t.Errorf("the log holds %d events, want the reply and the mark that it arrived", store.Count())
	}
}

func TestAReplyThatWasNeverMarkedIsSentAgainWithTheDuplicateMarker(t *testing.T) {
	ledger, _, _ := aLedger(t)
	if _, err := ledger.Record(context.Background(), "17", "signal", "the post is up"); err != nil {
		t.Fatalf("writing the reply down failed: %v", err)
	}
	collector := &sender{}

	sent, err := ledger.Resend(context.Background(), collector.send)
	if err != nil {
		t.Fatalf("sending the undelivered replies again failed: %v", err)
	}

	if sent != 1 || len(collector.sent) != 1 {
		t.Fatalf("%d replies were sent again and the sender saw %d, want one of each", sent, len(collector.sent))
	}
	if !strings.HasPrefix(collector.sent[0].text, reliability.DuplicateMarker) {
		t.Errorf("the reply was sent again without saying it may be a duplicate:\n%s", collector.sent[0].text)
	}
	if !strings.Contains(collector.sent[0].text, "the post is up") {
		t.Errorf("the reply that was sent again does not hold what it said:\n%s", collector.sent[0].text)
	}
	if collector.sent[0].channel != "signal" {
		t.Errorf("the reply went out on %q rather than on the channel it was written for", collector.sent[0].channel)
	}

	waiting, err := ledger.Undelivered(context.Background())
	if err != nil {
		t.Fatalf("reading the undelivered replies failed: %v", err)
	}
	if len(waiting) != 0 {
		t.Errorf("a reply that was sent again is still waiting: %+v", waiting)
	}
}

func TestAReplyIsGivenUpOnAfterThreeAttempts(t *testing.T) {
	ledger, store, _ := aLedger(t)
	if _, err := ledger.Record(context.Background(), "17", "signal", "the post is up"); err != nil {
		t.Fatalf("writing the reply down failed: %v", err)
	}
	collector := &sender{refuse: errors.New("the phone is not reachable right now")}

	for attempt := 1; attempt <= reliability.MaxDeliveryAttempts; attempt++ {
		sent, err := ledger.Resend(context.Background(), collector.send)
		if err != nil {
			t.Fatalf("attempt %d failed: %v", attempt, err)
		}
		if sent != 0 {
			t.Errorf("attempt %d counted %d replies as sent, and the sender refused them", attempt, sent)
		}
	}
	if len(collector.sent) != reliability.MaxDeliveryAttempts {
		t.Fatalf("the sender was asked %d times, want %d", len(collector.sent), reliability.MaxDeliveryAttempts)
	}

	if _, err := ledger.Resend(context.Background(), collector.send); err != nil {
		t.Fatalf("the attempt after the last one failed: %v", err)
	}

	if len(collector.sent) != reliability.MaxDeliveryAttempts {
		t.Errorf("the sender was asked %d times, and the limit is %d", len(collector.sent), reliability.MaxDeliveryAttempts)
	}
	waiting, err := ledger.Undelivered(context.Background())
	if err != nil {
		t.Fatalf("reading the undelivered replies failed: %v", err)
	}
	if len(waiting) != 0 {
		t.Errorf("a reply that was given up on is still waiting to be sent: %+v", waiting)
	}
	if !holdsAGivenUpLine(t, store) {
		t.Errorf("nothing in the log says that the reply was given up on")
	}
}

func TestAReplyOlderThanADayIsGivenUpOnWithoutBeingSent(t *testing.T) {
	ledger, _, clock := aLedger(t)
	if _, err := ledger.Record(context.Background(), "17", "signal", "the post is up"); err != nil {
		t.Fatalf("writing the reply down failed: %v", err)
	}
	clock.Advance(reliability.DeliveryLifetime + time.Minute)
	collector := &sender{}

	sent, err := ledger.Resend(context.Background(), collector.send)
	if err != nil {
		t.Fatalf("sending the undelivered replies again failed: %v", err)
	}

	if sent != 0 || len(collector.sent) != 0 {
		t.Errorf("a reply written %s ago was sent again, and the user would not know what it answered", reliability.DeliveryLifetime)
	}
	waiting, err := ledger.Undelivered(context.Background())
	if err != nil {
		t.Fatalf("reading the undelivered replies failed: %v", err)
	}
	if len(waiting) != 0 {
		t.Errorf("the reply that was given up on is still waiting: %+v", waiting)
	}
}

func TestResendingWithNothingWaitingSendsNothing(t *testing.T) {
	ledger, _, _ := aLedger(t)
	collector := &sender{}

	sent, err := ledger.Resend(context.Background(), collector.send)

	if err != nil || sent != 0 || len(collector.sent) != 0 {
		t.Errorf("a log with no replies in it sent %d replies and said %v", sent, err)
	}
}

func TestAReplyWithNoTextIsRefused(t *testing.T) {
	ledger, _, _ := aLedger(t)

	if _, err := ledger.Record(context.Background(), "17", "signal", ""); err == nil {
		t.Errorf("an empty reply was written into the log, and there is nothing to send")
	}
}

func TestTheLedgerNeedsSomewhereToSend(t *testing.T) {
	ledger, _, _ := aLedger(t)
	if _, err := ledger.Record(context.Background(), "17", "signal", "the post is up"); err != nil {
		t.Fatalf("writing the reply down failed: %v", err)
	}

	if _, err := ledger.Resend(context.Background(), nil); err == nil {
		t.Errorf("the ledger was asked to send with no way of sending, and said nothing")
	}
}

// holdsAGivenUpLine says whether the log holds a reply event that was given up
// on, which is the line a person reads to see why a reply never arrived.
func holdsAGivenUpLine(t *testing.T, store *testkit.FakeStore) bool {
	t.Helper()
	events, err := store.ByKind(context.Background(), contract.EventReply)
	if err != nil {
		t.Fatalf("reading the reply events failed: %v", err)
	}
	for _, event := range events {
		if strings.Contains(string(event.Body), string(reliability.ReplyGivenUp)) {
			return true
		}
	}
	return false
}
