package testkit_test

import (
	"context"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// tellingBrowser is a fixture browser whose event stream says one thing that no
// browser worker is allowed to say.
type tellingBrowser struct {
	*testkit.FakeBrowserWorker
	said contract.BrowserEvent
}

// Events hands back the one event the test wants the check to catch, and then
// ends, so that the check never waits.
func (worker tellingBrowser) Events(context.Context) (<-chan contract.BrowserEvent, error) {
	events := make(chan contract.BrowserEvent, 1)
	events <- worker.said
	close(events)
	return events, nil
}

func TestTheBrowserCheckCatchesAnEventStreamThatSaysTooMuch(t *testing.T) {
	ctx := context.Background()
	badly := map[string]contract.BrowserEvent{
		"a typing event holding what was typed": {
			Kind: contract.BrowserEventType, Ref: "e3", Text: "the password itself", Length: 19,
		},
		"a kind nobody defined": {
			Kind: "scroll", Ref: "e3",
		},
		"a click naming nothing at all": {
			Kind: contract.BrowserEventClick,
		},
		"a navigation with no address": {
			Kind: contract.BrowserEventNavigate,
		},
	}
	for what, said := range badly {
		worker := testkit.NewFakeBrowserWorker()
		if err := testkit.CheckBrowserWorker(ctx, tellingBrowser{FakeBrowserWorker: worker, said: said}); err == nil {
			t.Errorf("the check passed a worker sending %s, and it must catch it", what)
		}
		if err := worker.Close(); err != nil {
			t.Errorf("closing the fixture browser failed: %v", err)
		}
	}
}

func TestTheFakeBrowserDropsTheOldestEventPastItsBuffer(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	events, err := worker.Events(ctx)
	if err != nil {
		t.Fatalf("the fake browser would not hand out its event stream: %v", err)
	}
	// More than the fixture browser holds, so the oldest have to go. Nobody is
	// reading while they are sent, which is the whole point of the test.
	const sent = 200
	for number := 0; number < sent; number++ {
		worker.PersonDoes(contract.BrowserEvent{
			Kind:    contract.BrowserEventNavigate,
			Address: testkit.FixtureChangedPage,
		})
	}

	held := len(events)
	if held == 0 || held >= sent {
		t.Errorf("the stream holds %d of the %d events sent, want a capped few of them", held, sent)
	}
	if first := <-events; first.Kind != contract.BrowserEventNavigate {
		t.Errorf("the stream held %+v, want the navigations that were sent", first)
	}
}
