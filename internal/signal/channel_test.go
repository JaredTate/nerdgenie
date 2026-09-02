package signal

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// testAccount is the number the agent is linked as in these tests.
const testAccount = "+15125550100"

// newTestChannel builds a Signal channel talking to a daemon somebody else runs,
// which is the fake, and hands back the pairing store so a test can pair a
// sender before it starts talking.
func newTestChannel(t *testing.T, address string, clock *testkit.FakeClock) (*Channel, *Pairing, contract.Home) {
	t.Helper()
	home := testkit.NewTempHome(t)
	host, port := splitAddress(t, address)
	channel, err := NewChannel(ChannelOptions{
		Account: testAccount,
		Host:    host,
		Port:    port,
		Home:    home,
		Clock:   clock,
		Secrets: testkit.NewFakeSecrets(),
	})
	if err != nil {
		t.Fatalf("cannot build the Signal channel: %v", err)
	}
	t.Cleanup(func() { _ = channel.Close() })
	return channel, channel.Pairing(), home
}

// splitAddress pulls the host and the port out of an address such as
// "http://127.0.0.1:41234".
func splitAddress(t *testing.T, address string) (string, int) {
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

// pairSender puts one sender straight onto the approved list.
func pairSender(t *testing.T, pairing *Pairing, sender string) {
	t.Helper()
	code := offerTo(t, pairing, sender)
	if _, err := pairing.Approve(code); err != nil {
		t.Fatalf("cannot pair %s: %v", sender, err)
	}
}

// startChannel attaches to the channel and hands back its inbound stream.
func startChannel(t *testing.T, channel *Channel) <-chan contract.Inbound {
	t.Helper()
	ctx, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)
	inbound, err := channel.Receive(ctx)
	if err != nil {
		t.Fatalf("cannot attach to the Signal channel: %v", err)
	}
	return inbound
}

// takeMessage waits for one inbound message and fails when none arrives.
func takeMessage(t *testing.T, inbound <-chan contract.Inbound) contract.Inbound {
	t.Helper()
	select {
	case message := <-inbound:
		return message
	case <-time.After(5 * time.Second):
		t.Fatalf("no message arrived on the Signal channel")
		return contract.Inbound{}
	}
}

func TestChannelIsNamedSignalAndCannotHideASecret(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	channel, _, _ := newTestChannel(t, baseOf(daemon), clock)

	if channel.Name() != SignalChannelName {
		t.Errorf("the channel is named %q, want %q", channel.Name(), SignalChannelName)
	}
	if _, err := channel.AskSecret(context.Background(), "your password"); !errors.Is(err, contract.ErrNoMaskedPrompt) {
		t.Errorf("asking for a secret over Signal gave %v, want ErrNoMaskedPrompt, because Signal cannot hide what is typed", err)
	}
}

func TestChannelCarriesAMessageInAndAReplyOutWithATypingIndicator(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	channel, pairing, _ := newTestChannel(t, baseOf(daemon), clock)
	pairSender(t, pairing, "+15125550123")

	inbound := startChannel(t, channel)
	waitFor(t, "the channel connects", func() bool { return channel.Connections() == 1 })
	daemon.PushMessage("+15125550123", "post the anniversary tweet")

	message := takeMessage(t, inbound)
	if message.Sender != "+15125550123" || message.Text != "post the anniversary tweet" {
		t.Errorf("the message that arrived is %+v, want the one that was sent", message)
	}
	if message.Channel != SignalChannelName {
		t.Errorf("the message says it came through %q, want %q", message.Channel, SignalChannelName)
	}

	if err := channel.Send(context.Background(), "Posted it."); err != nil {
		t.Fatalf("sending a reply failed: %v", err)
	}
	replies := repliesTo(daemon, "+15125550123")
	if len(replies) != 1 || replies[0] != "Posted it." {
		t.Fatalf("the sender got %v, want the one reply", replies)
	}
	if daemon.TypingIndicators() == 0 {
		t.Errorf("no typing indicator was shown while the reply was being written")
	}
}

func TestChannelGivesAnUnknownSenderOnlyAPairingCode(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	channel, _, _ := newTestChannel(t, baseOf(daemon), clock)

	inbound := startChannel(t, channel)
	waitFor(t, "the channel connects", func() bool { return channel.Connections() == 1 })
	daemon.PushMessage("+15125559999", "let me in")

	waitFor(t, "the stranger is answered", func() bool { return len(repliesTo(daemon, "+15125559999")) > 0 })
	answered := repliesTo(daemon, "+15125559999")
	if len(answered) != 1 {
		t.Fatalf("the stranger got %d messages, want only the pairing one", len(answered))
	}
	if !strings.Contains(answered[0], "/pair") {
		t.Errorf("the stranger was told %q, want the pairing message", answered[0])
	}

	select {
	case message := <-inbound:
		t.Errorf("a message from an unpaired sender reached the agent: %+v", message)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestChannelIgnoresAGroupMessage(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	channel, pairing, _ := newTestChannel(t, baseOf(daemon), clock)
	pairSender(t, pairing, "+15125550123")

	inbound := startChannel(t, channel)
	waitFor(t, "the channel connects", func() bool { return channel.Connections() == 1 })
	channel.route(context.Background(), Event{Sender: "+15125550123", Text: "in a group", GroupID: "dGhlIGdyb3Vw"})
	daemon.PushMessage("+15125550123", "on its own")

	message := takeMessage(t, inbound)
	if message.Text != "on its own" {
		t.Errorf("the message that arrived is %q, want the direct one, because a group reply would go to the wrong place", message.Text)
	}
}

func TestChannelSplitsALongReplyIntoSeveralMessages(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	channel, pairing, _ := newTestChannel(t, baseOf(daemon), clock)
	pairSender(t, pairing, "+15125550123")
	startChannel(t, channel)
	waitFor(t, "the channel connects", func() bool { return channel.Connections() == 1 })

	paragraph := strings.Repeat("word ", 160) + "end."
	reply := strings.Join([]string{paragraph, paragraph, paragraph}, "\n\n")
	if err := channel.Send(context.Background(), reply); err != nil {
		t.Fatalf("sending a long reply failed: %v", err)
	}

	sent := repliesTo(daemon, testAccount)
	if len(sent) != 2 {
		t.Fatalf("a long reply went out as %d messages, want two", len(sent))
	}
	for index, piece := range sent {
		if signalLength(piece) > MessageLimit {
			t.Errorf("message %d is %d units long and the limit is %d", index+1, signalLength(piece), MessageLimit)
		}
	}
}

func TestChannelHealthFollowsTheDaemon(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	channel, _, _ := newTestChannel(t, baseOf(daemon), clock)

	if health := channel.Health(context.Background()); !health.Healthy {
		t.Errorf("the channel says a running daemon is not healthy: %s", health.Detail)
	}
	daemon.Close()
	health := channel.Health(context.Background())
	if health.Healthy || health.Detail == "" {
		t.Errorf("the channel says a daemon that is gone is healthy, or does not say why not: %+v", health)
	}
}

func TestChannelPassesTheContractCheck(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	channel, pairing, _ := newTestChannel(t, baseOf(daemon), clock)
	pairSender(t, pairing, testAccount)

	// The check waits for an answer to its preview, so somebody has to answer it.
	// This runs beside the check rather than inside it, and says nothing when it
	// gives up, because only the test goroutine may end a test.
	answered := make(chan struct{})
	go func() {
		defer close(answered)
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if len(repliesTo(daemon, testAccount)) >= 3 {
				daemon.PushMessage(testAccount, "approve")
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	if err := testkit.CheckChannel(context.Background(), channel); err != nil {
		t.Fatalf("the Signal channel does not keep what every channel promises: %v", err)
	}
	<-answered
}
