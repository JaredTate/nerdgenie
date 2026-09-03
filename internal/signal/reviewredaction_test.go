package signal_test

// What the wave 6 security review found on the way out over Signal, and on the
// way in through pairing. Design section 11 says one redaction pass runs on
// everything that leaves the program, and section 12 says an unknown sender
// gets a pairing code.

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/signal"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/vault"
)

// theSecret is a password long enough for the redactor to black out, and long
// enough that a split can fall in the middle of it.
const theSecret = "correct-horse-battery-staple-9137"

func TestASecretSplitAcrossTwoMessagesIsStillBlackedOut(t *testing.T) {
	home := testkit.NewTempHome(t)
	held, err := vault.Open(home, testkit.NewFakeClock(time.Unix(0, 0).UTC()))
	if err != nil {
		t.Fatalf("opening the vault failed: %v", err)
	}
	t.Cleanup(func() { _ = held.Close() })
	if err := held.Add(vault.Entry{Name: "a-site", Password: theSecret}); err != nil {
		t.Fatalf("adding the secret failed: %v", err)
	}

	// A reply long enough that Signal's own length limit falls in the middle of
	// the secret. The channel splits the reply and then redacts each piece on
	// its own, so neither piece holds the whole secret and neither is changed.
	padding := strings.Repeat("a", signal.MessageLimit-len(theSecret)/2)
	reply := padding + theSecret + " and that is the end of it."

	joined := &strings.Builder{}
	for _, piece := range signal.SplitReply(reply) {
		joined.WriteString(held.Redact(piece))
	}
	if strings.Contains(joined.String(), theSecret) {
		t.Errorf("the secret came back whole once the pieces were put together again; the redactor runs on one message at a time,"+
			" so a value that falls across a split leaves the program in two halves that a reader joins back up."+
			" Redact the whole reply before it is split, or split on a boundary the redactor has already passed. The pieces were %d.",
			len(signal.SplitReply(reply)))
	}
}

func TestThreeStrangersCannotStopTheOwnerFromPairing(t *testing.T) {
	home := testkit.NewTempHome(t)
	clock := testkit.NewFakeClock(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	pairing, err := signal.NewPairing(home, clock)
	if err != nil {
		t.Fatalf("opening the pairing store failed: %v", err)
	}

	strangers := []string{"+15550000001", "+15550000002", "+15550000003"}
	for _, stranger := range strangers {
		if _, offered, err := pairing.Offer(stranger); err != nil || !offered {
			t.Fatalf("the stranger %s was not offered a code: offered=%v err=%v", stranger, offered, err)
		}
	}

	// The owner's own second phone writes for the first time. Three codes are
	// already waiting, so it is told nothing at all, and it stays told nothing
	// for as long as the three keep asking every ten minutes.
	if _, offered, err := pairing.Offer("+15125550123"); err != nil || !offered {
		t.Errorf("the owner's own phone was offered no pairing code because three unknown senders hold the three waiting slots,"+
			" and it was told nothing at all, so three strangers can shut a user out of their own assistant: offered=%v err=%v", offered, err)
	}
}
