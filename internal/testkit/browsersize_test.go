package testkit_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestTheFakeBrowserSetsAndRemembersASizeAndRefusesOneNoScreenHas keeps the
// fake to the worker's own bounds, so a tool tested against it cannot pass a
// size the real worker would refuse.
func TestTheFakeBrowserSetsAndRemembersASizeAndRefusesOneNoScreenHas(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	if _, err := worker.Resize(context.Background(), 480, 640); err == nil {
		t.Fatalf("a resize with no page open was allowed")
	}
	if _, err := worker.Open(context.Background(), testkit.FixtureSimplePage); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	page, err := worker.Resize(context.Background(), 480, 640)
	if err != nil || page.Title != "A simple page" {
		t.Fatalf("resizing the page failed or read the wrong page: %v, %q", err, page.Title)
	}
	if width, height := worker.Size(); width != 480 || height != 640 {
		t.Errorf("the size remembered is %d by %d, want 480 by 640", width, height)
	}
	for _, size := range [][2]int{{10, 640}, {480, 9000}, {4000, 640}, {480, 100}} {
		_, err := worker.Resize(context.Background(), size[0], size[1])
		if err == nil || !strings.Contains(err.Error(), "between 320 and 3840") {
			t.Errorf("the size %v was not refused with the bounds: %v", size, err)
		}
	}
}
