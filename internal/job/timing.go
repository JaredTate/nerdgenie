package job

import (
	"context"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// Timing says when the job and each of its tasks started and finished, read
// off the state the log holds: the job's start is when it was made, a task's
// start is the moment it was first handed out and its finish the moment its
// report was taken, and the job's finish is when its last task closed it. A
// job or a task from a log written before these moments were kept gives the
// zero time.
func (jobs *Jobs) Timing(_ context.Context, jobID string) (contract.JobTiming, error) {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	held, err := jobs.find(jobID)
	if err != nil {
		return contract.JobTiming{}, err
	}
	timing := contract.JobTiming{Started: held.state.StartedAt, Finished: held.state.FinishedAt, Tasks: map[string]contract.TaskTiming{}}
	for taskID, facts := range held.state.Tasks {
		if facts.StartedAt.IsZero() {
			continue
		}
		timing.Tasks[taskID] = contract.TaskTiming{Started: facts.StartedAt, Finished: facts.FinishedAt}
	}
	return timing, nil
}
