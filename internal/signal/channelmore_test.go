package signal

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// baseOf is the address the fake daemon serves on, without any path.
func baseOf(daemon *testkit.FakeSignalCLI) string {
	return strings.TrimSuffix(daemon.HealthAddress(), testkit.SignalHealthPath)
}

// repliesTo is every message the harness sent to one recipient, in order.
func repliesTo(daemon *testkit.FakeSignalCLI, recipient string) []string {
	found := []string{}
	for _, one := range daemon.Sends() {
		for _, who := range one.Recipients {
			if who == recipient {
				found = append(found, one.Message)
			}
		}
	}
	return found
}

// withAttachments puts a small server in front of the fake daemon that answers
// the getAttachment call with the bytes given and passes everything else
// through. signal-cli hands attachment bytes back as base64 from its
// remote-procedure endpoint, and the fake in testkit answers every call with a
// timestamp instead, so this fills in the one call the fake cannot make.
func withAttachments(t *testing.T, daemon *testkit.FakeSignalCLI, bytesByID map[string][]byte) string {
	t.Helper()
	target, err := url.Parse(baseOf(daemon))
	if err != nil {
		t.Fatalf("cannot read the fake daemon's address: %v", err)
	}
	forward := httputil.NewSingleHostReverseProxy(target)

	router := http.NewServeMux()
	router.Handle(eventsPath, forward)
	router.Handle(healthPath, forward)
	router.HandleFunc(remoteProcedurePath, func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			http.Error(writer, "unreadable", http.StatusBadRequest)
			return
		}
		var call struct {
			Method string `json:"method"`
			Params struct {
				ID string `json:"id"`
			} `json:"params"`
		}
		_ = json.Unmarshal(body, &call)
		if call.Method == "getAttachment" {
			content, known := bytesByID[call.Params.ID]
			if !known {
				writeJSON(writer, map[string]any{"error": map[string]any{"code": -1, "message": "no such attachment here"}})
				return
			}
			writeJSON(writer, map[string]any{"result": map[string]any{"data": base64.StdEncoding.EncodeToString(content)}})
			return
		}
		request.Body = io.NopCloser(bytes.NewReader(body))
		request.ContentLength = int64(len(body))
		forward.ServeHTTP(writer, request)
	})

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server.URL
}

func TestChannelShowsAPreviewAndTakesTheAnswer(t *testing.T) {
	cases := []struct {
		typed string
		want  contract.PreviewAnswer
	}{
		{"approve", contract.AnswerOnce},
		{"always", contract.AnswerAlways},
		{"deny", contract.AnswerReject},
		{"  APPROVE  ", contract.AnswerOnce},
	}
	for _, oneCase := range cases {
		t.Run(oneCase.typed, func(t *testing.T) {
			daemon := testkit.NewFakeSignalCLI()
			defer daemon.Close()
			clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
			channel, pairing, _ := newTestChannel(t, baseOf(daemon), clock)
			pairSender(t, pairing, "+15125550123")
			startChannel(t, channel)
			waitFor(t, "the channel connects", func() bool { return channel.Connections() == 1 })
			daemon.PushMessage("+15125550123", "do it")
			waitForInbound(t, channel)

			answered := make(chan contract.PreviewAnswer, 1)
			go func() {
				answer, err := channel.ShowPreview(context.Background(), contract.Preview{
					ID: "3", Title: "Run a command", Body: "rm -rf /tmp/old",
				})
				if err == nil {
					answered <- answer
				}
			}()

			waitFor(t, "the preview is shown", func() bool { return len(repliesTo(daemon, "+15125550123")) >= 1 })
			shown := repliesTo(daemon, "+15125550123")[0]
			if !strings.Contains(shown, "rm -rf /tmp/old") {
				t.Errorf("the preview shown is %q, want the actual text of what is about to happen", shown)
			}
			for _, word := range []string{"approve", "always", "deny"} {
				if !strings.Contains(shown, word) {
					t.Errorf("the preview shown is %q, want it to explain %q", shown, word)
				}
			}
			daemon.PushMessage("+15125550123", oneCase.typed)

			select {
			case answer := <-answered:
				if answer != oneCase.want {
					t.Errorf("the answer to %q came back as %q, want %q", oneCase.typed, answer, oneCase.want)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("the preview was never answered")
			}
		})
	}
}

func TestChannelGivesUpOnAPreviewNobodyAnswers(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	channel, pairing, _ := newTestChannel(t, baseOf(daemon), clock)
	pairSender(t, pairing, "+15125550123")
	startChannel(t, channel)
	waitFor(t, "the channel connects", func() bool { return channel.Connections() == 1 })

	failed := make(chan error, 1)
	go func() {
		_, err := channel.ShowPreview(context.Background(), contract.Preview{ID: "3", Title: "Run", Body: "rm -rf /tmp/old"})
		failed <- err
	}()

	waitFor(t, "the preview waits for an answer", func() bool { return clock.Sleepers() > 0 })
	clock.Advance(PreviewTimeout)

	select {
	case err := <-failed:
		if err == nil {
			t.Fatalf("the preview said it was answered when nobody answered it")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("the preview never gave up waiting")
	}
}

func TestChannelSaysItIsStillWorkingOnceAfterFiveQuietMinutes(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	channel, pairing, _ := newTestChannel(t, baseOf(daemon), clock)
	pairSender(t, pairing, "+15125550123")
	startChannel(t, channel)
	waitFor(t, "the channel connects", func() bool { return channel.Connections() == 1 })

	daemon.PushMessage("+15125550123", "do the long thing")
	waitForInbound(t, channel)

	clock.Advance(StillWorkingAfter)
	waitFor(t, "the note is sent", func() bool { return len(repliesTo(daemon, "+15125550123")) == 1 })
	if !strings.Contains(repliesTo(daemon, "+15125550123")[0], "still working") {
		t.Errorf("the note says %q, want it to say the agent is still working", repliesTo(daemon, "+15125550123")[0])
	}

	clock.Advance(3 * StillWorkingAfter)
	time.Sleep(50 * time.Millisecond)
	if sent := repliesTo(daemon, "+15125550123"); len(sent) != 1 {
		t.Errorf("the note was sent %d times, want once for one task", len(sent))
	}
}

func TestChannelPutsAnInboundPhotoInTheInboxAndOnTheMessage(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	photo := []byte("these are the bytes of the photo")
	address := withAttachments(t, daemon, map[string][]byte{"attachment-1": photo})

	channel, pairing, home := newTestChannel(t, address, clock)
	pairSender(t, pairing, "+15125550123")
	inbound := startChannel(t, channel)
	waitFor(t, "the channel connects", func() bool { return channel.Connections() == 1 })

	channel.route(context.Background(), Event{
		Sender:      "+15125550123",
		Text:        "look at this",
		Timestamp:   1700000000000,
		Attachments: []Attachment{{ID: "attachment-1", Filename: "photo.jpg", ContentType: "image/jpeg"}},
	})

	message := takeMessage(t, inbound)
	if len(message.Attachments) != 1 {
		t.Fatalf("the message carries %d attachments, want the one photo", len(message.Attachments))
	}
	landed := message.Attachments[0]
	if filepath.Dir(landed) != home.InboxFolder() {
		t.Errorf("the photo landed at %q, want it in the inbox %q", landed, home.InboxFolder())
	}
	written, err := os.ReadFile(landed)
	if err != nil {
		t.Fatalf("cannot read the photo in the inbox: %v", err)
	}
	if string(written) != string(photo) {
		t.Errorf("the photo in the inbox holds %q, want the bytes the daemon sent", written)
	}
}

func TestChannelSendsAFileTheWayAHandoffArrives(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	channel, pairing, _ := newTestChannel(t, baseOf(daemon), clock)
	pairSender(t, pairing, "+15125550123")
	startChannel(t, channel)
	waitFor(t, "the channel connects", func() bool { return channel.Connections() == 1 })

	if err := channel.SendFile(context.Background(), "/tmp/screen.png", "a login page appeared"); err != nil {
		t.Fatalf("sending a file failed: %v", err)
	}
	sends := daemon.Sends()
	if len(sends) != 1 {
		t.Fatalf("the daemon saw %d sends, want the one file", len(sends))
	}
	if len(sends[0].Attachments) != 1 || sends[0].Attachments[0] != "/tmp/screen.png" {
		t.Errorf("the file sent was %v, want the screenshot", sends[0].Attachments)
	}
	if sends[0].Message != "a login page appeared" {
		t.Errorf("the words with the file are %q, want the caption", sends[0].Message)
	}
}

// waitForInbound waits until the channel has handed one message on, which is how
// a test knows a task has started without reading the message itself.
func waitForInbound(t *testing.T, channel *Channel) {
	t.Helper()
	waitFor(t, "a message reaches the agent", func() bool { return channel.Delivered() > 0 })
}
