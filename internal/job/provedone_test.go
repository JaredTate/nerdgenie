package job_test

import (
	"testing"

	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestAJobKeepsADoneListAndProvesItOneLineAtATime: a job made from a work
// order carries the person's done lines, the harness marks each with the
// report that proves it, and the job closes only when every line is proved.
func TestAJobKeepsADoneListAndProvesItOneLineAtATime(t *testing.T) {
	holding := newJobs(t)
	if err := testkit.CheckJobProvesDoneLines(t.Context(), holding.jobs); err != nil {
		t.Fatal(err)
	}
}
