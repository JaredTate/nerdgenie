package job

import (
	"context"
	"fmt"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// SetProjectFolder writes the folder into the job record's situation as the
// one line every task of the job reads to know where to work. A job made
// from a work order has it from the start; a job the model made learns it
// from the files its first task wrote.
func (jobs *Jobs) SetProjectFolder(ctx context.Context, jobID string, folder string) error {
	if folder == "" {
		return fmt.Errorf("the project folder of job %s cannot be empty, so name the folder the work lives in", jobID)
	}
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	held, err := jobs.find(jobID)
	if err != nil {
		return err
	}
	return held.keeper.SetSituation(ctx, []string{contract.ProjectFolderLine + folder})
}
