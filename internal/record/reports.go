package record

import (
	"context"
	"fmt"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// A job's report is read by its label, such as "j4.2", out of the log, by any
// task: the fifth game build's last task asked for the reports of the two
// tasks before it and was told there was no job record behind the tool.

// Reports reads a job's reports by their labels straight from the log.
type Reports struct {
	store contract.Store
}

// ReportsIn is the reader of reports over a store.
func ReportsIn(store contract.Store) Reports {
	return Reports{store: store}
}

// Read brings back the whole text of the report the label names, loading the
// job the label belongs to and reading its log.
func (reports Reports) Read(ctx context.Context, id string) (string, error) {
	jobID, _, isReport := contract.ParseReportID(id)
	if !isReport {
		return "", fmt.Errorf("%q is not the label of a job's report, which reads like j4.2: %w", id, ErrNoSuchResult)
	}
	job, err := Load(ctx, reports.store, contract.RecordJob, jobID)
	if err != nil {
		return "", fmt.Errorf("cannot read job %s to find the report %s: %w", jobID, id, err)
	}
	return job.Read(ctx, id)
}
