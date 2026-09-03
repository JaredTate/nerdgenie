package testkit_test

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/testkit"
)

func TestTheFakeSignalDaemonAnswersItsHealthCheck(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()

	answer, err := http.Get(daemon.HealthAddress())
	if err != nil {
		t.Fatalf("asking the fake daemon whether it is healthy failed: %v", err)
	}
	defer answer.Body.Close()
	if answer.StatusCode != http.StatusOK {
		t.Errorf("the health check answered %d, want 200", answer.StatusCode)
	}
}

func TestTheFakeSignalDaemonStreamsAMessageAndAnAttachment(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()

	lines := openEventStream(t, daemon.EventsAddress())
	pushMessage(t, daemon, "post the anniversary tweet")
	pushMessage(t, daemon, "here is the picture", "/tmp/inbox/photo.jpg")

	first := readEventLine(t, lines)
	if !strings.Contains(first, "post the anniversary tweet") {
		t.Errorf("the first event is %q, want the message that was pushed", first)
	}
	second := readEventLine(t, lines)
	if !strings.Contains(second, "photo.jpg") {
		t.Errorf("the second event is %q, want the attachment that was pushed", second)
	}
}

func TestTheFakeSignalDaemonRecordsSendsAndTypingIndicators(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()

	post(t, daemon.RemoteProcedureAddress(), `{"jsonrpc":"2.0","id":1,"method":"send","params":{"recipient":["+15555550123"],"message":"the post is up"}}`)
	post(t, daemon.RemoteProcedureAddress(), `{"jsonrpc":"2.0","id":2,"method":"sendTyping","params":{"recipient":["+15555550123"]}}`)

	sends := daemon.Sends()
	if len(sends) != 1 {
		t.Fatalf("the daemon recorded %d sends, want 1", len(sends))
	}
	if sends[0].Message != "the post is up" {
		t.Errorf("the recorded send says %q, want the message that was sent", sends[0].Message)
	}
	if daemon.TypingIndicators() != 1 {
		t.Errorf("the daemon recorded %d typing indicators, want 1", daemon.TypingIndicators())
	}
}

func TestTheFakeSignalDaemonCanDropItsStream(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()

	lines := openEventStream(t, daemon.EventsAddress())
	daemon.DropStream()

	deadline := time.After(2 * time.Second)
	done := make(chan struct{})
	go func() {
		for lines.Scan() {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-deadline:
		t.Fatal("the stream stayed open after it was dropped")
	}
}

// drainStream reads a scanner until it ends, which is how a test waits for the
// server to let go of a connection it dropped.
func drainStream(t *testing.T, lines *bufio.Scanner) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		for lines.Scan() {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the stream stayed open two seconds after it was dropped")
	}
}

func TestADroppedStreamLetsTheNextConnectionSucceed(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()

	first := openEventStream(t, daemon.EventsAddress())
	daemon.DropStream()
	drainStream(t, first)

	second := openEventStream(t, daemon.EventsAddress())
	pushMessage(t, daemon, "the harness reconnected")

	line := readEventLine(t, second)
	if !strings.Contains(line, "the harness reconnected") {
		t.Errorf("the stream after the drop carried %q, want the message pushed after it", line)
	}
}

func TestAStalledStreamGoesQuietAfterItHasSentSomething(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	daemon.StallStream(500 * time.Millisecond)
	pushMessage(t, daemon, "the first message")
	pushMessage(t, daemon, "the second message")

	lines := openEventStream(t, daemon.EventsAddress())
	started := time.Now()
	first := readEventLine(t, lines)
	untilFirst := time.Since(started)
	second := readEventLine(t, lines)
	untilSecond := time.Since(started)

	if !strings.Contains(first, "the first message") || !strings.Contains(second, "the second message") {
		t.Fatalf("the stream carried %q then %q, want both messages in order", first, second)
	}
	if untilFirst > 300*time.Millisecond {
		t.Errorf("the first event took %s, and a stall models a stream that goes quiet after some bytes, not a connect that hangs", untilFirst)
	}
	if untilSecond < 400*time.Millisecond {
		t.Errorf("the second event took %s, and the stream was told to go quiet for half a second", untilSecond)
	}
}

func TestAStallCoversTheCurrentStreamOnly(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	daemon.StallStream(5 * time.Second)
	pushMessage(t, daemon, "the first message")
	pushMessage(t, daemon, "the second message")

	stalled := openEventStream(t, daemon.EventsAddress())
	readEventLine(t, stalled)
	daemon.DropStream()
	drainStream(t, stalled)

	fresh := openEventStream(t, daemon.EventsAddress())
	started := time.Now()
	line := readEventLine(t, fresh)

	if !strings.Contains(line, "the second message") {
		t.Errorf("the new stream carried %q, want the message the stalled one never sent", line)
	}
	if took := time.Since(started); took > time.Second {
		t.Errorf("the new connection waited %s, and a stall covers the stream it was set on and no other", took)
	}
}

func TestAnAttachmentIsAnOpaqueIdTheDaemonServes(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	path := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(path, []byte("the picture the user sent"), 0o644); err != nil {
		t.Fatalf("cannot write the fixture attachment: %v", err)
	}

	lines := openEventStream(t, daemon.EventsAddress())
	pushMessage(t, daemon, "here is the picture", path)
	line := readEventLine(t, lines)

	identifier := attachmentIDIn(t, line)
	if identifier == path {
		t.Errorf("the attachment id is the local path %q, and signal-cli hands back an opaque id the harness has to download", identifier)
	}
	if strings.Contains(line, path) {
		t.Errorf("the event carries the local path:\n%s", line)
	}
	if !strings.Contains(line, "photo.jpg") {
		t.Errorf("the event carries no readable filename:\n%s", line)
	}

	// signal-cli hands the bytes back through the getAttachment call on the same
	// remote-procedure endpoint, as base64 under "data". There is no separate
	// address to fetch, so a harness reads an attachment the one way it reads
	// everything else.
	answered := callSignal(t, daemon.RemoteProcedureAddress(),
		`{"jsonrpc":"2.0","id":1,"method":"getAttachment","params":{"id":"`+identifier+`","recipient":"+15555550123"}}`)

	result, isResult := answered["result"].(map[string]any)
	if !isResult {
		t.Fatalf("getAttachment came back with no result: %+v", answered)
	}
	encoded, isText := result["data"].(string)
	if !isText {
		t.Fatalf("getAttachment came back with no data: %+v", result)
	}
	content, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("the data getAttachment gave back is not base64: %v", err)
	}
	if string(content) != "the picture the user sent" {
		t.Errorf("getAttachment gave back %q, want the bytes that were pushed", content)
	}

	missing := callSignal(t, daemon.RemoteProcedureAddress(),
		`{"jsonrpc":"2.0","id":2,"method":"getAttachment","params":{"id":"attachment-nobody-pushed","recipient":"+15555550123"}}`)
	if _, isFailure := missing["error"]; !isFailure {
		t.Errorf("asking for an attachment nobody pushed came back as a success: %+v", missing)
	}
}

// callSignal sends one remote-procedure call to the fake daemon and returns the
// answer it gave.
func callSignal(t *testing.T, address string, body string) map[string]any {
	t.Helper()
	answer, err := http.Post(address, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("calling %s failed: %v", address, err)
	}
	defer answer.Body.Close()
	read, err := io.ReadAll(answer.Body)
	if err != nil {
		t.Fatalf("reading the answer from %s failed: %v", address, err)
	}
	var given map[string]any
	if err := json.Unmarshal(read, &given); err != nil {
		t.Fatalf("the answer from %s is not JSON: %v\n%s", address, err, read)
	}
	return given
}

// attachmentIDIn reads the id of the first attachment out of one event line.
func attachmentIDIn(t *testing.T, line string) string {
	t.Helper()
	body, found := strings.CutPrefix(line, "data: ")
	if !found {
		t.Fatalf("that is not an event line: %s", line)
	}
	var event struct {
		Envelope struct {
			DataMessage struct {
				Attachments []struct {
					ID       string `json:"id"`
					Filename string `json:"filename"`
				} `json:"attachments"`
			} `json:"dataMessage"`
		} `json:"envelope"`
	}
	if err := json.Unmarshal([]byte(body), &event); err != nil {
		t.Fatalf("the event is not JSON: %v\n%s", err, body)
	}
	if len(event.Envelope.DataMessage.Attachments) == 0 {
		t.Fatalf("the event carries no attachments:\n%s", body)
	}
	return event.Envelope.DataMessage.Attachments[0].ID
}

func TestTheFakeSignalProgramPrintsALinkAndThenSaysItIsAssociated(t *testing.T) {
	folder := testkit.WriteFakeSignalProgram(t, "sgnl://linkdevice?uuid=fixture&pub_key=fixture")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(folder, "signal-cli"), "link", "-n", "coeus")
	printed, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("the fake signal-cli program failed: %v\n%s", err, printed)
	}

	text := string(printed)
	if !strings.Contains(text, "sgnl://linkdevice") {
		t.Errorf("the program printed %q, want the linking address in it", text)
	}
	if !strings.Contains(text, "Associated") {
		t.Errorf("the program printed %q, want it to say the device was associated", text)
	}
}

func TestPushingIntoAFullEventQueueSaysSoRatherThanBlocking(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()

	overflowed := make(chan error, 1)
	go func() {
		for range 1000 {
			if err := daemon.PushMessage("+15555550123", "nobody is reading this"); err != nil {
				overflowed <- err
				return
			}
		}
		overflowed <- nil
	}()

	select {
	case err := <-overflowed:
		if err == nil {
			t.Fatal("a thousand events went onto a stream nobody was reading, and the queue is supposed to be capped")
		}
		if !strings.Contains(err.Error(), "read") {
			t.Errorf("the overflow error does not say what to do about it: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pushing onto a full event queue blocked the test rather than reporting the overflow")
	}
}

// pushMessage puts one message on the fake daemon's stream and fails the test
// when the queue will not take it.
func pushMessage(t *testing.T, daemon *testkit.FakeSignalCLI, text string, attachments ...string) {
	t.Helper()
	if err := daemon.PushMessage("+15555550123", text, attachments...); err != nil {
		t.Fatalf("pushing %q onto the stream failed: %v", text, err)
	}
}

// openEventStream attaches to the fake daemon's event stream and returns a
// scanner over its lines.
func openEventStream(t *testing.T, address string) *bufio.Scanner {
	t.Helper()
	answer, err := http.Get(address)
	if err != nil {
		t.Fatalf("attaching to the event stream failed: %v", err)
	}
	t.Cleanup(func() { answer.Body.Close() })
	return bufio.NewScanner(answer.Body)
}

// readEventLine reads until it finds a data line, so that the blank lines
// between events do not confuse a test.
func readEventLine(t *testing.T, lines *bufio.Scanner) string {
	t.Helper()
	for range 20 {
		if !lines.Scan() {
			t.Fatal("the event stream ended before an event arrived")
		}
		if strings.HasPrefix(lines.Text(), "data: ") {
			return lines.Text()
		}
	}
	t.Fatal("twenty lines went by with no event in them")
	return ""
}

// post sends one JSON body and fails the test when it cannot.
func post(t *testing.T, address string, body string) {
	t.Helper()
	answer, err := http.Post(address, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("posting to %s failed: %v", address, err)
	}
	defer answer.Body.Close()
	if answer.StatusCode != http.StatusOK {
		t.Fatalf("posting to %s answered %d, want 200", address, answer.StatusCode)
	}
}
