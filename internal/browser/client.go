package browser

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

// mostAnswerBytes caps one answer line. A screenshot is the big one: a picture
// of a full window comes back as base64 text on a single line, so eight
// megabytes is room for that and the marks beside it.
const mostAnswerBytes = 8 << 20

// Connection is one running browser worker: where its requests go, where its
// answers come from, how to stop it, and the exact process identifier, which is
// the only thing this package ever ends.
type Connection struct {
	// Requests takes one request line at a time.
	Requests io.WriteCloser
	// Responses hands back one answer line at a time.
	Responses io.Reader
	// Stop ends the worker and waits for it to go.
	Stop func() error
	// ProcessID is the worker's exact process identifier.
	ProcessID int
}

// client speaks worker/browser/PROTOCOL.md over one connection. It sends one
// request at a time and waits for its answer, because a browser has one window
// and the protocol says so in its first paragraph.
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
		return fmt.Errorf("the request to the browser worker for %s could not be written: %w", method, err)
	}
	if _, err := talker.connection.Requests.Write(append(written, '\n')); err != nil {
		return fmt.Errorf("the browser worker could not be reached to ask it to %s: %w", method, err)
	}
	return nil
}

// waitForLine reads one answer line, or gives up when the deadline passes.
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
			return nil, fmt.Errorf("the answer to %s could not be read from the browser worker: %w", method, answer.err)
		}
		return answer.line, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("the browser worker did not answer %s in time, so it is being started again: %w", method, ctx.Err())
	}
}

// readLine reads one line, and refuses one longer than the cap rather than
// holding it, because a line that long means the other end is broken.
func readLine(reader *bufio.Reader) ([]byte, error) {
	var held []byte
	for {
		part, err := reader.ReadSlice('\n')
		held = append(held, part...)
		if len(held) > mostAnswerBytes {
			return nil, fmt.Errorf("the browser worker sent a line longer than the cap of %d bytes", mostAnswerBytes)
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

// readAnswer reads one answer line into result, or returns what it refused. A
// refusal that carries no id at all is still a refusal, which is what the
// protocol says of a line the worker could not read.
func readAnswer(line []byte, identifier int64, result any) error {
	var answer struct {
		ID     *int64          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *RefusedError   `json:"error"`
	}
	if err := json.Unmarshal(line, &answer); err != nil {
		return fmt.Errorf("the browser worker sent a line that could not be read as an answer: %q", firstPart(line))
	}
	if answer.Error != nil {
		return answer.Error
	}
	if answer.ID == nil || *answer.ID != identifier {
		return fmt.Errorf("the browser worker answered request %v when %d was asked, so the two are out of step", answerID(answer.ID), identifier)
	}
	if len(answer.Result) == 0 {
		return fmt.Errorf("the browser worker's answer to request %d carried neither a result nor an error", identifier)
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(answer.Result, result); err != nil {
		return fmt.Errorf("the browser worker's answer to request %d was not the shape the protocol promises: %w", identifier, err)
	}
	return nil
}

// answerID is the identifier an answer carried, written for an error message,
// and the word "nothing" when it carried none.
func answerID(identifier *int64) string {
	if identifier == nil {
		return "nothing"
	}
	return fmt.Sprintf("%d", *identifier)
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
