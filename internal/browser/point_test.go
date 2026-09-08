package browser

import (
	"context"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// A click at a point goes out as the protocol's click with x and y, both as a
// call of its own and as a step of a batch, and the worker on the other end of
// the wire is handed the point.
func TestAClickAtAPointSendsThePointOnTheWire(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	openTheSimplePage(t, browser)
	ctx := context.Background()

	diff, err := browser.ClickAt(ctx, 519, 335, "the info panel names Jupiter")
	if err != nil {
		t.Fatalf("clicking a point failed: %v", err)
	}
	if diff.Snapshot.URL == "" {
		t.Fatalf("the click at a point came back with no snapshot of the page: %+v", diff)
	}

	across, down := 40, 60
	diffs, err := browser.Act(ctx, []contract.ActStep{{Method: "click", Across: &across, Down: &down, Expectation: "the planet is named"}})
	if err != nil {
		t.Fatalf("a batch with a click at a point failed: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("the batch answered with %d diffs, want one for its one step", len(diffs))
	}

	points := world.worker.PointsClicked()
	want := []testkit.Point{{Across: 519, Down: 335}, {Across: 40, Down: 60}}
	if len(points) != len(want) {
		t.Fatalf("the worker was handed the points %v, want %v", points, want)
	}
	for at := range want {
		if points[at] != want[at] {
			t.Errorf("the worker was handed %v, want %v", points, want)
		}
	}
}
