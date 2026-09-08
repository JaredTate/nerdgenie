package testkit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// samePoints says whether the points the worker recorded are exactly these.
func samePoints(got []testkit.Point, want ...testkit.Point) bool {
	if len(got) != len(want) {
		return false
	}
	for at := range want {
		if got[at] != want[at] {
			return false
		}
	}
	return true
}

func TestTheFakeBrowserClicksAPointAndRecordsIt(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()

	if _, err := worker.ClickAt(ctx, 10, 20, "anything at all"); err == nil {
		t.Fatal("clicking a point before a page was open was reported as a success, and there is nothing to click on")
	}
	if _, err := worker.Open(ctx, testkit.FixtureSimplePage); err != nil {
		t.Fatalf("opening the page failed: %v", err)
	}

	diff, err := worker.ClickAt(ctx, 10, 20, "the page changes")
	if err != nil {
		t.Fatalf("clicking a point failed: %v", err)
	}
	if diff.Snapshot.URL != testkit.FixtureSimplePage || !diff.Settled {
		t.Errorf("the click at a point gave %+v, want a settled diff with the page it left behind", diff)
	}
	if points := worker.PointsClicked(); !samePoints(points, testkit.Point{Across: 10, Down: 20}) {
		t.Errorf("the worker recorded the points %v, want the one point 10,20", points)
	}
}

func TestABatchCarriesAClickAtAPoint(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	if _, err := worker.Open(ctx, testkit.FixtureSimplePage); err != nil {
		t.Fatalf("opening the page failed: %v", err)
	}

	across, down := 30, 40
	diffs, err := worker.Act(ctx, []contract.ActStep{{Method: "click", Across: &across, Down: &down, Expectation: "the planet is named"}})
	if err != nil {
		t.Fatalf("a batch holding a click at a point was refused: %v", err)
	}
	if len(diffs) != 1 {
		t.Errorf("the batch gave %d diffs, want one for its one step", len(diffs))
	}
	if points := worker.PointsClicked(); !samePoints(points, testkit.Point{Across: 30, Down: 40}) {
		t.Errorf("the worker recorded the points %v, want the one point 30,40", points)
	}
}

func TestTheProtocolServerSendsAClickWithAPointToClickAt(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	server := testkit.NewBrowserProtocolServer(t, worker)

	opened := callProtocol(t, server.SocketPath(), `{"jsonrpc":"2.0","id":1,"method":"open","params":{"url":"`+testkit.FixtureSimplePage+`"}}`)
	if _, failed := opened["error"]; failed {
		t.Fatalf("opening the page came back with an error: %+v", opened["error"])
	}
	answer := callProtocol(t, server.SocketPath(), `{"jsonrpc":"2.0","id":3,"method":"click","params":{"x":519,"y":335,"expectation":"the info panel names Jupiter"}}`)
	if _, failed := answer["error"]; failed {
		t.Fatalf("the click at a point came back with an error: %+v", answer["error"])
	}
	if _, isDiff := resultOf(t, answer)["snapshot"]; !isDiff {
		t.Errorf("the click at a point answered %v, want a diff with a snapshot in it", answer)
	}
	if points := worker.PointsClicked(); !samePoints(points, testkit.Point{Across: 519, Down: 335}) {
		t.Errorf("the worker was handed the points %v, want the one point 519,335 from the wire", points)
	}

	for _, wrong := range []string{
		`{"jsonrpc":"2.0","id":4,"method":"click","params":{"ref":"` + testkit.FixtureChangeLinkRef + `","x":1,"y":1,"expectation":"anything"}}`,
		`{"jsonrpc":"2.0","id":5,"method":"click","params":{"x":1,"expectation":"anything"}}`,
	} {
		answer := callProtocol(t, server.SocketPath(), wrong)
		if _, failed := answer["error"]; !failed {
			t.Errorf("the click %s was made, and a click names a ref or a whole point, never both or half", wrong)
		}
	}
}

// blankPointBrowser clicks a point and hands back no snapshot of the page.
type blankPointBrowser struct{ *testkit.FakeBrowserWorker }

// ClickAt clicks the point properly and then takes the snapshot off the diff.
func (worker blankPointBrowser) ClickAt(ctx context.Context, across int, down int, expectation string) (contract.Diff, error) {
	diff, err := worker.FakeBrowserWorker.ClickAt(ctx, across, down, expectation)
	diff.Snapshot = contract.Snapshot{}
	return diff, err
}

// refusingPointBrowser cannot click a point at all.
type refusingPointBrowser struct{ *testkit.FakeBrowserWorker }

// ClickAt refuses every point, as a worker without the click's second form would.
func (refusingPointBrowser) ClickAt(context.Context, int, int, string) (contract.Diff, error) {
	return contract.Diff{}, errors.New("this worker clicks by reference only")
}

func TestTheBrowserCheckCatchesAWorkerThatCannotClickAPointProperly(t *testing.T) {
	blank := blankPointBrowser{testkit.NewFakeBrowserWorker()}
	defer blank.Close()
	if err := testkit.CheckBrowserWorker(context.Background(), blank); err == nil {
		t.Error("the browser check passed a worker that clicks a point and hands back no snapshot")
	}

	refusing := refusingPointBrowser{testkit.NewFakeBrowserWorker()}
	defer refusing.Close()
	if err := testkit.CheckBrowserWorker(context.Background(), refusing); err == nil {
		t.Error("the browser check passed a worker that cannot click a point")
	}
}
