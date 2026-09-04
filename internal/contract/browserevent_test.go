package contract_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// TestTheThreeBrowserEventKindsAreNamed pins the three things a person does in
// the browser window that the worker reports, because worker/browser/PROTOCOL.md
// and this file have to say the same three words.
func TestTheThreeBrowserEventKindsAreNamed(t *testing.T) {
	kinds := map[contract.BrowserEventKind]string{
		contract.BrowserEventClick:    "click",
		contract.BrowserEventType:     "type",
		contract.BrowserEventNavigate: "navigate",
	}
	for kind, written := range kinds {
		if string(kind) != written {
			t.Errorf("the event kind is written %q on the wire, want %q", string(kind), written)
		}
		if !contract.KnownBrowserEventKind(kind) {
			t.Errorf("the kind %q is not one the contract knows, and it is one of the three", kind)
		}
	}
	for _, madeUp := range []contract.BrowserEventKind{"", "scroll", "Click"} {
		if contract.KnownBrowserEventKind(madeUp) {
			t.Errorf("the kind %q was taken as one of the three, and it is not one of them", madeUp)
		}
	}
}

// TestATypedEventCarriesTheLengthAndNeverTheText is the rule that keeps a
// password out of a recording: the worker says how much the person typed and
// never what they typed.
func TestATypedEventCarriesTheLengthAndNeverTheText(t *testing.T) {
	typed := contract.BrowserEvent{
		Kind:   contract.BrowserEventType,
		Ref:    "e3",
		Length: len("a password nobody should ever see"),
		At:     time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC),
	}
	written, err := json.Marshal(typed)
	if err != nil {
		t.Fatalf("a typing event cannot be written as JSON: %v", err)
	}
	if strings.Contains(string(written), "text") {
		t.Errorf("the typing event is written as %s, and it must carry no text at all", written)
	}

	var read contract.BrowserEvent
	if err := json.Unmarshal(written, &read); err != nil {
		t.Fatalf("a typing event cannot be read back from JSON: %v", err)
	}
	if read.Length != typed.Length || read.Ref != typed.Ref || !read.At.Equal(typed.At) {
		t.Errorf("the event read back is %+v, want %+v", read, typed)
	}
}

// TestAClickEventCarriesTheElementAndItsText holds what a recorded click is
// written down from, and TestANavigationCarriesTheAddress holds the third kind.
func TestAClickEventCarriesTheElementAndItsText(t *testing.T) {
	written, err := json.Marshal(contract.BrowserEvent{
		Kind: contract.BrowserEventClick,
		Ref:  "e7",
		Text: "Post",
		At:   time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("a click event cannot be written as JSON: %v", err)
	}
	for _, wanted := range []string{`"kind":"click"`, `"ref":"e7"`, `"text":"Post"`} {
		if !strings.Contains(string(written), wanted) {
			t.Errorf("the click event is written as %s, and it should carry %s", written, wanted)
		}
	}
}

// TestANavigationCarriesTheAddress holds the one field a navigation needs.
func TestANavigationCarriesTheAddress(t *testing.T) {
	written, err := json.Marshal(contract.BrowserEvent{
		Kind:    contract.BrowserEventNavigate,
		Address: "https://fixture.test/changed",
		At:      time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("a navigation event cannot be written as JSON: %v", err)
	}
	if !strings.Contains(string(written), `"address":"https://fixture.test/changed"`) {
		t.Errorf("the navigation event is written as %s, and it should carry the address", written)
	}
}

// TestTheBrowserWorkerNamesTheEventStream is the compile-time proof that every
// browser worker hands out the stream of what the person did, and that the cap
// on what is held for a slow reader is one of the design's own limits.
func TestTheBrowserWorkerNamesTheEventStream(t *testing.T) {
	var streams interface {
		Events(ctx context.Context) (<-chan contract.BrowserEvent, error)
	} = contract.BrowserWorker(nil)
	if streams != nil {
		t.Error("a nil browser worker is not nil when it is read as an event stream")
	}
	if held := contract.DefaultConfig().Caps.BufferedBrowserEvents; held != 256 {
		t.Errorf("the cap on buffered browser events is %d, want 256", held)
	}
}
