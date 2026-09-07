package testkit_test

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestTheFakeJobKeepsADoneListAndProvesItOneLineAtATime holds the fake to the
// same promise the real store keeps: a job made with a done list keeps it
// unmarked, the harness marks a line with the report that proves it, and the
// job closes only when every line is proved.
func TestTheFakeJobKeepsADoneListAndProvesItOneLineAtATime(t *testing.T) {
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(time.Unix(0, 0).UTC()))
	if err := testkit.CheckJobProvesDoneLines(context.Background(), jobs); err != nil {
		t.Fatal(err)
	}
}
