package desktop

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// scriptedWorker is a desktop worker on a pair of pipes. A test says what each
// method answers, and the worker records what it was asked, so that the Go side
// can be driven through the whole protocol without Node or a screen.
type scriptedWorker struct {
	guard    sync.Mutex
	results  map[string]any
	failures map[string]*workerFailure
	rawLines map[string]string
	asked    []string
	closed   bool

	requests  *io.PipeWriter
	responses *io.PipeReader
	stopped   chan struct{}
}

// newScriptedWorker starts a worker that answers health and nothing else.
func newScriptedWorker() *scriptedWorker {
	worker := &scriptedWorker{
		results:  map[string]any{"health": map[string]any{"healthy": true, "driverVersion": "0.23.2"}},
		failures: map[string]*workerFailure{},
		rawLines: map[string]string{},
		stopped:  make(chan struct{}),
	}
	return worker
}

// answer says what one method returns.
func (worker *scriptedWorker) answer(method string, result any) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	worker.results[method] = result
}

// refuse says that one method fails with the protocol's own code.
func (worker *scriptedWorker) refuse(method string, failure *workerFailure) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	worker.failures[method] = failure
}

// sendRaw says that one method answers with exactly this line, whatever it is.
func (worker *scriptedWorker) sendRaw(method string, line string) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	worker.rawLines[method] = line
}

// methodsAsked is every method the Go side asked for, in order.
func (worker *scriptedWorker) methodsAsked() []string {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	copied := make([]string, len(worker.asked))
	copy(copied, worker.asked)
	return copied
}

// wasStopped says whether the Go side stopped this worker.
func (worker *scriptedWorker) wasStopped() bool {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	return worker.closed
}

// start returns the connection the Go side talks over, and runs the worker.
func (worker *scriptedWorker) start() *Connection {
	requestReader, requestWriter := io.Pipe()
	responseReader, responseWriter := io.Pipe()
	worker.requests, worker.responses = requestWriter, responseReader
	go worker.serve(requestReader, responseWriter)
	return &Connection{
		Requests:  requestWriter,
		Responses: responseReader,
		ProcessID: 4242,
		Stop: func() error {
			worker.guard.Lock()
			worker.closed = true
			worker.guard.Unlock()
			_ = requestWriter.Close()
			_ = responseWriter.Close()
			return nil
		},
	}
}

// serve reads one request line at a time and answers it.
func (worker *scriptedWorker) serve(requests io.Reader, responses io.WriteCloser) {
	lines := bufio.NewScanner(requests)
	lines.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for lines.Scan() {
		answer, ok := worker.answerTo(lines.Bytes())
		if !ok {
			continue
		}
		if _, err := responses.Write([]byte(answer + "\n")); err != nil {
			return
		}
	}
}

// answerTo builds the one line that answers one request line.
func (worker *scriptedWorker) answerTo(line []byte) (string, bool) {
	var request struct {
		ID     int64  `json:"id"`
		Method string `json:"method"`
	}
	if err := json.Unmarshal(line, &request); err != nil {
		return "", false
	}
	worker.guard.Lock()
	defer worker.guard.Unlock()
	worker.asked = append(worker.asked, request.Method)
	if raw, written := worker.rawLines[request.Method]; written {
		return raw, true
	}
	if failure, written := worker.failures[request.Method]; written {
		body, _ := json.Marshal(map[string]any{"code": failure.Code, "message": failure.Message, "data": failure.Data})
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"error":%s}`, request.ID, body), true
	}
	result, written := worker.results[request.Method]
	if !written {
		result = map[string]any{}
	}
	body, err := json.Marshal(result)
	if err != nil {
		return "", false
	}
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":%s}`, request.ID, body), true
}

// aDiff is what an action of the protocol returns, in the parts a test cares about.
func aDiff(met bool, seen string) map[string]any {
	return map[string]any{
		"titleChanged":   false,
		"title":          "Coeus fixture window",
		"newMarks":       []any{},
		"goneMarks":      0,
		"marks":          []any{},
		"expectationMet": met,
		"seen":           seen,
		"settled":        true,
	}
}

// aScreenshot is what the screenshot method returns.
func aScreenshot() map[string]any {
	return map[string]any{
		"pngBase64": "iVBORw0KGgoFAKE",
		"marks": []any{
			map[string]any{"number": 1, "role": "text box", "name": "Type here"},
			map[string]any{"number": 2, "role": "button", "name": "OK"},
		},
		"application": "zenity",
		"title":       "Coeus fixture window",
		"hidden":      0,
	}
}
