package signal_test

// What the wave 6 security review found on the way out over Signal, and on the
// way in through pairing. Design section 11 says one redaction pass runs on
// everything that leaves the program, and section 12 says an unknown sender
// gets a pairing code.

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/signal"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/vault"
)

// theSecret is a password long enough for the redactor to black out, and long
// enough that a split can fall in the middle of it.
const theSecret = "correct-horse-battery-staple-9137"

// theAccount is the number the agent is linked as here, which is also where a
// reply goes while nobody has written in.
const theAccount = "+15125550100"

func TestASecretSplitAcrossTwoMessagesIsStillBlackedOut(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	home := testkit.NewTempHome(t)
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	held, err := vault.Open(home, clock)
	if err != nil {
		t.Fatalf("opening the vault failed: %v", err)
	}
	t.Cleanup(func() { _ = held.Close() })
	if err := held.Add(vault.Entry{Name: "a-site", Password: theSecret}); err != nil {
		t.Fatalf("adding the secret failed: %v", err)
	}

	host, port := hostAndPortOf(t, strings.TrimSuffix(daemon.HealthAddress(), testkit.SignalHealthPath))
	channel, err := signal.NewChannel(signal.ChannelOptions{
		Account: theAccount,
		Host:    host,
		Port:    port,
		Home:    home,
		Clock:   clock,
		Secrets: held,
	})
	if err != nil {
		t.Fatalf("building the Signal channel failed: %v", err)
	}
	t.Cleanup(func() { _ = channel.Close() })

	// A reply long enough that Signal's own length limit falls in the middle of
	// the secret, so that a channel which redacts each piece after the split
	// sends the secret out in two halves a reader joins back up.
	padding := strings.Repeat("a", signal.MessageLimit-len(theSecret)/2)
	reply := padding + theSecret + " and that is the end of it."
	if err := channel.Send(context.Background(), reply); err != nil {
		t.Fatalf("sending the reply failed: %v", err)
	}

	sent := daemon.Sends()
	if len(sent) < 2 {
		t.Fatalf("the reply went out in %d messages, and this test needs one long enough to be split in two", len(sent))
	}
	joined := &strings.Builder{}
	for _, one := range sent {
		joined.WriteString(one.Message)
	}
	if strings.Contains(joined.String(), theSecret) {
		t.Errorf("the secret came back whole once the %d messages were put together again; the redactor runs on one message at a time,"+
			" so a value that falls across a split leaves the program in two halves that a reader joins back up."+
			" Redact the whole reply before it is split.", len(sent))
	}
	if !strings.Contains(joined.String(), contract.RedactedMarker) {
		t.Errorf("nothing in the reply was blacked out at all, and the reply held a password the vault knows")
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

// hostAndPortOf pulls the host and the port out of an address such as
// "http://127.0.0.1:41234", which is how the fake daemon says where it is.
func hostAndPortOf(t *testing.T, address string) (string, int) {
	t.Helper()
	parsed, err := url.Parse(address)
	if err != nil {
		t.Fatalf("cannot read the daemon address %q: %v", address, err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("cannot read the port out of %q: %v", address, err)
	}
	return parsed.Hostname(), port
}
