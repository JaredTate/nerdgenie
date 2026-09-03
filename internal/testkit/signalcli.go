// The shapes here were read from OpenClaw's Signal extension, at
// ~/Code/openclaw/extensions/signal/src/daemon.ts and
// ~/Code/openclaw/extensions/signal/src/client.ts, which is where the three
// paths and the event envelope come from. Nothing was copied: that code is
// TypeScript and talks to a real signal-cli, and this is Go and pretends to be
// one.

package testkit

import (
	"encoding/base64"
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
	// SignalRemoteProcedurePath is where sends, typing indicators, and requests
	// for an attachment's bytes go.
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
	// The stream this connection belongs to is claimed before the headers go out,
	// so that a test which drops the stream the moment it is connected drops this
	// one rather than the next.
	dropped, stopped := daemon.streamSignals()
	stall := daemon.takeStall()

	writer.Header().Set("Content-Type", "text/event-stream")
	writer.WriteHeader(http.StatusOK)
	flush, canFlush := writer.(http.Flusher)
	if canFlush {
		flush.Flush()
	}

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

// handleRemoteProcedure records a send or a typing indicator, and hands back an
// attachment's bytes. The parameters are kept unread until the method is known,
// because getAttachment names one recipient where send names a list of them.
func (daemon *FakeSignalCLI) handleRemoteProcedure(writer http.ResponseWriter, request *http.Request) {
	var call struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
		http.Error(writer, `{"error":{"message":"the call was not JSON"}}`, http.StatusBadRequest)
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	if call.Method == "getAttachment" {
		daemon.answerAttachment(writer, call.Params)
		return
	}

	daemon.recordCall(call.Method, call.Params)
	fmt.Fprint(writer, `{"jsonrpc":"2.0","id":1,"result":{"timestamp":1}}`)
}

// recordCall writes down a send or a typing indicator.
func (daemon *FakeSignalCLI) recordCall(method string, raw json.RawMessage) {
	var params struct {
		Recipient   []string `json:"recipient"`
		Message     string   `json:"message"`
		Attachments []string `json:"attachments"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &params)
	}

	daemon.guard.Lock()
	defer daemon.guard.Unlock()
	switch method {
	case "send":
		daemon.sends = append(daemon.sends, SignalSend{
			Recipients:  params.Recipient,
			Message:     params.Message,
			Attachments: params.Attachments,
		})
	case "sendTyping":
		daemon.typings++
	}
}

// answerAttachment hands back one attachment's bytes as base64 under "data",
// which is what signal-cli answers a getAttachment call with. The method name,
// its parameters, and the base64 field were read from OpenClaw's Signal monitor
// at ~/Code/openclaw/extensions/signal/src/monitor.ts.
func (daemon *FakeSignalCLI) answerAttachment(writer http.ResponseWriter, raw json.RawMessage) {
	var params struct {
		ID string `json:"id"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &params)
	}

	daemon.guard.Lock()
	path, known := daemon.attachments[params.ID]
	daemon.guard.Unlock()

	if !known {
		fmt.Fprintf(writer, `{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":%s}}`,
			mustJSON("there is no attachment with the id "+params.ID+", so use the id from the event"))
		return
	}
	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(writer, `{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":%s}}`,
			mustJSON("the attachment "+params.ID+" is not on the disk, so write the file before pushing the message"))
		return
	}
	fmt.Fprintf(writer, `{"jsonrpc":"2.0","id":1,"result":{"data":%s}}`,
		mustJSON(base64.StdEncoding.EncodeToString(content)))
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
