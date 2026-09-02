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
	"os"
	"path/filepath"
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

	guard    sync.Mutex
	events   chan string
	dropped  chan struct{}
	stall    time.Duration
	sends    []SignalSend
	typings  int
	nextID   int
	isClosed bool
}

// NewFakeSignalCLI starts the fake daemon. Close it when the test is done.
func NewFakeSignalCLI() *FakeSignalCLI {
	daemon := &FakeSignalCLI{
		events:  make(chan string, inboundQueueSize),
		dropped: make(chan struct{}),
	}
	router := http.NewServeMux()
	router.HandleFunc(SignalHealthPath, daemon.handleHealth)
	router.HandleFunc(SignalEventsPath, daemon.handleEvents)
	router.HandleFunc(SignalRemoteProcedurePath, daemon.handleRemoteProcedure)
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

// PushMessage puts one inbound message on the stream, with any attachments, and
// reports the overflow rather than blocking when the queue is full and nothing
// is reading it.
func (daemon *FakeSignalCLI) PushMessage(sender string, text string, attachments ...string) error {
	daemon.guard.Lock()
	daemon.nextID++
	number := daemon.nextID
	daemon.guard.Unlock()

	event := map[string]any{
		"envelope": map[string]any{
			"source":    sender,
			"timestamp": number,
			"dataMessage": map[string]any{
				"message":     text,
				"attachments": attachmentList(attachments),
			},
		},
	}
	daemon.events <- fmt.Sprintf("data: %s\n\n", mustJSON(event))
	return nil
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

// DropStream cuts the event stream, the way a daemon that died does.
func (daemon *FakeSignalCLI) DropStream() {
	daemon.guard.Lock()
	defer daemon.guard.Unlock()
	if !daemon.isClosed {
		daemon.isClosed = true
		close(daemon.dropped)
	}
}

// StallStream makes the stream go quiet for a while without closing, which is
// what the harness's forced reconnect exists for.
func (daemon *FakeSignalCLI) StallStream(wait time.Duration) {
	daemon.guard.Lock()
	defer daemon.guard.Unlock()
	daemon.stall = wait
}

// Close shuts the fake daemon down.
func (daemon *FakeSignalCLI) Close() {
	daemon.DropStream()
	daemon.server.Close()
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

	daemon.guard.Lock()
	stall := daemon.stall
	daemon.guard.Unlock()
	if stall > 0 {
		select {
		case <-time.After(stall):
		case <-request.Context().Done():
			return
		}
	}

	for {
		select {
		case event := <-daemon.events:
			fmt.Fprint(writer, event)
			if canFlush {
				flush.Flush()
			}
		case <-daemon.dropped:
			return
		case <-request.Context().Done():
			return
		}
	}
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

// attachmentList turns the paths into the shape signal-cli reports them in.
func attachmentList(paths []string) []map[string]any {
	listed := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		listed = append(listed, map[string]any{"filename": filepath.Base(path), "id": path})
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
