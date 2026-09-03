package provider

import (
	"strings"
	"testing"
)

// The seeds are one good Responses API stream and the broken shapes the reader
// is meant to refuse. What is asserted of every input is what the other stream
// fuzzers assert: no panic, no spinning, and when the reader returns a reply
// the deltas it handed on join to the reply's text.

func FuzzCodexStreamReader(f *testing.F) {
	f.Add(codexStream(
		`{"type":"response.output_text.delta","delta":"hi"}`,
		`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file"}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{}"}`,
		`{"type":"response.function_call_arguments.done","item_id":"fc_1","arguments":"{}"}`,
		`{"type":"response.output_item.done","output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file","arguments":"{}"}}`,
		codexCompleted,
	) + "data: [DONE]\n\n")
	f.Add(codexStream(`{"type":"response.incomplete","response":{"incomplete_details":{"reason":"max_output_tokens"}}}`))
	f.Add(codexStream(`{"type":"error","message":"broken"}`))
	f.Add(codexStream(`{"type":"response.failed","response":{"error":{"message":"broken"}}}`))
	f.Add(codexStream(`{"type":"response.output_text.delta","delta":5}`))
	f.Add("data: {\n\n")
	f.Add("event: response.completed\n\n")
	f.Add("")
	f.Add("data:")
	f.Add("\x00\xff\xfe")

	f.Fuzz(func(t *testing.T, stream string) {
		checkPromptly(t, "the codex stream reader", func() {
			written := strings.Builder{}
			result, err := readCodexStream(strings.NewReader(stream), func(delta string) {
				written.WriteString(delta)
			})
			if err == nil && result.Text != written.String() {
				t.Fatalf("the deltas joined to %q and the reply is %q, and the two must always be the same",
					written.String(), result.Text)
			}
		})
	})
}
