package log

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// aTimeAsText is what the file holds for aTime.
const aTimeAsText = "2026-03-04T05:06:07.890123456Z"

func TestEncodeTimeWritesUTCAndReadsBackTheSameMoment(t *testing.T) {
	cases := []time.Time{
		aTime,
		time.Time{},
		time.Unix(0, 0).UTC(),
		aTime.In(time.FixedZone("nine hours ahead", 9*60*60)),
	}
	for _, moment := range cases {
		found, err := time.Parse(timeFormat, encodeTime(moment))
		if err != nil {
			t.Errorf("the time %s was written as %q, which cannot be read back: %v", moment, encodeTime(moment), err)
			continue
		}
		if !found.Equal(moment) {
			t.Errorf("the time %s came back as %s, want the same moment", moment, found)
		}
	}
}

func TestEncodeBodyRefusesAnythingThatIsNotJSON(t *testing.T) {
	empty, err := encodeBody(nil)
	if err != nil {
		t.Errorf("a body with nothing in it was refused, and an event may have no body: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("a body with nothing in it became %q, want nothing", empty)
	}

	kept, err := encodeBody(json.RawMessage(`{"text":"hello"}`))
	if err != nil {
		t.Errorf("a body that is JSON was refused: %v", err)
	}
	if string(kept) != `{"text":"hello"}` {
		t.Errorf("a body that is JSON became %q, want it kept as it was", kept)
	}

	if _, err := encodeBody(json.RawMessage("{not json at all")); err == nil {
		t.Error("a body that is not JSON was accepted, and it must be refused")
	}
}

func TestDecodeEventReadsAWholeRow(t *testing.T) {
	found, err := decodeEvent(7, aTimeAsText, "t31", string(contract.EventToolResult), []byte(`{"result":"r7"}`))
	if err != nil {
		t.Fatalf("reading a good row failed: %v", err)
	}
	if found.Sequence != 7 {
		t.Errorf("the sequence number is %d, want 7", found.Sequence)
	}
	if !found.Occurred.Equal(aTime) {
		t.Errorf("the time is %s, want %s", found.Occurred, aTime)
	}
	if found.TaskID != "t31" {
		t.Errorf("the task id is %q, want %q", found.TaskID, "t31")
	}
	if found.Kind != contract.EventToolResult {
		t.Errorf("the kind is %q, want %q", found.Kind, contract.EventToolResult)
	}
	if string(found.Body) != `{"result":"r7"}` {
		t.Errorf("the body is %s, want %s", found.Body, `{"result":"r7"}`)
	}
}

func TestDecodeEventRefusesEveryDamagedRow(t *testing.T) {
	cases := []struct {
		name     string
		sequence int64
		occurred string
		kind     string
		body     []byte
	}{
		{name: "a sequence number of nothing", sequence: 0, occurred: aTimeAsText, kind: "message"},
		{name: "a sequence number below one", sequence: -3, occurred: aTimeAsText, kind: "message"},
		{name: "a time that is not a time", sequence: 1, occurred: "last tuesday", kind: "message"},
		{name: "a time that is empty", sequence: 1, occurred: "", kind: "message"},
		{name: "a kind nothing knows", sequence: 1, occurred: aTimeAsText, kind: "weather report"},
		{name: "a body that is not JSON", sequence: 1, occurred: aTimeAsText, kind: "message", body: []byte("{oh dear")},
	}
	for _, one := range cases {
		if _, err := decodeEvent(one.sequence, one.occurred, "t1", one.kind, one.body); err == nil {
			t.Errorf("a row with %s was read without complaint, and it must be refused", one.name)
		}
	}
}

func TestDecodeEventLeavesAnEmptyBodyEmpty(t *testing.T) {
	found, err := decodeEvent(1, aTimeAsText, "t1", string(contract.EventReply), []byte{})
	if err != nil {
		t.Fatalf("reading a row with no body failed: %v", err)
	}
	if found.Body != nil {
		t.Errorf("the body is %s, want nothing at all", found.Body)
	}
}

func TestShortenedCutsALongValueDown(t *testing.T) {
	short := shortened([]byte("hello"))
	if short != `"hello"` {
		t.Errorf("a short value was quoted as %s, want %q", short, "hello")
	}

	long := shortened([]byte(strings.Repeat("x", maxQuotedLength+40)))
	if !strings.Contains(long, "40 more bytes") {
		t.Errorf("a long value was quoted as %s, and it must say how many bytes it left out", long)
	}
	if len(long) > maxQuotedLength+40 {
		t.Errorf("a long value was quoted in %d characters, and it must be cut short", len(long))
	}
}

func FuzzDecodeEvent(f *testing.F) {
	f.Add(int64(1), aTimeAsText, "t1", "message", []byte(`{"text":"hello"}`))
	f.Add(int64(0), "", "", "", []byte(nil))
	f.Add(int64(2), "last tuesday", "t2", "tool result", []byte("{"))
	f.Add(int64(3), "0001-01-01T00:00:00Z", "t3", "checkpoint", []byte("[1,2,3]"))
	f.Add(int64(-1), aTimeAsText, "t4", "weather report", []byte("null"))

	f.Fuzz(func(t *testing.T, sequence int64, occurred string, taskID string, kind string, body []byte) {
		found, err := decodeEvent(sequence, occurred, taskID, kind, body)
		if err != nil {
			return
		}
		if found.Sequence != sequence {
			t.Errorf("the row numbered %d came back numbered %d", sequence, found.Sequence)
		}
		if found.TaskID != taskID {
			t.Errorf("the task id %q came back as %q", taskID, found.TaskID)
		}
		if !contract.KnownEventKind(found.Kind) {
			t.Errorf("the kind %q was accepted, and only the kinds in internal/contract may be", found.Kind)
		}
		if found.Occurred.Location() != time.UTC {
			t.Errorf("the time came back in the zone %s, and every time the log hands back is in UTC", found.Occurred.Location())
		}
		if found.Body != nil && !json.Valid(found.Body) {
			t.Errorf("the body %q was accepted, and only JSON may be", found.Body)
		}
	})
}
