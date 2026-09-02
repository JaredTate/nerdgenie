package provider

import (
	"strings"
	"testing"
)

// The two tests here are the ones the fake provider server cannot ask, because
// they are about bytes rather than about events: a stream with rubbish in it,
// and a line longer than the cap. Everything else that once needed a stream
// written by hand is now asked of the fake in wire_test.go.

func TestTheOpenAIReaderSkipsLinesThatAreNotJSON(t *testing.T) {
	stream := ": keep alive\n\ndata: not json at all\n\nevent: ping\n\n" +
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"fine\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
	written := strings.Builder{}

	reply, err := parseOpenAIStream(strings.NewReader(stream), "local", Options{}, func(delta string) {
		written.WriteString(delta)
	})

	if err != nil {
		t.Fatalf("a stream with rubbish in it failed: %v", err)
	}
	if reply.Text != "fine" || written.String() != "fine" {
		t.Errorf("the reply is %q and the deltas are %q, want the one good chunk's text", reply.Text, written.String())
	}
}

func TestTheAnthropicReaderRefusesALineLongerThanTheLimit(t *testing.T) {
	stream := "data: {\"type\":\"message_start\",\"message\":{\"note\":\"" + strings.Repeat("x", maxEventLineBytes) + "\"}}\n\n"

	_, err := parseAnthropicStream(strings.NewReader(stream), "opus", Options{}, nil)

	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("a line past the cap came back as %v, want an error naming the limit", err)
	}
}
