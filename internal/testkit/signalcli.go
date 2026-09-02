// The shapes here were read from OpenClaw's Signal extension, at
// ~/Code/openclaw/extensions/signal/src/daemon.ts and
// ~/Code/openclaw/extensions/signal/src/client.ts, which is where the three
// paths and the event envelope come from. Nothing was copied: that code is
// TypeScript and talks to a real signal-cli, and this is Go and pretends to be
// one.

package testkit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// The three paths signal-cli serves when it is started with --http.
const (
	// SignalHealthPath is the health check.
	SignalHealthPath = "/api/v1/check"
	// SignalEventsPath is the stream of inbound messages.
	SignalEventsPath = "/api/v1/events"
	// SignalRemoteProcedurePath is where sends and typing indicators go.
	SignalRemoteProcedurePath = "/api/v1/rpc"
	// SignalAttachmentsPath is where the daemon serves the bytes of one
	// attachment, named by the opaque id it put on the event.
	SignalAttachmentsPath = "/api/v1/attachments/"
)

// SignalSend is one message the harness asked the daemon to send.
type SignalSend struct {
	// Recipients are who it went to.
	Recipients []string
	// Message is what was sent.
	Message string
	// Attachments are the files that went with it.
	Attachments []string
}

// FakeSignalCLI is a loopback server shaped like signal-cli started with --http:
// a health check, a stream of inbound events a test pushes into, and a
// remote-procedure endpoint that records what the harness sent.
type FakeSignalCLI struct {
	server *httptest.Server

	guard          sync.Mutex
	events         chan string
	dropped        chan struct{}
	stopped        chan struct{}
	stall          time.Duration
	sends          []SignalSend
	typings        int
	nextID         int
	attachments    map[string]string
	nextAttachment int
	isClosed       bool
}

// NewFakeSignalCLI starts the fake daemon. Close it when the test is done.
func NewFakeSignalCLI() *FakeSignalCLI {
	daemon := &FakeSignalCLI{
		events:      make(chan string, inboundQueueSize),
		dropped:     make(chan struct{}),
		stopped:     make(chan struct{}),
		attachments: map[string]string{},
	}
	router := http.NewServeMux()
	router.HandleFunc(SignalHealthPath, daemon.handleHealth)
	router.HandleFunc(SignalEventsPath, daemon.handleEvents)
	router.HandleFunc(SignalRemoteProcedurePath, daemon.handleRemoteProcedure)
	router.HandleFunc(SignalAttachmentsPath, daemon.handleAttachment)
	daemon.server = httptest.NewServer(router)
	return daemon
}

// HealthAddress is the full address of the health check.
func (daemon *FakeSignalCLI) HealthAddress() string {
	return daemon.server.URL + SignalHealthPath
}

// EventsAddress is the full address of the event stream.
func (daemon *FakeSignalCLI) EventsAddress() string {
	return daemon.server.URL + SignalEventsPath
}

// RemoteProcedureAddress is the full address sends and typing indicators go to.
func (daemon *FakeSignalCLI) RemoteProcedureAddress() string {
	return daemon.server.URL + SignalRemoteProcedurePath
}

// AttachmentAddress is the full address of one attachment's bytes, by the id the
// daemon put on the event.
func (daemon *FakeSignalCLI) AttachmentAddress(identifier string) string {
	return daemon.server.URL + SignalAttachmentsPath + url.PathEscape(identifier)
}

// PushMessage puts one inbound message on the stream, with any attachments, and
// reports the overflow rather than blocking when the queue is full and nothing
// is reading it.
func (daemon *FakeSignalCLI) PushMessage(sender string, text string, attachments ...string) error {
	daemon.guard.Lock()
	daemon.nextID++
	number := daemon.nextID
	listed := daemon.attachmentList(attachments)
	daemon.guard.Unlock()

	event := map[string]any{
		"envelope": map[string]any{
			"source":    sender,
			"timestamp": number,
			"dataMessage": map[string]any{
				"message":     text,
				"attachments": listed,
			},
		},
	}
	select {
	case daemon.events <- fmt.Sprintf("data: %s\n\n", mustJSON(event)):
		return nil
	default:
		return fmt.Errorf("the fake signal-cli daemon is already holding %d events nobody has read, so attach to the stream before pushing more",
			inboundQueueSize)
	}
}

// Sends is every message the harness asked the daemon to send, in order.
func (daemon *FakeSignalCLI) Sends() []SignalSend {
	daemon.guard.Lock()
	defer daemon.guard.Unlock()
	copied := make([]SignalSend, len(daemon.sends))
	copy(copied, daemon.sends)
	return copied
}

// TypingIndicators is how many typing indicators the harness sent.
func (daemon *FakeSignalCLI) TypingIndicators() int {
	daemon.guard.Lock()
	defer daemon.guard.Unlock()
	return daemon.typings
}

// DropStream cuts the stream the daemon is serving now, the way a daemon that
// died does, and leaves the daemon ready for the next connection, so that a test
// can prove the harness reconnects and picks up where it left off.
func (daemon *FakeSignalCLI) DropStream() {
	daemon.guard.Lock()
	defer daemon.guard.Unlock()
	close(daemon.dropped)
	daemon.dropped = make(chan struct{})
}

// StallStream makes the next stream go quiet for a while after it has sent
// something, which is what a daemon that stops producing looks like and what the
// harness's forced reconnect exists for. It covers one stream: the connection
// after it starts fresh.
func (daemon *FakeSignalCLI) StallStream(wait time.Duration) {
	daemon.guard.Lock()
	defer daemon.guard.Unlock()
	daemon.stall = wait
}

// Close shuts the fake daemon down for good.
func (daemon *FakeSignalCLI) Close() {
	daemon.guard.Lock()
	if !daemon.isClosed {
		daemon.isClosed = true
		close(daemon.stopped)
	}
	daemon.guard.Unlock()
	daemon.server.Close()
}

// streamSignals is the drop channel for the stream about to be served and the
// one that says the whole daemon is going away.
func (daemon *FakeSignalCLI) streamSignals() (<-chan struct{}, <-chan struct{}) {
	daemon.guard.Lock()
	defer daemon.guard.Unlock()
	return daemon.dropped, daemon.stopped
}

// takeStall takes the stall set for the next stream and leaves the daemon
// behaving well, so that a stall covers one stream and no other.
func (daemon *FakeSignalCLI) takeStall() time.Duration {
	daemon.guard.Lock()
	defer daemon.guard.Unlock()
	stall := daemon.stall
	daemon.stall = 0
	return stall
}

// handleHealth answers the health check.
func (daemon *FakeSignalCLI) handleHealth(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	fmt.Fprint(writer, `{"status":"ok"}`)
}

// handleEvents streams the messages a test pushed until the stream is dropped or
// the caller goes away.
func (daemon *FakeSignalCLI) handleEvents(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.WriteHeader(http.StatusOK)
	flush, canFlush := writer.(http.Flusher)
	if canFlush {
		flush.Flush()
	}

	dropped, stopped := daemon.streamSignals()
	stall := daemon.takeStall()

	for {
		select {
		case event := <-daemon.events:
			fmt.Fprint(writer, event)
			if canFlush {
				flush.Flush()
			}
			if stall > 0 {
				if !waitOrGiveUp(stall, dropped, stopped, request.Context().Done()) {
					return
				}
				stall = 0
			}
		case <-dropped:
			return
		case <-stopped:
			return
		case <-request.Context().Done():
			return
		}
	}
}

// waitOrGiveUp goes quiet for the wait, and says whether the stream is still
// worth carrying on with afterwards.
func waitOrGiveUp(wait time.Duration, dropped <-chan struct{}, stopped <-chan struct{}, gone <-chan struct{}) bool {
	select {
	case <-time.After(wait):
		return true
	case <-dropped:
		return false
	case <-stopped:
		return false
	case <-gone:
		return false
	}
}

// handleAttachment serves the bytes of one attachment by its opaque id, the way
// signal-cli does.
func (daemon *FakeSignalCLI) handleAttachment(writer http.ResponseWriter, request *http.Request) {
	identifier := strings.TrimPrefix(request.URL.Path, SignalAttachmentsPath)
	daemon.guard.Lock()
	path, known := daemon.attachments[identifier]
	daemon.guard.Unlock()

	if !known {
		http.Error(writer, `{"error":{"message":"there is no attachment with that id, so use the id from the event"}}`, http.StatusNotFound)
		return
	}
	content, err := os.ReadFile(path)
	if err != nil {
		http.Error(writer, `{"error":{"message":"that attachment is not on the disk, so write the file before pushing the message"}}`, http.StatusNotFound)
		return
	}
	writer.Header().Set("Content-Type", "application/octet-stream")
	_, _ = writer.Write(content)
}

// handleRemoteProcedure records a send or a typing indicator.
func (daemon *FakeSignalCLI) handleRemoteProcedure(writer http.ResponseWriter, request *http.Request) {
	var call struct {
		Method string `json:"method"`
		Params struct {
			Recipient   []string `json:"recipient"`
			Message     string   `json:"message"`
			Attachments []string `json:"attachments"`
		} `json:"params"`
	}
	if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
		http.Error(writer, `{"error":{"message":"the call was not JSON"}}`, http.StatusBadRequest)
		return
	}

	daemon.guard.Lock()
	switch call.Method {
	case "send":
		daemon.sends = append(daemon.sends, SignalSend{
			Recipients:  call.Params.Recipient,
			Message:     call.Params.Message,
			Attachments: call.Params.Attachments,
		})
	case "sendTyping":
		daemon.typings++
	}
	daemon.guard.Unlock()

	writer.Header().Set("Content-Type", "application/json")
	fmt.Fprint(writer, `{"jsonrpc":"2.0","id":1,"result":{"timestamp":1}}`)
}

// attachmentList turns the paths into the shape signal-cli reports them in: a
// filename a person can read and an opaque id, never a path. signal-cli hands
// the harness an id and serves the bytes at its attachment endpoint, and a fake
// that handed back a local path would let a harness read the file straight off
// the disk and never exercise the download at all. The caller holds the lock.
func (daemon *FakeSignalCLI) attachmentList(paths []string) []map[string]any {
	listed := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		daemon.nextAttachment++
		identifier := fmt.Sprintf("attachment-%d", daemon.nextAttachment)
		daemon.attachments[identifier] = path
		listed = append(listed, map[string]any{
			"filename":    filepath.Base(path),
			"id":          identifier,
			"contentType": "application/octet-stream",
		})
	}
	return listed
}

// WriteFakeSignalProgram writes a program named signal-cli into a new folder and
// returns the folder, so that a test can put it at the front of the PATH and
// drive the linking flow without a real signal-cli. The program prints the
// linking address, waits a moment, and then says the device was associated.
func WriteFakeSignalProgram(t testing.TB, linkURI string) string {
	t.Helper()
	folder := t.TempDir()
	path := filepath.Join(folder, "signal-cli")
	program := fmt.Sprintf("#!/bin/sh\necho %q\nsleep 0.1\necho \"Associated with: +15555550123\"\n", linkURI)
	if err := os.WriteFile(path, []byte(program), 0o755); err != nil {
		t.Fatalf("cannot write the fake signal-cli program: %v", err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("cannot make the fake signal-cli program runnable: %v", err)
	}
	return folder
}
