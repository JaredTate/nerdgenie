package browser

import (
	"context"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// theRankingsText is what a rankings table reads as: one line per row, with
// the cells separated by a bar, so that a number that lives in a cell and on
// no element reaches the model.
const theRankingsText = "Rank | Name | D-Score\n| 2 | DigiByte | 91.4"

// The page's text rides on the snapshot over the wire, with open and with read,
// so that what a page says reaches the model beside what it can act on.
func TestThePageTextComesBackWithOpenAndWithRead(t *testing.T) {
	world := newWorld(t)
	world.worker.AddPage(contract.Snapshot{
		URL: "https://fixture.test/rankings", Title: "Rankings", TabID: "t1", Text: theRankingsText,
	})
	browser := world.browser(t, nil)
	ctx := context.Background()

	page, err := browser.Open(ctx, "https://fixture.test/rankings")
	if err != nil || page.Text != theRankingsText {
		t.Fatalf("open answered with text %q and %v, and it should have carried the table", page.Text, err)
	}
	read, err := browser.Read(ctx, contract.ReadOptions{})
	if err != nil || read.Text != theRankingsText {
		t.Fatalf("read answered with text %q and %v, and it should have carried the table", read.Text, err)
	}
}
