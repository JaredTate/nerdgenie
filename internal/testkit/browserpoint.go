package testkit

import (
	"context"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// Point is one place on the page that was clicked, in whole CSS pixels from
// the top left of the viewport.
type Point struct {
	// Across is how far from the left.
	Across int
	// Down is how far from the top.
	Down int
}

// ClickAt clicks a point on the page and records it, answering the way Click
// answers for an element with no link behind it: the page settles and the diff
// says what changed. It is the protocol's click with a point in place of a
// reference.
func (worker *FakeBrowserWorker) ClickAt(_ context.Context, across int, down int, expectation string) (contract.Diff, error) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	if _, err := worker.page(); err != nil {
		return contract.Diff{}, err
	}
	worker.points = append(worker.points, Point{Across: across, Down: down})
	return worker.actAndSettle("", expectation, "")
}

// PointsClicked is every point that was clicked, in order, which is how a test
// proves that a point reached the worker as that point.
func (worker *FakeBrowserWorker) PointsClicked() []Point {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	return append([]Point(nil), worker.points...)
}
