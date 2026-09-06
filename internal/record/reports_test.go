package record

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestAJobsReportsAreReadByLabelStraightFromTheLog is the fifth game build's
// last task: it asked to read j2.10 and j2.11, the reports of the two tasks
// before it, and was told there was no job record behind the tool, because a
// task's read tool was wired with its own results and nothing for reports.
// A job's report is read by its label out of the log, for any task.
func TestAJobsReportsAreReadByLabelStraightFromTheLog(t *testing.T) {
	store := testkit.NewFakeStore()
	ctx := t.Context()
	job, err := New(ctx, store, jobStart())
	if err != nil {
		t.Fatalf("cannot create the job: %v", err)
	}
	label, err := job.AddReport(ctx, "the engine is built", "the whole of the engine's report")
	if err != nil {
		t.Fatalf("cannot add the job's report: %v", err)
	}

	reports := ReportsIn(store)
	text, err := reports.Read(ctx, label)
	if err != nil || text != "the whole of the engine's report" {
		t.Errorf("reading %s gave %q (%v), want the whole report", label, text, err)
	}
	if _, err := reports.Read(ctx, "j99.1"); err == nil || !strings.Contains(err.Error(), "j99") {
		t.Errorf("reading a report of a job that is not there gave %v, want a refusal naming the job", err)
	}
	if _, err := reports.Read(ctx, "r7"); err == nil {
		t.Error("reading a task's result label as a report was accepted")
	}
	_ = contract.RecordJob
}
