package testkit_test

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// callProtocol sends one JSON-RPC line to the fake browser worker's socket and
// returns the line that comes back.
func callProtocol(t *testing.T, socketPath string, line string) map[string]any {
	t.Helper()
	connection, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		t.Fatalf("cannot reach the browser worker's socket: %v", err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("cannot set a deadline on the socket: %v", err)
	}

	if _, err := connection.Write([]byte(line + "\n")); err != nil {
		t.Fatalf("cannot send the request: %v", err)
	}
	reader := bufio.NewReader(connection)
	answer, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("cannot read the answer: %v", err)
	}

	var read map[string]any
	if err := json.Unmarshal([]byte(answer), &read); err != nil {
		t.Fatalf("the answer is not JSON: %v\n%s", err, answer)
	}
	return read
}

// callProtocolWithoutWaitingForTheWrite sends a line that may be bigger than
// anything the server will read, so the write happens on its own and the answer
// is read whether or not the whole line got through.
func callProtocolWithoutWaitingForTheWrite(t *testing.T, socketPath string, line string) map[string]any {
	t.Helper()
	connection, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		t.Fatalf("cannot reach the browser worker's socket: %v", err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("cannot set a deadline on the socket: %v", err)
	}

	go func() { _, _ = connection.Write([]byte(line + "\n")) }()

	answer, err := bufio.NewReader(connection).ReadString('\n')
	if err != nil {
		t.Fatalf("cannot read the answer to an oversized line: %v", err)
	}
	var read map[string]any
	if err := json.Unmarshal([]byte(answer), &read); err != nil {
		t.Fatalf("the answer is not JSON: %v\n%s", err, answer)
	}
	return read
}

func TestTheBrowserProtocolServerAnswersOpenWithASnapshot(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	server := testkit.NewBrowserProtocolServer(t, worker)

	answer := callProtocol(t, server.SocketPath(),
		`{"jsonrpc":"2.0","id":1,"method":"open","params":{"url":"`+testkit.FixtureSimplePage+`"}}`)

	if answer["jsonrpc"] != "2.0" {
		t.Errorf("the answer says jsonrpc is %v, want 2.0", answer["jsonrpc"])
	}
	result, isResult := answer["result"].(map[string]any)
	if !isResult {
		t.Fatalf("the answer has no result in it: %+v", answer)
	}
	if result["url"] != testkit.FixtureSimplePage {
		t.Errorf("the snapshot is of %v, want the simple fixture page", result["url"])
	}
}

func TestTheBrowserProtocolServerAnswersHealthAndTheEightOtherMethods(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	server := testkit.NewBrowserProtocolServer(t, worker)

	callProtocol(t, server.SocketPath(),
		`{"jsonrpc":"2.0","id":1,"method":"open","params":{"url":"`+testkit.FixtureSimplePage+`"}}`)

	byName := map[string]map[string]any{}
	calls := []string{
		`{"jsonrpc":"2.0","id":2,"method":"read","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"click","params":{"ref":"` + testkit.FixtureChangeLinkRef + `","expectation":"the page changes"}}`,
		`{"jsonrpc":"2.0","id":4,"method":"type","params":{"ref":"` + testkit.FixtureUsernameRef + `","text":"hello","expectation":"the box holds it"}}`,
		`{"jsonrpc":"2.0","id":5,"method":"press","params":{"key":"Enter","expectation":"the form is submitted"}}`,
		`{"jsonrpc":"2.0","id":6,"method":"scroll","params":{"direction":"down","amount":3,"expectation":"more shows"}}`,
		`{"jsonrpc":"2.0","id":7,"method":"act","params":{"steps":[{"method":"press","key":"Enter","expectation":"submitted"}]}}`,
		`{"jsonrpc":"2.0","id":8,"method":"tabs","params":{"action":"list"}}`,
		`{"jsonrpc":"2.0","id":9,"method":"screenshot","params":{}}`,
		`{"jsonrpc":"2.0","id":10,"method":"health","params":{}}`,
	}
	for _, call := range calls {
		answer := callProtocol(t, server.SocketPath(), call)
		if _, failed := answer["error"]; failed {
			t.Fatalf("the call %s came back with an error: %+v", call, answer["error"])
		}
		byName[nameOfCall(t, call)] = resultOf(t, answer)
	}

	if diffs, listed := byName["act"]["diffs"].([]any); !listed || len(diffs) != 1 {
		t.Errorf("act answered with %v, want one diff per step that ran", byName["act"]["diffs"])
	}
	if tabs, listed := byName["tabs"]["tabs"].([]any); !listed || len(tabs) == 0 {
		t.Errorf("tabs answered with %v, want the list of tabs PROTOCOL.md shows", byName["tabs"]["tabs"])
	}
	if picture, isText := byName["screenshot"]["pngBase64"].(string); !isText || picture == "" {
		t.Errorf("screenshot answered with no pngBase64: %+v", byName["screenshot"])
	}
	if marks, listed := byName["screenshot"]["marks"].([]any); !listed || len(marks) == 0 {
		t.Errorf("screenshot answered with no marks, and a handoff needs the elements numbered: %+v", byName["screenshot"])
	}
	if healthy, isTrue := byName["health"]["healthy"].(bool); !isTrue || !healthy {
		t.Errorf("health answered %v, want a worker that says it is healthy", byName["health"])
	}
	if byName["read"]["url"] != testkit.FixtureSimplePage {
		t.Errorf("read answered with the page %v, want the one that was opened", byName["read"]["url"])
	}
	for _, method := range []string{"click", "type", "press", "scroll"} {
		snapshot, held := byName[method]["snapshot"].(map[string]any)
		if !held || snapshot["url"] == "" {
			t.Errorf("%s answered with no snapshot of the page it left behind: %+v", method, byName[method])
		}
		if _, said := byName[method]["expectationMet"]; !said {
			t.Errorf("%s never said whether the expectation was met: %+v", method, byName[method])
		}
	}
}

// nameOfCall reads the method out of one request line.
func nameOfCall(t *testing.T, call string) string {
	t.Helper()
	var asked struct {
		Method string `json:"method"`
	}
	if err := json.Unmarshal([]byte(call), &asked); err != nil {
		t.Fatalf("the call %s is not JSON: %v", call, err)
	}
	return asked.Method
}

// resultOf reads the result object off one answer.
func resultOf(t *testing.T, answer map[string]any) map[string]any {
	t.Helper()
	result, isObject := answer["result"].(map[string]any)
	if !isObject {
		t.Fatalf("the answer carries no result object: %+v", answer)
	}
	return result
}

func TestTheBrowserProtocolServerReportsAnUnknownMethodAndABadLine(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	server := testkit.NewBrowserProtocolServer(t, worker)

	unknown := callProtocol(t, server.SocketPath(), `{"jsonrpc":"2.0","id":1,"method":"fly","params":{}}`)
	failure, isFailure := unknown["error"].(map[string]any)
	if !isFailure {
		t.Fatalf("an unknown method came back with no error: %+v", unknown)
	}
	if failure["code"] != float64(-32601) {
		t.Errorf("an unknown method came back with code %v, want -32601", failure["code"])
	}

	broken := callProtocol(t, server.SocketPath(), `{"jsonrpc":`)
	failure, isFailure = broken["error"].(map[string]any)
	if !isFailure || failure["code"] != float64(-32700) {
		t.Errorf("a line that is not JSON came back as %+v, want an error with code -32700", broken)
	}
}

func TestTheProtocolServerAnswersTheDialogMethod(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	server := testkit.NewBrowserProtocolServer(t, worker)

	callProtocol(t, server.SocketPath(),
		`{"jsonrpc":"2.0","id":1,"method":"open","params":{"url":"`+testkit.FixtureSimplePage+`"}}`)
	worker.NextActionOpensADialog(contract.Dialog{Kind: "confirm", Message: "Are you sure?"})
	callProtocol(t, server.SocketPath(),
		`{"jsonrpc":"2.0","id":2,"method":"click","params":{"ref":"`+testkit.FixtureChangeLinkRef+`","expectation":"a dialog opens"}}`)

	answer := callProtocol(t, server.SocketPath(),
		`{"jsonrpc":"2.0","id":3,"method":"dialog","params":{"action":"accept","text":"yes"}}`)

	if failed, isFailure := answer["error"]; isFailure {
		t.Fatalf("answering the dialog came back with an error: %+v", failed)
	}
	result := resultOf(t, answer)
	if _, held := result["snapshot"].(map[string]any); !held {
		t.Errorf("the dialog answer carries no snapshot of the page it left behind: %+v", result)
	}
	if settled, said := result["settled"].(bool); !said || !settled {
		t.Errorf("the dialog answer does not say the page settled: %+v", result)
	}
	if answers := worker.DialogAnswers(); len(answers) != 1 || answers[0].Text != "yes" {
		t.Errorf("the worker recorded the answers %+v, want the one that was given", answers)
	}

	refused := callProtocol(t, server.SocketPath(),
		`{"jsonrpc":"2.0","id":4,"method":"dialog","params":{"action":"maybe"}}`)
	if _, isFailure := refused["error"]; !isFailure {
		t.Errorf("a dialog action nobody defined was accepted: %+v", refused)
	}
}

func TestAnActThatAbortsStillReturnsTheDiffsOfTheStepsThatRan(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	server := testkit.NewBrowserProtocolServer(t, worker)

	callProtocol(t, server.SocketPath(),
		`{"jsonrpc":"2.0","id":1,"method":"open","params":{"url":"`+testkit.FixtureSimplePage+`"}}`)
	answer := callProtocol(t, server.SocketPath(),
		`{"jsonrpc":"2.0","id":2,"method":"act","params":{"steps":[`+
			`{"method":"click","ref":"`+testkit.FixtureChangeLinkRef+`","expectation":"the page changed"},`+
			`{"method":"click","ref":"e999","expectation":"anything at all"}]}}`)

	failure, isFailure := answer["error"].(map[string]any)
	if !isFailure {
		t.Fatalf("a batch that hit a reference nobody has came back with no error: %+v", answer)
	}
	if failure["code"] != float64(-32000) {
		t.Errorf("the aborted batch came back with code %v, want -32000", failure["code"])
	}

	data, carried := failure["data"].(map[string]any)
	if !carried {
		t.Fatalf("the aborted batch carried no data, and PROTOCOL.md promises one diff per step that ran: %+v", failure)
	}
	diffs, listed := data["diffs"].([]any)
	if !listed || len(diffs) != 1 {
		t.Errorf("the aborted batch carried %v, want the one diff of the step that ran", data["diffs"])
	}
}

func TestAnActBatchOverTheWireTakesAScrollStep(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	server := testkit.NewBrowserProtocolServer(t, worker)

	callProtocol(t, server.SocketPath(),
		`{"jsonrpc":"2.0","id":1,"method":"open","params":{"url":"`+testkit.FixtureSimplePage+`"}}`)
	answer := callProtocol(t, server.SocketPath(),
		`{"jsonrpc":"2.0","id":2,"method":"act","params":{"steps":[`+
			`{"method":"scroll","direction":"down","amount":3,"expectation":"more of the page shows"},`+
			`{"method":"press","key":"Enter","expectation":"the form is submitted"}]}}`)

	if failure, isFailure := answer["error"].(map[string]any); isFailure {
		t.Fatalf("a batch holding a scroll step was refused: %+v", failure)
	}
	result, carried := answer["result"].(map[string]any)
	if !carried {
		t.Fatalf("the batch answered without a result: %+v", answer)
	}
	diffs, listed := result["diffs"].([]any)
	if !listed || len(diffs) != 2 {
		t.Errorf("the batch carried %v, want one diff for the scroll and one for the press", result["diffs"])
	}
}

// errorOf reads the error object off one protocol answer, failing the test when
// the answer carried none.
func errorOf(t *testing.T, answer map[string]any) map[string]any {
	t.Helper()
	failure, isFailure := answer["error"].(map[string]any)
	if !isFailure {
		t.Fatalf("the answer carried no error: %+v", answer)
	}
	return failure
}

// protocolErrorRow is one row of the error table in
// worker/browser/PROTOCOL.md: what to send, in what state, and the code that
// must come back.
type protocolErrorRow struct {
	// name says what the row is about.
	name string
	// close shuts the worker down before the call, so the browser is gone.
	close bool
	// open opens a page before the call, so there is something to act on.
	open bool
	// line is the request to send.
	line string
	// code is the error code the protocol says must come back.
	code float64
}

// theProtocolErrorTable is the whole of PROTOCOL.md's error table as rows.
func theProtocolErrorTable() []protocolErrorRow {
	return []protocolErrorRow{
		{
			name: "a line that is not JSON",
			line: `{"jsonrpc":`,
			code: testkit.CodeParseError,
		},
		{
			name: "JSON that is not a request",
			line: `{"jsonrpc":"2.0","id":1}`,
			code: testkit.CodeInvalidRequest,
		},
		{
			name: "a version nobody speaks",
			line: `{"jsonrpc":"1.0","id":1,"method":"health","params":{}}`,
			code: testkit.CodeInvalidRequest,
		},
		{
			name: "a method the protocol does not have",
			line: `{"jsonrpc":"2.0","id":1,"method":"fly","params":{}}`,
			code: testkit.CodeMethodNotFound,
		},
		{
			name: "parameters that are not an object",
			open: true,
			line: `{"jsonrpc":"2.0","id":1,"method":"click","params":[1,2,3]}`,
			code: testkit.CodeBadParameters,
		},
		{
			name: "a reference that is not on the page",
			open: true,
			line: `{"jsonrpc":"2.0","id":1,"method":"click","params":{"ref":"e999","expectation":"anything"}}`,
			code: testkit.CodeNoSuchReference,
		},
		{
			name: "reading with no page open",
			line: `{"jsonrpc":"2.0","id":1,"method":"read","params":{}}`,
			code: testkit.CodeNoBrowserOpen,
		},
		{
			name:  "a worker whose browser has gone",
			close: true,
			line:  `{"jsonrpc":"2.0","id":1,"method":"read","params":{}}`,
			code:  testkit.CodeBrowserGone,
		},
	}
}

func TestEveryErrorInTheProtocolTableComesBackWithItsOwnCode(t *testing.T) {
	for _, row := range theProtocolErrorTable() {
		t.Run(row.name, func(t *testing.T) {
			worker := testkit.NewFakeBrowserWorker()
			defer worker.Close()
			server := testkit.NewBrowserProtocolServer(t, worker)
			if row.open {
				callProtocol(t, server.SocketPath(),
					`{"jsonrpc":"2.0","id":0,"method":"open","params":{"url":"`+testkit.FixtureSimplePage+`"}}`)
			}
			if row.close {
				if err := worker.Close(); err != nil {
					t.Fatalf("closing the worker failed: %v", err)
				}
			}

			failure := errorOf(t, callProtocol(t, server.SocketPath(), row.line))

			if failure["code"] != row.code {
				t.Errorf("the answer came back with code %v, want %v. It said: %v",
					failure["code"], row.code, failure["message"])
			}
		})
	}
}

func TestARequestLineOverTheCapIsAnsweredRatherThanIgnored(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	server := testkit.NewBrowserProtocolServer(t, worker)

	huge := `{"jsonrpc":"2.0","id":1,"method":"type","params":{"ref":"e2","text":"` +
		strings.Repeat("a", 2*testkit.MaxProtocolLine) + `"}}`
	answer := callProtocolWithoutWaitingForTheWrite(t, server.SocketPath(), huge)

	failure := errorOf(t, answer)
	if failure["code"] != float64(testkit.CodeParseError) {
		t.Errorf("a line over the cap came back with code %v, want %v", failure["code"], testkit.CodeParseError)
	}
}

func TestAConnectionThatSaysNothingIsClosedRatherThanHeldForEver(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	server := testkit.NewBrowserProtocolServer(t, worker)
	server.IdleAfter(200 * time.Millisecond)

	connection, err := net.DialTimeout("unix", server.SocketPath(), 2*time.Second)
	if err != nil {
		t.Fatalf("cannot reach the browser worker's socket: %v", err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("cannot set a deadline on the socket: %v", err)
	}

	if _, err := io.ReadAll(connection); err != nil {
		t.Fatalf("the server held a silent connection open past its own deadline: %v", err)
	}
}

func TestTheBrowserProtocolServerReportsAReferenceItCannotFind(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	server := testkit.NewBrowserProtocolServer(t, worker)

	callProtocol(t, server.SocketPath(),
		`{"jsonrpc":"2.0","id":1,"method":"open","params":{"url":"`+testkit.FixtureSimplePage+`"}}`)
	answer := callProtocol(t, server.SocketPath(),
		`{"jsonrpc":"2.0","id":2,"method":"click","params":{"ref":"e999","expectation":"anything"}}`)

	failure, isFailure := answer["error"].(map[string]any)
	if !isFailure {
		t.Fatalf("clicking a reference that is not on the page came back with no error: %+v", answer)
	}
	if failure["code"] != float64(-32000) {
		t.Errorf("the error came back with code %v, want -32000", failure["code"])
	}
	if message, isText := failure["message"].(string); !isText || !strings.Contains(message, "e999") {
		t.Errorf("the error message is %v, want it to name the reference", failure["message"])
	}
}
