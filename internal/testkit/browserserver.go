package testkit

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// The error codes worker/browser/PROTOCOL.md defines.
const (
	// CodeParseError means the line was not JSON.
	CodeParseError = -32700
	// CodeMethodNotFound means there is no such method.
	CodeMethodNotFound = -32601
	// CodeBadParameters means the parameters were wrong.
	CodeBadParameters = -32602
	// CodeNoSuchReference means the element could not be found on the page.
	CodeNoSuchReference = -32000
	// CodeSettleTimeout means the page did not settle before the limit.
	CodeSettleTimeout = -32001
)

// ErrNoSuchMethod means the caller asked for a method the protocol does not
// have.
var ErrNoSuchMethod = errors.New("the browser worker has no such method, so check the list in worker/browser/PROTOCOL.md")

// BrowserProtocolServer serves a browser worker over a local socket, speaking
// the JSON-RPC of worker/browser/PROTOCOL.md, one JSON object per line. The Go
// client of wave 5 is tested against this rather than against the interface, so
// that the protocol document itself is what both sides agree on.
type BrowserProtocolServer struct {
	worker     contract.BrowserWorker
	listener   net.Listener
	socketPath string
}

// NewBrowserProtocolServer starts the server on a socket under a temporary
// folder, and stops it when the test ends.
func NewBrowserProtocolServer(t testing.TB, worker contract.BrowserWorker) *BrowserProtocolServer {
	t.Helper()
	socketPath := filepath.Join(t.TempDir(), "browser.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("cannot open the browser worker's socket: %v", err)
	}

	server := &BrowserProtocolServer{worker: worker, listener: listener, socketPath: socketPath}
	go server.accept()
	t.Cleanup(func() { _ = server.Close() })
	return server
}

// SocketPath is where the server is listening.
func (server *BrowserProtocolServer) SocketPath() string {
	return server.socketPath
}

// Close stops the server.
func (server *BrowserProtocolServer) Close() error {
	return server.listener.Close()
}

// accept takes callers one at a time until the listener closes.
func (server *BrowserProtocolServer) accept() {
	for {
		connection, err := server.listener.Accept()
		if err != nil {
			return
		}
		go server.serve(connection)
	}
}

// serve answers every line one caller sends.
func (server *BrowserProtocolServer) serve(connection net.Conn) {
	defer connection.Close()
	lines := bufio.NewScanner(connection)
	for lines.Scan() {
		answer := server.answer(lines.Bytes())
		written, err := json.Marshal(answer)
		if err != nil {
			return
		}
		if _, err := connection.Write(append(written, '\n')); err != nil {
			return
		}
	}
}

// protocolCall is one JSON-RPC request as it arrives on the socket.
type protocolCall struct {
	// ID is the caller's number for the request, echoed back on the answer.
	ID any `json:"id"`
	// Method is which of the eleven methods was asked for.
	Method string `json:"method"`
	// Params is the method's arguments, still unparsed.
	Params json.RawMessage `json:"params"`
}

// answer turns one line into the JSON-RPC answer for it.
func (server *BrowserProtocolServer) answer(line []byte) map[string]any {
	var call protocolCall
	if err := json.Unmarshal(line, &call); err != nil {
		return protocolFailure(nil, CodeParseError, "the line was not JSON, so send one JSON object per line", nil)
	}

	result, err := server.run(call)
	if err != nil {
		// A failed call may still have something to report: PROTOCOL.md promises
		// one diff per step of a batch that ran, and says a -32000 carries a fresh
		// snapshot. Both ride in the error's data field.
		return protocolFailure(call.ID, codeFor(err), err.Error(), result)
	}
	return map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": result}
}

// run does the work of one method and returns whatever it produced.
func (server *BrowserProtocolServer) run(call protocolCall) (any, error) {
	ctx := context.Background()
	var params struct {
		URL         string                   `json:"url"`
		VisibleOnly bool                     `json:"visibleOnly"`
		Ref         string                   `json:"ref"`
		Text        string                   `json:"text"`
		Key         string                   `json:"key"`
		Direction   contract.ScrollDirection `json:"direction"`
		Amount      int                      `json:"amount"`
		Expectation string                   `json:"expectation"`
		Steps       []contract.ActStep       `json:"steps"`
		Action      contract.TabAction       `json:"action"`
		TabID       string                   `json:"tabId"`
	}
	if len(call.Params) > 0 {
		if err := json.Unmarshal(call.Params, &params); err != nil {
			return nil, errors.New("the parameters were not an object, so check the method in PROTOCOL.md")
		}
	}

	switch call.Method {
	case "open":
		return onlyOnSuccess(server.worker.Open(ctx, params.URL))
	case "read":
		return onlyOnSuccess(server.worker.Read(ctx, contract.ReadOptions{VisibleOnly: params.VisibleOnly}))
	case "click":
		return onlyOnSuccess(server.worker.Click(ctx, params.Ref, params.Expectation))
	case "type":
		return onlyOnSuccess(server.worker.Type(ctx, params.Ref, params.Text, params.Expectation))
	case "press":
		return onlyOnSuccess(server.worker.Press(ctx, params.Key, params.Expectation))
	case "scroll":
		return onlyOnSuccess(server.worker.Scroll(ctx, params.Direction, params.Amount, params.Expectation))
	case "act":
		return server.act(ctx, params.Steps)
	case "tabs":
		tabs, err := server.worker.Tabs(ctx, params.Action, params.TabID)
		return onlyOnSuccess(map[string]any{"tabs": tabs}, err)
	case "loginFill":
		return server.loginFill(ctx, call.Params)
	case "screenshot":
		return onlyOnSuccess(server.worker.Screenshot(ctx))
	case "health":
		return onlyOnSuccess(server.worker.Health(ctx))
	case "dialog":
		return onlyOnSuccess(server.dialog(ctx, call.Params))
	default:
		return nil, fmt.Errorf("the method %q is not one of the twelve: %w", call.Method, ErrNoSuchMethod)
	}
}

// dialog reads the two dialog fields and answers the open dialog box.
func (server *BrowserProtocolServer) dialog(ctx context.Context, raw json.RawMessage) (contract.Diff, error) {
	var params struct {
		Action contract.DialogAction `json:"action"`
		Text   string                `json:"text"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return contract.Diff{}, fmt.Errorf("cannot read the dialog parameters, so check the action and text fields: %w", err)
		}
	}
	return server.worker.Dialog(ctx, params.Action, params.Text)
}

// act runs a batch and keeps the diffs of the steps that ran, even when a later
// step failed, because PROTOCOL.md promises one diff per step that ran and the
// model needs to see exactly where the batch stopped.
func (server *BrowserProtocolServer) act(ctx context.Context, steps []contract.ActStep) (any, error) {
	diffs, err := server.worker.Act(ctx, steps)
	if err != nil && len(diffs) == 0 {
		return nil, err
	}
	return map[string]any{"diffs": diffs}, err
}

// onlyOnSuccess drops a half-built result when the call failed, so that an error
// carries nothing but what it meant to carry.
func onlyOnSuccess[Result any](result Result, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return result, nil
}

// loginFill reads the credential fields separately, so that they are never part
// of the parameter struct the other methods share.
func (server *BrowserProtocolServer) loginFill(ctx context.Context, raw json.RawMessage) (any, error) {
	var fields contract.LoginFields
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, errors.New("the login parameters were not an object, so check loginFill in PROTOCOL.md")
	}
	return server.worker.LoginFill(ctx, fields)
}

// protocolFailure builds the error answer PROTOCOL.md describes, carrying
// whatever the failed call still had to report in the error's data field.
func protocolFailure(id any, code int, message string, data any) map[string]any {
	failure := map[string]any{"code": code, "message": message}
	if data != nil {
		failure["data"] = data
	}
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   failure,
	}
}

// codeFor picks the protocol's error code for one Go error.
func codeFor(err error) int {
	switch {
	case errors.Is(err, ErrSettleTimeout):
		return CodeSettleTimeout
	case errors.Is(err, ErrNoSuchMethod):
		return CodeMethodNotFound
	case errors.Is(err, ErrNoSuchReference):
		return CodeNoSuchReference
	default:
		return CodeBadParameters
	}
}
