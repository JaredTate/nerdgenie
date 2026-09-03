package testkit

import (
	"encoding/json"
	"slices"
	"testing"
)

// Everything in Coeus that parses bytes from outside the program has a fuzz
// target. Two of them live here, in the package rather than beside it, because
// the browser protocol server's answer function is not exported and driving it
// over its socket would be far too slow to fuzz.

func FuzzTheBrowserProtocolAnswer(f *testing.F) {
	for _, seed := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"health","params":{}}`,
		`{"jsonrpc":"2.0","id":1,"method":"open","params":{"url":"https://fixture.test/simple"}}`,
		`{"jsonrpc":"2.0","id":1,"method":"click","params":{"ref":"e1","expectation":"it moves"}}`,
		`{"jsonrpc":"2.0","id":1,"method":"act","params":{"steps":[{"method":"press","key":"Enter"}]}}`,
		`{"jsonrpc":`,
		``,
		`[]`,
		`{"jsonrpc":"2.0","id":1}`,
	} {
		f.Add([]byte(seed))
	}

	codes := []int{
		CodeParseError, CodeInvalidRequest, CodeMethodNotFound, CodeBadParameters,
		CodeNoSuchReference, CodeSettleTimeout, CodeNoBrowserOpen, CodeBrowserGone,
	}

	f.Fuzz(func(t *testing.T, line []byte) {
		server := &BrowserProtocolServer{worker: NewFakeBrowserWorker(), idleTimeout: DefaultProtocolIdleTimeout}
		answer := server.answer(line)

		if answer["jsonrpc"] != "2.0" {
			t.Fatalf("the answer says jsonrpc is %v, and every answer is JSON-RPC 2.0: %+v", answer["jsonrpc"], answer)
		}
		_, hasResult := answer["result"]
		failure, hasError := answer["error"].(map[string]any)
		if hasResult == hasError {
			t.Fatalf("the answer carries both a result and an error, or neither: %+v", answer)
		}
		if !hasError {
			return
		}

		code, isNumber := failure["code"].(int)
		if !isNumber || !slices.Contains(codes, code) {
			t.Fatalf("the answer carries the code %v, and the protocol defines eight: %+v", failure["code"], failure)
		}
		if message, isText := failure["message"].(string); !isText || message == "" {
			t.Fatalf("the answer carries no message saying what went wrong: %+v", failure)
		}
		if _, err := json.Marshal(answer); err != nil {
			t.Fatalf("the answer cannot be written back as JSON: %v", err)
		}
	})
}

func FuzzWholeRequestBodyText(f *testing.F) {
	for _, seed := range []string{
		`{"system":[{"type":"text","text":"the rules"}],"messages":[{"role":"user","content":"hello"}]}`,
		`{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`,
		`{"tools":[{"name":"read","description":"reads a file"}]}`,
		`{"messages":"not a list"}`,
		`not json at all`,
		``,
		`{"messages":[[[[[[[[[[[[[[[[[[[[[[[[[["deep"]]]]]]]]]]]]]]]]]]]]]]]]]]}`,
		"{\"messages\":\"\xd7\xd3\xd3\"}",
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, body []byte) {
		read := WholeRequestBodyText(body)

		if again := WholeRequestBodyText(body); again != read {
			t.Fatalf("the same body read as %q and then as %q, and the walk is supposed to be in sorted order", read, again)
		}

		var anything any
		if json.Unmarshal(body, &anything) != nil && read != "" {
			t.Fatalf("a body that is not JSON read as %q, and it is supposed to read as no text at all", read)
		}
	})
}
