package replay_test

import (
	"context"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/replay"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theKindsARecordingReads are the five kinds of event the recording reader
// looks inside, so the fuzz target throws its bytes at every one of them.
var theKindsARecordingReads = []contract.EventKind{
	contract.EventCheckpoint,
	contract.EventToolCall,
	contract.EventToolResult,
	contract.EventMessage,
	contract.EventReply,
}

// FuzzReadingARecordedEvent throws any bytes at the body of every kind of event
// a recording is read out of. The log is written by this program, so a body
// that will not read is a log that has been damaged rather than an attack, and
// the reader has to say so rather than panic.
func FuzzReadingARecordedEvent(f *testing.F) {
	f.Add(`{"number":1,"text":"# task 1   running   from terminal   budget left: 99 rounds, 59 minutes"}`)
	f.Add(`{"ID":"c1","Name":"search","Input":{"pattern":"*"}}`)
	f.Add(`{"id":"r1","summary":"a search","text":"three files"}`)
	f.Add(`{"ID":"in-1","Text":"count the files","Channel":"terminal"}`)
	f.Add(`{"text":"there are three files"}`)
	f.Add("")
	f.Add("[]")
	f.Add("{")

	f.Fuzz(func(t *testing.T, body string) {
		store := testkit.NewFakeStore()
		for _, kind := range theKindsARecordingReads {
			if _, err := store.Append(context.Background(), contract.Event{
				TaskID: "1", Kind: kind, Body: []byte(body),
			}); err != nil {
				t.Fatalf("cannot write a fuzzed event into the log: %v", err)
			}
		}
		recording, err := replay.Read(context.Background(), store, "1")
		if err != nil {
			return
		}
		if recording.TaskID != "1" {
			t.Fatalf("the recording came back as task %q and it was read as task 1", recording.TaskID)
		}
	})
}

// FuzzDecodingARecording throws any bytes at the reader of the recording file
// that travels beside a generated test. That file comes off disk, where a
// person may have edited it, so it has to be refused rather than trusted.
func FuzzDecodingARecording(f *testing.F) {
	f.Add(`{"taskId":"1","ask":"count the files","rounds":[],"answer":"three","finalText":""}`)
	f.Add(`{"taskId":"1","rounds":[{"orient":"starting","calls":[{"name":"search","ran":true}]}]}`)
	f.Add("null")
	f.Add("")

	f.Fuzz(func(t *testing.T, written string) {
		recording, err := replay.Decode([]byte(written))
		if err != nil {
			return
		}
		if _, err := recording.Encode(); err != nil {
			t.Fatalf("a recording that read back cannot be written out again: %v", err)
		}
	})
}
