package command

import (
	"context"
	"fmt"
	"slices"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// Pause is the "/pause" command: it stops every job that is running and
// remembers which ones it stopped, so that "/resume" starts exactly those again
// and never a job the user had paused for a reason of their own.
//
// A job is the scheduled work in Nerd Genie: work too big for one sitting, run task
// by task by the job runner. "/cron" shows the smaller set of jobs that also
// carry a clock schedule.
func (commands *Commands) Pause() contract.Command {
	return contract.Command{
		Name: "pause",
		Help: "Pauses every job that is running. /resume starts them again.",
		Run: func(ctx context.Context, _ string, _ contract.CommandContext) (string, error) {
			if commands.deps.Jobs == nil {
				return "", notWiredUp("pause", "Jobs")
			}
			listed, err := commands.deps.Jobs.List(ctx)
			if err != nil {
				return "", fmt.Errorf("the jobs could not be listed, so none was paused: %w", err)
			}

			stopped := []string{}
			for _, job := range listed {
				if job.State != contract.JobRunning {
					continue
				}
				if err := commands.deps.Jobs.Pause(ctx, job.ID); err != nil {
					return "", fmt.Errorf("the job %s could not be paused, so %s were: %w", job.ID, inPlainList(stopped), err)
				}
				stopped = append(stopped, job.ID)
			}
			if len(stopped) == 0 {
				return "no job is running, so there is nothing to pause.\n", nil
			}
			commands.rememberPaused(stopped)
			return fmt.Sprintf("paused %s: %s. Type /resume to start them again.\n",
				countedJobs(len(stopped)), inPlainList(stopped)), nil
		},
	}
}

// Resume is the "/resume" command: it starts again exactly the jobs "/pause"
// stopped, and nothing else.
func (commands *Commands) Resume() contract.Command {
	return contract.Command{
		Name: "resume",
		Help: "Starts again the jobs /pause stopped, and nothing else.",
		Run: func(ctx context.Context, _ string, _ contract.CommandContext) (string, error) {
			stopped := commands.takePaused()
			if len(stopped) == 0 {
				return "/pause has stopped nothing, so there is nothing to resume.\n", nil
			}
			if commands.deps.ResumeJob == nil {
				commands.rememberPaused(stopped)
				return "", notWiredUp("resume", "ResumeJob")
			}

			started := []string{}
			for _, jobID := range stopped {
				if err := commands.deps.ResumeJob(ctx, jobID); err != nil {
					commands.rememberPaused(stopped[len(started):])
					return "", fmt.Errorf("the job %s could not be started again, so it is still paused: %w", jobID, err)
				}
				started = append(started, jobID)
			}
			return fmt.Sprintf("resumed %s: %s.\n", countedJobs(len(started)), inPlainList(started)), nil
		},
	}
}

// rememberPaused writes down the jobs "/pause" stopped, so that "/resume" knows
// which ones to start again.
func (commands *Commands) rememberPaused(jobIDs []string) {
	commands.guard.Lock()
	defer commands.guard.Unlock()
	commands.paused = append(commands.paused, jobIDs...)
}

// takePaused hands back the jobs "/pause" stopped and forgets them, so that
// running "/resume" twice cannot start the same job twice.
func (commands *Commands) takePaused() []string {
	commands.guard.Lock()
	defer commands.guard.Unlock()
	taken := slices.Clone(commands.paused)
	commands.paused = nil
	return taken
}

// countedJobs writes a number of jobs the way a person would say it.
func countedJobs(many int) string {
	if many == 1 {
		return "one job"
	}
	return fmt.Sprintf("%d jobs", many)
}
