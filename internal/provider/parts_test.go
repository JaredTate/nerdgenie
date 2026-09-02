package provider

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// The tests here are for the small pieces the whole-provider tests reach only
// one way each: the header a server asks the caller to wait with, the wait
// between attempts, the words a tool call and a tool result are written in for a
// program that has no tool interface, and the two writers that keep their
// buffers inside a cap.

func TestTheRetryAfterHeaderIsReadInBothOfItsForms(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"a whole number of seconds", "30", 30 * time.Second},
		{"a fraction of a second", "1.5", 1500 * time.Millisecond},
		{"a moment in the future", now.Add(2 * time.Minute).UTC().Format(http.TimeFormat), 2 * time.Minute},
		{"a moment already past", now.Add(-time.Hour).UTC().Format(http.TimeFormat), 0},
		{"nothing at all", "", defaultRetryAfter},
		{"words nobody can read", "soon please", defaultRetryAfter},
		{"a negative number", "-5", defaultRetryAfter},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			header := http.Header{}
			if test.header != "" {
				header.Set("Retry-After", test.header)
			}

			if waited := retryAfterFrom(header, now); waited != test.want {
				t.Errorf("the header %q was read as a wait of %s, want %s", test.header, waited, test.want)
			}
		})
	}
}

func TestTheWaitBetweenAttemptsDoublesAndIsCapped(t *testing.T) {
	tests := []struct {
		attempt int
		jitter  float64
		want    time.Duration
	}{
		{1, 0, time.Second},
		{2, 0, 2 * time.Second},
		{3, 0, 4 * time.Second},
		{1, 1, 1250 * time.Millisecond},
		{20, 0, maxBackoff},
		{0, 0, time.Second},
		{2, -1, 2 * time.Second},
	}
	for _, test := range tests {
		if waited := backoffDelay(test.attempt, test.jitter); waited != test.want {
			t.Errorf("attempt %d with a jitter of %v waits %s, want %s", test.attempt, test.jitter, waited, test.want)
		}
	}
}

func TestARefusalWithNoWordsInItStillSaysSomething(t *testing.T) {
	if said := messageFromBody(nil); said == "" {
		t.Error("a refusal with an empty body was read as nothing at all")
	}
	if said := messageFromBody([]byte(`{"message":"the key is not valid"}`)); said != "the key is not valid" {
		t.Errorf("a refusal with a plain message was read as %q", said)
	}
	long := strings.Repeat("x", maxErrorBodyBytes*2)
	if said := messageFromBody([]byte(long)); len(said) > maxErrorBodyBytes {
		t.Errorf("a very long refusal was read as %d bytes, and the cap is %d", len(said), maxErrorBodyBytes)
	}
}

func TestAnUnknownStopReasonIsReadAsStopped(t *testing.T) {
	said := []string{}
	options := Options{Log: func(line string) { said = append(said, line) }}

	if finish := anthropicFinish("something new", false, "opus", options); finish != contract.FinishStopped {
		t.Errorf("an unknown stop reason was read as %q, want %q", finish, contract.FinishStopped)
	}
	if finish := openAIFinish("something new", false, "local", options); finish != contract.FinishStopped {
		t.Errorf("an unknown finish reason was read as %q, want %q", finish, contract.FinishStopped)
	}
	if len(said) != 2 {
		t.Errorf("the two unknown reasons were written down %d times, want twice: %v", len(said), said)
	}
}

func TestAFieldTypeNobodyRecognisesIsSentAsAString(t *testing.T) {
	schema := schemaForFields([]contract.ToolField{
		{Name: "count", Type: "integer", Required: true},
		{Name: "shape", Type: "a made-up type"},
	})

	if schema.Properties["count"].Type != "integer" {
		t.Errorf("a known type was changed to %q", schema.Properties["count"].Type)
	}
	if schema.Properties["shape"].Type != "string" {
		t.Errorf("an unknown type was sent as %q, want string", schema.Properties["shape"].Type)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "count" {
		t.Errorf("the required fields are %v, want just the one", schema.Required)
	}
}

func TestAFailedToolResultSaysSoOnEveryWire(t *testing.T) {
	failed := contract.ToolResult{CallID: "call_1", Text: "no such file", Failed: true}

	written := renderOneResult(failed)
	if !strings.Contains(written, "failed") || !strings.Contains(written, "call_1") {
		t.Errorf("the words a program reads do not say the tool failed:\n%s", written)
	}

	messages := openAIMessagesFor(contract.Message{Role: contract.RoleUser, ToolResults: []contract.ToolResult{failed}})
	if len(messages) != 1 || !strings.HasPrefix(messages[0].Content, failedToolResultPrefix) {
		t.Errorf("the tool message does not say the tool failed: %+v", messages)
	}

	blocks := anthropicBlocksFor(contract.Message{Role: contract.RoleUser, ToolResults: []contract.ToolResult{failed}})
	if len(blocks) != 1 || !blocks[0].IsError {
		t.Errorf("the tool-result block does not say the tool failed: %+v", blocks)
	}
}

func TestAToolCallWithNoArgumentsIsStillWrittenAsAnObject(t *testing.T) {
	call := contract.ToolCall{ID: "call_1", Name: contract.ToolRead}

	if written := textFormOfCall(call); !strings.Contains(written, `"arguments": {}`) {
		t.Errorf("a call with no arguments was written as:\n%s", written)
	}
	blocks := anthropicBlocksFor(contract.Message{Role: contract.RoleAssistant, ToolCalls: []contract.ToolCall{call}})
	if len(blocks) != 1 || string(blocks[0].Input) != "{}" {
		t.Errorf("a call with no arguments went on the wire as %+v", blocks)
	}
	messages := openAIMessagesFor(contract.Message{Role: contract.RoleAssistant, ToolCalls: []contract.ToolCall{call}})
	if len(messages) != 1 || messages[0].ToolCalls[0].Function.Arguments != "{}" {
		t.Errorf("a call with no arguments went on the wire as %+v", messages)
	}
}

func TestAMessageWithNothingInItIsLeftOut(t *testing.T) {
	blocks := anthropicMessages([]contract.Message{{Role: contract.RoleUser}})
	if len(blocks) != 0 {
		t.Errorf("an empty message was sent anyway: %+v", blocks)
	}
	messages := openAIMessagesFor(contract.Message{Role: contract.RoleUser})
	if len(messages) != 0 {
		t.Errorf("an empty message was sent anyway: %+v", messages)
	}
}

func TestTheAnthropicAddressIsThePublicOneUnlessTheConfigurationSaysOtherwise(t *testing.T) {
	plain := &anthropicModel{alias: contract.ModelAlias{Name: "opus"}}
	if plain.address() != anthropicPublicAddress+"/v1/messages" {
		t.Errorf("an alias with no address calls %q", plain.address())
	}
	named := &anthropicModel{alias: contract.ModelAlias{Name: "opus", BaseAddress: "http://example.test/"}}
	if named.address() != "http://example.test/v1/messages" {
		t.Errorf("an alias with an address calls %q", named.address())
	}
}

func TestTheCostOfTheLastRunIsKept(t *testing.T) {
	said := []string{}
	model := &commandLineModel{
		alias:   contract.ModelAlias{Name: "opus", Program: contract.ClaudeProgram},
		options: Options{Log: func(line string) { said = append(said, line) }},
	}

	model.rememberCost(programResult{cost: 0.0042, usage: contract.Usage{InputTokens: 10, OutputTokens: 2}})

	if model.LastCostUSD() != 0.0042 {
		t.Errorf("the last call is remembered as costing %v, want 0.0042", model.LastCostUSD())
	}
	model.rememberCost(programResult{usage: contract.Usage{InputTokens: 10}})
	if model.LastCostUSD() != 0 {
		t.Errorf("a run that reported no cost left %v behind", model.LastCostUSD())
	}
	if len(said) != 2 {
		t.Errorf("the two runs were written down %d times: %v", len(said), said)
	}
}

func TestAProgramThatComplainsForEverIsCutOffAtTheCap(t *testing.T) {
	writer := &cappedWriter{limit: 10}

	written, err := writer.Write([]byte("the first part"))

	if err != nil || written != len("the first part") {
		t.Fatalf("the writer reported %d bytes and %v, and it must never refuse what a program prints", written, err)
	}
	if _, err := writer.Write([]byte(" and the rest")); err != nil {
		t.Fatalf("the writer refused the rest: %v", err)
	}
	if writer.text() != "the first" {
		t.Errorf("the writer kept %q, want the first ten bytes trimmed", writer.text())
	}
}

func TestTextTooLongForOneReplyIsCutAtTheCap(t *testing.T) {
	written := strings.Builder{}
	stream := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"" +
		strings.Repeat("a", 1000) + "\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"

	reply, err := parseOpenAIStream(strings.NewReader(stream), "local", Options{}, func(delta string) {
		written.WriteString(delta)
	})

	if err != nil {
		t.Fatalf("a long reply failed: %v", err)
	}
	if reply.Text != written.String() {
		t.Error("the deltas and the reply disagree on a long answer")
	}
}

func TestOneOfSeveralStringsIsChosen(t *testing.T) {
	if chosen := firstNonEmpty("", "  ", "third"); chosen != "third" {
		t.Errorf("the first string with something in it is %q, want third", chosen)
	}
	if chosen := firstNonEmpty("", ""); chosen != "" {
		t.Errorf("a list with nothing in it gave %q", chosen)
	}
}
