package testkit

import (
	"context"
	"fmt"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// Resize records the size asked for and reads the page again, refusing a size
// no screen has with the worker's own bounds.
func (worker *FakeBrowserWorker) Resize(_ context.Context, width int, height int) (contract.Snapshot, error) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	if width < 320 || width > 3840 || height < 240 || height > 2160 {
		return contract.Snapshot{}, fmt.Errorf("the resize method needs a width in whole pixels between 320 and 3840 and a height between 240 and 2160, and this was %d by %d", width, height)
	}
	page, err := worker.page()
	if err != nil {
		return contract.Snapshot{}, err
	}
	worker.width, worker.height = width, height
	return page, nil
}

// Size is the size the page was last set to, or zero by zero when it never was.
func (worker *FakeBrowserWorker) Size() (int, int) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	return worker.width, worker.height
}
