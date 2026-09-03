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
//
// One reader takes every line the worker sends, rather than each call reading
// its own answer, because the worker also sends lines nobody asked for: the
// events saying what the person did in the window. Those arrive between calls as
// well as during one, and a reader that only ran while a call was outstanding
// would leave a person's clicks sitting in the pipe until the agent next did
// something.
type client struct {
	guard      sync.Mutex
	connection *Connection
	lastID     int64
	// answers holds the one line a waiting call is about to read. It has room
	// for one because the protocol allows one request at a time.
	answers chan []byte
	// whyItStopped is why the reader gave up, written before answers is closed
	// and read only after, which is what makes it safe to read without a lock.
	whyItStopped error
	events       *eventStream
}

// newClient wraps one connection in the protocol and starts reading its lines.
func newClient(connection *Connection, events *eventStream) *client {
	talker := &client{
		connection: connection,
		answers:    make(chan []byte, 1),
		events:     events,
	}
	go talker.readEveryLine(bufio.NewReaderSize(connection.Responses, 64*1024))
	return talker
}

// readEveryLine reads what the worker sends until the connection ends: an event
// goes to whoever is watching, and an answer goes to the call waiting for it. It
// ends by closing the answers, so that a call waiting on a worker that has gone
// is told rather than left waiting.
func (talker *client) readEveryLine(reader *bufio.Reader) {
	defer close(talker.answers)
	for {
		line, err := readLine(reader)
		if err != nil {
			talker.whyItStopped = err
			return
		}
		if event, isEvent := eventInLine(line); isEvent {
			talker.events.deliver(event)
			continue
		}
		select {
		case talker.answers <- line:
		default:
			// Nothing asked for this line, so it is an answer to a call that has
			// already given up. Holding it would hand it to the next call.
		}
	}
}

// call sends one request, waits for its answer, and reads the answer into
// result. The context carries the method's deadline; when it passes, the caller
// stops the worker, because a worker that did not answer cannot be trusted to
// answer later.
func (talker *client) call(ctx context.Context, method string, params any, result any) error {
	talker.guard.Lock()
	defer talker.guard.Unlock()

	talker.forgetAnyStaleAnswer()
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

// forgetAnyStaleAnswer throws away an answer to a call that has already given
// up, so that the next call is never handed the answer to the last one.
func (talker *client) forgetAnyStaleAnswer() {
	select {
	case <-talker.answers:
	default:
	}
}

// waitForLine takes the next answer the reader picked up, or gives up when the
// deadline passes.
func (talker *client) waitForLine(ctx context.Context, method string) ([]byte, error) {
	select {
	case line, open := <-talker.answers:
		if !open {
			return nil, fmt.Errorf("the answer to %s could not be read from the browser worker: %w", method, talker.whyItStopped)
		}
		return line, nil
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
