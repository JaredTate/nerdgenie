package testkit_test

import (
	"bufio"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/testkit"
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
			t.Errorf("the call %s came back with an error: %+v", call, answer["error"])
		}
	}
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
