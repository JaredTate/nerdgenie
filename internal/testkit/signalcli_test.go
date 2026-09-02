package testkit_test

import (
	"bufio"
	"context"
	"net/http"
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
	daemon.PushMessage("+15555550123", "post the anniversary tweet")
	daemon.PushMessage("+15555550123", "here is the picture", "/tmp/inbox/photo.jpg")

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
