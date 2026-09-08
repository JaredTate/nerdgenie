package testkit

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The error codes worker/browser/PROTOCOL.md defines, all eight of them.
const (
	// CodeParseError means the line was not JSON, or was longer than the cap.
	CodeParseError = -32700
	// CodeInvalidRequest means the line was JSON but not a JSON-RPC request.
	CodeInvalidRequest = -32600
	// CodeMethodNotFound means there is no such method.
	CodeMethodNotFound = -32601
	// CodeBadParameters means the parameters were wrong.
	CodeBadParameters = -32602
	// CodeNoSuchReference means the element could not be found on the page.
	CodeNoSuchReference = -32000
	// CodeSettleTimeout means the page did not settle before the limit.
	CodeSettleTimeout = -32001
	// CodeNoBrowserOpen means no page is open, so there is nothing to act on.
	CodeNoBrowserOpen = -32002
	// CodeBrowserGone means the browser died and the worker must be restarted.
	CodeBrowserGone = -32003
)

// The bounds the protocol server keeps, because every buffer has a cap and every
// wait has a timeout.
const (
	// MaxProtocolLine is the longest request line the server will read. A longer
	// one is answered with a parse error rather than ending the connection in
	// silence.
	MaxProtocolLine = 256 * 1024
	// DefaultProtocolIdleTimeout is how long the server waits for the next line
	// from a caller that has said nothing before it closes the connection.
	DefaultProtocolIdleTimeout = 30 * time.Second
	// protocolCallTimeout bounds one method call, so a worker that hangs does not
	// hold the connection for ever.
	protocolCallTimeout = 30 * time.Second
	// maxProtocolCallers is how many callers the server serves at once. More than
	// this wait in the listener's own queue.
	maxProtocolCallers = 16
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

	guard       sync.Mutex
	idleTimeout time.Duration
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

	server := &BrowserProtocolServer{
		worker:      worker,
		listener:    listener,
		socketPath:  socketPath,
		idleTimeout: DefaultProtocolIdleTimeout,
	}
	go server.accept()
	t.Cleanup(func() { _ = server.Close() })
	return server
}

// SocketPath is where the server is listening.
func (server *BrowserProtocolServer) SocketPath() string {
	return server.socketPath
}

// IdleAfter says how long the server waits for the next line from a caller that
// has said nothing, so that a test can prove the deadline without waiting the
// usual half a minute for it.
func (server *BrowserProtocolServer) IdleAfter(wait time.Duration) {
	server.guard.Lock()
	defer server.guard.Unlock()
	server.idleTimeout = wait
}

// idleWait is how long the server currently waits between lines.
func (server *BrowserProtocolServer) idleWait() time.Duration {
	server.guard.Lock()
	defer server.guard.Unlock()
	return server.idleTimeout
}

// Close stops the server.
func (server *BrowserProtocolServer) Close() error {
	return server.listener.Close()
}

// accept takes callers until the listener closes, no more than
// maxProtocolCallers of them at once. A caller beyond that waits in the
// listener's own queue rather than costing another goroutine.
func (server *BrowserProtocolServer) accept() {
	callers := make(chan struct{}, maxProtocolCallers)
	for {
		connection, err := server.listener.Accept()
		if err != nil {
			return
		}
		callers <- struct{}{}
		go func() {
			defer func() { <-callers }()
			server.serve(connection)
		}()
	}
}

// serve answers every line one caller sends, until the caller goes quiet for
// longer than the idle timeout or sends a line longer than the cap. What the
// person did in the window goes the other way at the same time, as the event
// notifications worker/browser/PROTOCOL.md rule 7 describes.
func (server *BrowserProtocolServer) serve(connection net.Conn) {
	defer connection.Close()
	caller := &oneCaller{server: server, connection: connection}
	watching, stopWatching := context.WithCancel(context.Background())
	defer stopWatching()
	if events, err := server.worker.Events(watching); err == nil {
		go caller.sendEvents(events)
	}

	lines := bufio.NewScanner(connection)
	lines.Buffer(make([]byte, 0, 64*1024), MaxProtocolLine)
	for {
		if err := connection.SetDeadline(time.Now().Add(server.idleWait())); err != nil {
			return
		}
		if !lines.Scan() {
			caller.sayWhyTheLineWasNotRead(lines.Err())
			return
		}
		if !caller.write(server.answer(lines.Bytes())) {
			return
		}
	}
}

// oneCaller is one connection to the server. It exists because two things write
// to a connection now, the answers and the event notifications, and a line from
// one must never land in the middle of a line from the other.
type oneCaller struct {
	server     *BrowserProtocolServer
	connection net.Conn
	guard      sync.Mutex
}

// sendEvents passes on what the person did in the window until the stream ends
// or the caller has gone.
func (caller *oneCaller) sendEvents(events <-chan contract.BrowserEvent) {
	for event := range events {
		if !caller.write(map[string]any{"jsonrpc": "2.0", "method": "event", "params": event}) {
			return
		}
	}
}

// sayWhyTheLineWasNotRead answers a caller whose line could not be read at all,
// which the protocol calls a parse error. A caller that simply went away or went
// quiet gets nothing, because there is nobody left to tell.
func (caller *oneCaller) sayWhyTheLineWasNotRead(err error) {
	if !errors.Is(err, bufio.ErrTooLong) {
		return
	}
	caller.write(protocolFailure(nil, CodeParseError,
		fmt.Sprintf("that request line is longer than the %d byte cap, so send a smaller one", MaxProtocolLine), nil))
}

// write sends one line and says whether the caller is still there.
func (caller *oneCaller) write(answer map[string]any) bool {
	caller.guard.Lock()
	defer caller.guard.Unlock()
	written, err := json.Marshal(answer)
	if err != nil {
		return false
	}
	if err := caller.connection.SetWriteDeadline(time.Now().Add(caller.server.idleWait())); err != nil {
		return false
	}
	_, err = caller.connection.Write(append(written, '\n'))
	return err == nil
}

// protocolCall is one JSON-RPC request as it arrives on the socket.
type protocolCall struct {
	// Version must be "2.0", and anything else is not a request this server
	// speaks.
	Version string `json:"jsonrpc"`
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
	if call.Version != "2.0" || call.Method == "" {
		return protocolFailure(call.ID, CodeInvalidRequest,
			"that is JSON but not a JSON-RPC request, so send a jsonrpc of 2.0 and a method", nil)
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
	ctx, cancel := context.WithTimeout(context.Background(), protocolCallTimeout)
	defer cancel()
	var params struct {
		URL         string                   `json:"url"`
		VisibleOnly bool                     `json:"visibleOnly"`
		Ref         string                   `json:"ref"`
		Across      *int                     `json:"x"`
		Down        *int                     `json:"y"`
		Text        string                   `json:"text"`
		Key         string                   `json:"key"`
		Direction   contract.ScrollDirection `json:"direction"`
		Amount      int                      `json:"amount"`
		Width       int                      `json:"width"`
		Height      int                      `json:"height"`
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
		return onlyOnSuccess(server.click(ctx, params.Ref, params.Across, params.Down, params.Expectation))
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
	case "resize":
		return onlyOnSuccess(server.worker.Resize(ctx, params.Width, params.Height))
	case "health":
		return onlyOnSuccess(server.worker.Health(ctx))
	case "dialog":
		return onlyOnSuccess(server.dialog(ctx, call.Params))
	default:
		return nil, fmt.Errorf("the method %q is not one of the thirteen: %w", call.Method, ErrNoSuchMethod)
	}
}

// click is the protocol's click in either of its forms: by reference, or at a
// point when x and y are both given. A request naming both, or neither, is
// refused the way the real worker refuses it.
func (server *BrowserProtocolServer) click(ctx context.Context, ref string, across *int, down *int, expectation string) (contract.Diff, error) {
	namesPoint := across != nil || down != nil
	if ref != "" && namesPoint {
		return contract.Diff{}, errors.New("a click names a ref or a point, never both, so send one of the two")
	}
	if ref == "" && (across == nil || down == nil) {
		return contract.Diff{}, errors.New("a click names a ref or a point with both x and y, so send one of the two")
	}
	if namesPoint {
		return server.worker.ClickAt(ctx, *across, *down, expectation)
	}
	return server.worker.Click(ctx, ref, expectation)
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

// codeFor picks the protocol's error code for one Go error, following the table
// in worker/browser/PROTOCOL.md. What is left over is a bad set of parameters,
// which is the one thing the model can act on, so it is the residue rather than
// the catch-all it used to be.
func codeFor(err error) int {
	switch {
	case errors.Is(err, ErrSettleTimeout):
		return CodeSettleTimeout
	case errors.Is(err, ErrNoSuchMethod):
		return CodeMethodNotFound
	case errors.Is(err, ErrNoSuchReference):
		return CodeNoSuchReference
	case errors.Is(err, ErrBrowserGone):
		return CodeBrowserGone
	case errors.Is(err, ErrNoPageOpen):
		return CodeNoBrowserOpen
	default:
		return CodeBadParameters
	}
}
