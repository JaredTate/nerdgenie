package signal

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestPairingForgetsTheOldestRequestWhenItRemembersTooMany(t *testing.T) {
	pairing, _, _ := newTestPairing(t)

	for number := range MaxRememberedRequests + 5 {
		if _, _, err := pairing.Offer(fmt.Sprintf("+1512555%04d", number)); err != nil {
			t.Fatalf("offering to sender %d failed: %v", number, err)
		}
	}
	if len(pairing.state.LastRequest) > MaxRememberedRequests {
		t.Errorf("the store remembers %d senders, and the cap is %d", len(pairing.state.LastRequest), MaxRememberedRequests)
	}
}

func TestPairingRefusesToOfferACodeToNobody(t *testing.T) {
	pairing, _, _ := newTestPairing(t)
	if _, _, err := pairing.Offer(""); err == nil {
		t.Errorf("a code was offered to a sender with no name")
	}
}

func TestPairingRefusesAFileItCannotRead(t *testing.T) {
	home := testkit.NewTempHome(t)
	clock := testkit.NewFakeClock(time.Now())

	if err := os.WriteFile(codesFilePath(home.SignalFolder()), []byte("this is not JSON"), contract.SecretFileMode); err != nil {
		t.Fatalf("cannot write the broken file the test needs: %v", err)
	}
	if _, err := NewPairing(home, clock); err == nil {
		t.Errorf("the store opened a codes file it cannot read, which would quietly unpair everybody")
	}
}

func TestPairingRefusesAFilePastTheCap(t *testing.T) {
	home := testkit.NewTempHome(t)
	clock := testkit.NewFakeClock(time.Now())

	huge := make([]byte, maxStateFileBytes+1)
	for at := range huge {
		huge[at] = ' '
	}
	if err := os.WriteFile(approvedFilePath(home.SignalFolder()), huge, contract.SecretFileMode); err != nil {
		t.Fatalf("cannot write the oversized file the test needs: %v", err)
	}
	if _, err := NewPairing(home, clock); err == nil {
		t.Errorf("the store opened an approved file larger than any real one")
	}
}

func TestPairingSaysSoWhenItCannotWrite(t *testing.T) {
	pairing, _, home := newTestPairing(t)
	if err := os.Chmod(home.SignalFolder(), 0o500); err != nil {
		t.Fatalf("cannot make the Signal folder read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(home.SignalFolder(), contract.HomeFolderMode) })

	if _, _, err := pairing.Offer("+15125550123"); err == nil {
		t.Errorf("the store said it wrote a code down into a folder it cannot write to")
	}
}

func TestNewPairingSaysSoWhenItCannotMakeItsFolder(t *testing.T) {
	folder := t.TempDir()
	blocked := filepath.Join(folder, "blocked")
	if err := os.WriteFile(blocked, []byte("not a folder"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file in the way: %v", err)
	}
	if _, err := NewPairing(contract.NewHome(blocked), testkit.NewFakeClock(time.Now())); err == nil {
		t.Errorf("the store made its folder where a file already is")
	}
}

func TestNewChannelRefusesWhatItCannotWorkWith(t *testing.T) {
	home := testkit.NewTempHome(t)
	clock := testkit.NewFakeClock(time.Now())
	cases := []struct {
		name    string
		options ChannelOptions
	}{
		{"no account", ChannelOptions{Home: home, Clock: clock, Secrets: testkit.NewFakeSecrets()}},
		{"no clock", ChannelOptions{Account: testAccount, Home: home, Secrets: testkit.NewFakeSecrets()}},
		{"no vault", ChannelOptions{Account: testAccount, Home: home, Clock: clock}},
		{"a program with no account", ChannelOptions{Account: testAccount, Program: "signal-cli", Home: home, Clock: clock, Secrets: testkit.NewFakeSecrets(), Host: "127.0.0.1", Port: 1}},
	}
	for _, oneCase := range cases[:3] {
		t.Run(oneCase.name, func(t *testing.T) {
			if _, err := NewChannel(oneCase.options); err == nil {
				t.Errorf("the channel was built with %s", oneCase.name)
			}
		})
	}
	if _, err := NewChannel(cases[3].options); err != nil {
		t.Errorf("the channel refused to look after a daemon it could look after: %v", err)
	}
}

func TestNewClientRefusesWhatItCannotWorkWith(t *testing.T) {
	home := testkit.NewTempHome(t)
	if _, err := NewClient(ClientOptions{Home: home, Secrets: testkit.NewFakeSecrets()}); err == nil {
		t.Errorf("the client was built with no daemon address")
	}
	if _, err := NewClient(ClientOptions{BaseAddress: "http://127.0.0.1:1", Home: home}); err == nil {
		t.Errorf("the client was built with no vault, and every message goes through the redactor")
	}
}

func TestClientRefusesADownloadItCannotMake(t *testing.T) {
	client, _ := newTestClient(t, "http://127.0.0.1:1")
	if _, err := client.Download(context.Background(), Attachment{}, "+1"); err == nil {
		t.Errorf("the client downloaded an attachment with no identifier")
	}
	if _, err := client.Download(context.Background(), Attachment{ID: "big", Size: maxAttachmentBytes + 1}, "+1"); err == nil {
		t.Errorf("the client downloaded an attachment larger than Signal allows")
	}
	if err := client.Typing(context.Background(), "", false); err == nil {
		t.Errorf("the client showed a typing indicator to nobody")
	}
}

func TestClientTakesAnAnswerWithNoBodyAsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	client, _ := newTestClient(t, server.URL)

	if err := client.Send(context.Background(), "+15125550123", "hello", nil); err != nil {
		t.Errorf("the client read a 201 with no body as a failure, and signal-cli answers that way when it has nothing to say: %v", err)
	}
}

func TestClientSaysSoWhenTheDaemonAnswersWithNothing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client, _ := newTestClient(t, server.URL)

	if err := client.Send(context.Background(), "+15125550123", "hello", nil); err == nil {
		t.Errorf("the client read an empty answer as a success")
	}
}

func TestClientSaysSoWhenTheDaemonAnswersWithSomethingElse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(writer, "this is not JSON at all")
	}))
	defer server.Close()
	client, _ := newTestClient(t, server.URL)

	if err := client.Send(context.Background(), "+15125550123", "hello", nil); err == nil {
		t.Errorf("the client read something that is not JSON as a success")
	}
}

func TestClientSaysSoWhenTheHealthCheckAnswersBadly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "not today", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client, _ := newTestClient(t, server.URL)

	health := client.Health(context.Background())
	if health.Healthy || !strings.Contains(health.Detail, "503") {
		t.Errorf("the health check came back as %+v, want it unhealthy and naming what the daemon said", health)
	}
}

func TestStreamSaysSoWhenTheDaemonRefusesTheEventStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "no stream for you", http.StatusForbidden)
	}))
	defer server.Close()
	clock := testkit.NewFakeClock(time.Now())
	stream, _ := runStream(t, server.URL, clock)

	waitFor(t, "the stream waits before trying again", func() bool { return clock.Sleepers() > 0 })
	if stream.Connections() != 0 {
		t.Errorf("the stream says it connected %d times to a daemon that refused it", stream.Connections())
	}
}

func TestInboxNameAlwaysMakesAName(t *testing.T) {
	cases := []struct {
		name       string
		attachment Attachment
		wants      string
	}{
		{"a readable filename", Attachment{ID: "abc", Filename: "photo.jpg"}, "signal-7-photo.jpg"},
		{"no filename, so the identifier", Attachment{ID: "abc123"}, "signal-7-abc123"},
		{"nothing at all", Attachment{}, "signal-7-attachment"},
		{"a name too long to use whole", Attachment{Filename: strings.Repeat("a", 200) + ".png"}, "signal-7-" + strings.Repeat("a", 60) + ".png"},
		{"a name that is only punctuation", Attachment{Filename: "..", ID: "//"}, "signal-7-attachment"},
	}
	for _, oneCase := range cases {
		t.Run(oneCase.name, func(t *testing.T) {
			got := inboxName(Event{Timestamp: 7}, oneCase.attachment)
			if got != oneCase.wants {
				t.Errorf("the attachment is called %q, want %q", got, oneCase.wants)
			}
		})
	}
}

func TestCopyIntoInboxSaysSoWhenItCannotRead(t *testing.T) {
	inbox := t.TempDir()
	if _, err := copyIntoInbox(filepath.Join(t.TempDir(), "not-there"), inbox, "name"); err == nil {
		t.Errorf("a file that is not there was copied into the inbox")
	}
}

func TestReceivedAtUsesTheMachinesTimeWhenTheDaemonGaveNone(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	clock := testkit.NewFakeClock(now)
	if got := receivedAt(Event{}, clock); !got.Equal(now) {
		t.Errorf("a message with no time was received at %v, want the machine's own time %v", got, now)
	}
	if got := receivedAt(Event{Timestamp: 1700000000000}, clock); got.UnixMilli() != 1700000000000 {
		t.Errorf("a message with a time was received at %v, want the time Signal gave", got)
	}
}

func TestLinkEndedErrorSaysWhichThingWentWrong(t *testing.T) {
	if !strings.Contains(linkEndedError(true).Error(), "more quickly") {
		t.Errorf("a link that was drawn and then ended says %q, want it to say to try again more quickly", linkEndedError(true))
	}
	if !strings.Contains(linkEndedError(false).Error(), "no linking address") {
		t.Errorf("a link that printed no address says %q, want it to say so", linkEndedError(false))
	}
}
