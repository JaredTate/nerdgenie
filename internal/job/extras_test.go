package job_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/job"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// aDoneList is one done line, with the report that proves it when there is one.
func aDoneList(text string, reportID string) record.Update {
	return record.Update{DoneWhen: []contract.DoneLine{{Text: text, Done: reportID != "", ResultID: reportID}}}
}

func TestOneIncidentPerDistinctFailureCountedRatherThanRepeated(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aScheduledJob(t, "Post every hour.",
		contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour})
	taskID := holding.aTask(t, jobID, "the task that keeps failing", time.Time{})

	holding.finish(t, jobID, taskID, "the site answered 500", true)
	holding.clock.Advance(time.Hour)
	holding.finish(t, jobID, taskID, "The   site  answered 500", true)
	holding.finish(t, jobID, taskID, "the login page appeared", true)

	incidents, err := holding.jobs.Incidents(ctx, jobID)
	if err != nil {
		t.Fatalf("cannot read the incidents of the job: %v", err)
	}
	if len(incidents) != 2 {
		t.Fatalf("three failures made %d incidents, want the two distinct ones: %+v", len(incidents), incidents)
	}
	if incidents[0].Count != 2 {
		t.Errorf("the same failure twice was counted %d times", incidents[0].Count)
	}
	if !incidents[0].LastSeen.After(incidents[0].FirstSeen) {
		t.Errorf("the incident was last seen at %s and first at %s", incidents[0].LastSeen, incidents[0].FirstSeen)
	}
	if incidents[1].Count != 1 || incidents[1].Error != "the login page appeared" {
		t.Errorf("the second incident is %+v, want the login page seen once", incidents[1])
	}
	if _, err := holding.jobs.Incidents(ctx, "99"); err == nil {
		t.Error("the incidents of a job that is not there were read without an error naming it")
	}
}

func TestTheIncidentListIsCapped(t *testing.T) {
	holding := newJobs(t)
	jobID := holding.aJob(t, "Do a long thing.")

	for round := range job.MaxIncidents + 5 {
		taskID := holding.aTask(t, jobID, fmt.Sprintf("the task numbered %d", round), time.Time{})
		holding.finish(t, jobID, taskID, fmt.Sprintf("failure number %d, which reads unlike every other", round), true)
		if err := holding.jobs.Resume(t.Context(), jobID); err != nil {
			t.Fatalf("cannot set the job running again: %v", err)
		}
	}

	incidents, err := holding.jobs.Incidents(t.Context(), jobID)
	if err != nil {
		t.Fatalf("cannot read the incidents: %v", err)
	}
	if len(incidents) != job.MaxIncidents {
		t.Errorf("the job remembers %d incidents, and the cap is %d", len(incidents), job.MaxIncidents)
	}
	if !strings.Contains(incidents[len(incidents)-1].Error, fmt.Sprintf("failure number %d", job.MaxIncidents+4)) {
		t.Errorf("the newest incident is %q, and the oldest are the ones that are dropped", incidents[len(incidents)-1].Error)
	}
}

func TestTheNotepadCarriesNotesBetweenTasksAndIsCapped(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Watch the release page.")

	if err := holding.jobs.AppendNote(ctx, jobID, "the last version seen was 8.22.2"); err != nil {
		t.Fatalf("cannot write the first note: %v", err)
	}
	notepad, err := holding.jobs.Notepad(ctx, jobID)
	if err != nil {
		t.Fatalf("cannot read the notepad: %v", err)
	}
	if notepad != "the last version seen was 8.22.2\n" {
		t.Errorf("the notepad reads %q, want the one note that was written", notepad)
	}
	if err := holding.jobs.AppendNote(ctx, jobID, "  "); err == nil {
		t.Error("a note that says nothing was written onto the notepad")
	}

	line := strings.Repeat("x", 200)
	for range job.NotepadBytes/len(line) + 10 {
		if err := holding.jobs.AppendNote(ctx, jobID, line); err != nil {
			t.Fatalf("cannot write a note: %v", err)
		}
	}
	notepad, err = holding.jobs.Notepad(ctx, jobID)
	if err != nil {
		t.Fatalf("cannot read the notepad: %v", err)
	}
	if len(notepad) > job.NotepadBytes {
		t.Errorf("the notepad holds %d bytes, and the cap is %d", len(notepad), job.NotepadBytes)
	}
	if strings.Contains(notepad, "8.22.2") {
		t.Error("the notepad kept its oldest line and dropped its newest, and it is the other way round")
	}

	after := holding.restart(t)
	kept, err := after.jobs.Notepad(ctx, jobID)
	if err != nil {
		t.Fatalf("cannot read the notepad after the restart: %v", err)
	}
	if kept != notepad {
		t.Errorf("the notepad after the restart holds %d bytes, want the %d it held before", len(kept), len(notepad))
	}
	if _, err := after.jobs.Notepad(ctx, "99"); err == nil {
		t.Error("the notepad of a job that is not there was read without an error naming it")
	}
	if err := after.jobs.AppendNote(ctx, "99", "a note"); err == nil {
		t.Error("a note was written onto the notepad of a job that is not there")
	}
}

func TestAJobThatWatchesKeepsQuietUntilItsOutputChanges(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aScheduledJob(t, "Tell me when the release page changes.",
		contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour})
	if err := holding.jobs.SetMonitor(ctx, jobID, true); err != nil {
		t.Fatalf("cannot set the job watching: %v", err)
	}

	first, _ := holding.nextTask(t, theEpoch().Add(time.Hour))
	firstReport := holding.finish(t, jobID, first.TaskID, "the page says 8.22.2", false)
	second, _ := holding.nextTask(t, theEpoch().Add(2*time.Hour))
	sameReport := holding.finish(t, jobID, second.TaskID, "the page says 8.22.2", false)
	third, _ := holding.nextTask(t, theEpoch().Add(3*time.Hour))
	changedReport := holding.finish(t, jobID, third.TaskID, "the page says 8.22.3", false)

	if sameReport != firstReport {
		t.Errorf("a tick whose output had not changed wrote the new report %s, want the standing %s", sameReport, firstReport)
	}
	if changedReport == firstReport {
		t.Errorf("a tick whose output had changed wrote no new report, and %s is the old one", changedReport)
	}
	held, err := holding.jobs.Load(ctx, jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if len(held.Work.Results) != 2 {
		t.Errorf("three ticks wrote %d reports, want the two that said something new: %+v", len(held.Work.Results), held.Work.Results)
	}
}

func TestOnlyAJobWithAScheduleCanWatchForAChange(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Do a long thing.")

	if err := holding.jobs.SetMonitor(ctx, jobID, true); err == nil {
		t.Error("a job with no schedule was set watching, and it has no tick to watch with")
	}
	if err := holding.jobs.SetMonitor(ctx, jobID, false); err != nil {
		t.Errorf("a job with no schedule could not be told it is not watching: %v", err)
	}
	if err := holding.jobs.SetMonitor(ctx, "99", true); err == nil {
		t.Error("a job that is not there was set watching")
	}
}

func TestAJobCanNeverBeMadeToRestartTheAgent(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Do a long thing.")

	for _, refused := range []string{
		"run systemctl --user restart nerdgenie every morning",
		"sudo systemctl stop nerdgenie.service",
		"nerdgenie restart when the memory gets high",
		"pkill -f nerdgenie",
		"tidy the logs; reboot",
		"tidy the logs\nshutdown now",
		`run ["killall", "nerdgenie"] from the script`,
		"systemctl \\\n restart nerdgenie.service",
	} {
		if _, err := holding.jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: refused}); err == nil {
			t.Errorf("a task was added that would stop the agent: %q", refused)
		}
	}
	for _, allowed := range []string{
		"write a blog piece about the reboot of the franchise",
		"restart the conversation with the user",
		"check that systemctl is installed",
		"tell the user when the machine was last rebooted",
	} {
		if _, err := holding.jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: allowed}); err != nil {
			t.Errorf("an ordinary task was refused as one that restarts the agent: %q: %v", allowed, err)
		}
	}
	_, err := holding.jobs.Create(ctx, contract.NewJob{
		Ask:          "Keep the agent healthy.",
		Schedule:     &contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour},
		TaskTemplate: "systemctl restart nerdgenie if it is using too much memory",
	})
	if err == nil {
		t.Error("a scheduled job was created whose every tick would restart the agent")
	}
	_, err = holding.jobs.Create(ctx, contract.NewJob{
		Ask: "Watch how much memory is being used and systemctl restart nerdgenie when it gets high.",
		Why: "because the machine has been running out",
	})
	if err == nil {
		t.Error("a job was created whose ask names work that restarts the agent, and the ask is what the model reads at the top of every one of that job's tasks")
	}
	err = holding.jobs.Update(ctx, jobID, record.Update{Tasks: []record.NewJobTask{{TaskID: "t99", Text: "reboot the machine"}}})
	if err == nil {
		t.Error("the model wrote a task list holding work that would restart the agent")
	}
}

func TestAJobsListIsCapped(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aJob(t, "Do a long thing.")

	for range job.MaxTasksPerJob {
		if _, err := holding.jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "one more task"}); err != nil {
			t.Fatalf("cannot fill the job's task list: %v", err)
		}
	}

	if _, err := holding.jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "one too many"}); err == nil {
		t.Errorf("a job took more than the %d tasks one job holds", job.MaxTasksPerJob)
	}
}

func TestAScheduledJobWhoseListIsFullIsSwitchedOffWithAReason(t *testing.T) {
	holding := newJobs(t)
	ctx := t.Context()
	jobID := holding.aScheduledJob(t, "Post every hour.",
		contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour})
	for range job.MaxTasksPerJob {
		if _, err := holding.jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "one more task"}); err != nil {
			t.Fatalf("cannot fill the job's task list: %v", err)
		}
	}

	if _, due := holding.nextTask(t, theEpoch().Add(time.Hour)); due {
		t.Error("a job whose list is full still handed out work")
	}

	if state := holding.summaryOf(t, jobID).State; state != contract.JobOff {
		t.Errorf("a scheduled job whose list is full is %q, want %q", state, contract.JobOff)
	}
	held, err := holding.jobs.Load(ctx, jobID)
	if err != nil {
		t.Fatalf("cannot load the job: %v", err)
	}
	if len(held.Lessons.Failures) != 1 || !strings.Contains(held.Lessons.Failures[0].Text, "switched off") {
		t.Errorf("the job does not say why it was switched off: %+v", held.Lessons.Failures)
	}
}
