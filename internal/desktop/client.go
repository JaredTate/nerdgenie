package desktop

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

// maximumResponseBytes caps one response line. A screenshot is the big one: the
// worker refuses a picture over four megabytes of base64 text, so eight is room
// for that and the marks beside it.
const maximumResponseBytes = 8 << 20

// Connection is one running desktop worker: where its requests go, where its
// answers come from, how to stop it, and the exact process identifier, which is
// the only thing this package ever kills.
type Connection struct {
	// Requests takes one request line at a time.
	Requests io.WriteCloser
	// Responses hands back one response line at a time.
	Responses io.Reader
	// Stop ends the worker and waits for it to go.
	Stop func() error
	// ProcessID is the worker's exact process identifier.
	ProcessID int
}

// client speaks worker/desktop/PROTOCOL.md over one connection. It sends one
// request at a time, because a desktop has one mouse.
type client struct {
	guard      sync.Mutex
	connection *Connection
	reader     *bufio.Reader
	lastID     int64
}

// newClient wraps one connection in the protocol.
func newClient(connection *Connection) *client {
	return &client{
		connection: connection,
		reader:     bufio.NewReaderSize(connection.Responses, 64*1024),
	}
}

// call sends one request, waits for its answer, and reads the answer into
// result. The context carries the method's deadline; when it passes, the caller
// stops the worker, because a worker that did not answer cannot be trusted to
// answer later.
func (talker *client) call(ctx context.Context, method string, params any, result any) error {
	talker.guard.Lock()
	defer talker.guard.Unlock()

	talker.lastID++
	identifier := talker.lastID
	if err := talker.send(identifier, method, params); err != nil {
		return err
	}
	line, err := talker.waitForLine(ctx, method)
	if err != nil {
		return err
	}
	return readAnswer(line, identifier, result)
}

// send writes one request line.
func (talker *client) send(identifier int64, method string, params any) error {
	written, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      identifier,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return fmt.Errorf("the request to the desktop worker for %s could not be written: %w", method, err)
	}
	if _, err := talker.connection.Requests.Write(append(written, '\n')); err != nil {
		return fmt.Errorf("the desktop worker could not be reached to ask it to %s: %w", method, err)
	}
	return nil
}

// waitForLine reads one response line, or gives up when the deadline passes.
func (talker *client) waitForLine(ctx context.Context, method string) ([]byte, error) {
	type reading struct {
		line []byte
		err  error
	}
	read := make(chan reading, 1)
	go func() {
		line, err := readLine(talker.reader)
		read <- reading{line: line, err: err}
	}()

	select {
	case answer := <-read:
		if answer.err != nil {
			return nil, fmt.Errorf("the answer to %s could not be read from the desktop worker: %w", method, answer.err)
		}
		return answer.line, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("the desktop worker did not answer %s in time, so it is being started again: %w", method, ctx.Err())
	}
}

// readLine reads one line, and refuses one longer than the cap rather than
// holding it, because a line that long means the other end is broken.
func readLine(reader *bufio.Reader) ([]byte, error) {
	var held []byte
	for {
		part, err := reader.ReadSlice('\n')
		held = append(held, part...)
		if len(held) > maximumResponseBytes {
			return nil, fmt.Errorf("the desktop worker sent a line longer than the cap of %d bytes", maximumResponseBytes)
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return held, nil
	}
}

// readAnswer reads one response line into result, or returns what it refused.
func readAnswer(line []byte, identifier int64, result any) error {
	var answer struct {
		ID     int64           `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *workerFailure  `json:"error"`
	}
	if err := json.Unmarshal(line, &answer); err != nil {
		return fmt.Errorf("the desktop worker sent a line that could not be read as an answer: %q", firstPart(line))
	}
	if answer.Error != nil {
		return answer.Error
	}
	if answer.ID != identifier {
		return fmt.Errorf("the desktop worker answered request %d when %d was asked, so the two are out of step", answer.ID, identifier)
	}
	if len(answer.Result) == 0 {
		return fmt.Errorf("the desktop worker's answer to request %d carried neither a result nor an error", identifier)
	}
	if err := json.Unmarshal(answer.Result, result); err != nil {
		return fmt.Errorf("the desktop worker's answer to request %d was not the shape the protocol promises: %w", identifier, err)
	}
	return nil
}

// firstPart is as much of a line as is worth putting in an error message.
func firstPart(line []byte) string {
	const shown = 120
	trimmed := strings.TrimSpace(string(line))
	if len(trimmed) <= shown {
		return trimmed
	}
	return trimmed[:shown] + "..."
}
