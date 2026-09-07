package testkit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The named job the job checks make: a short name, and an ask long enough that
// a store listing by the ask instead of the name shows up in the title.
const (
	theNamedJobsName = "Tater Tots Tetris"
	theNamedJobsAsk  = "Build a Tetris game where every piece is a tater tot, with a score board, a pause key, and a sound when a row clears, then put it on the web so that my nephew can play it on his tablet."
)

// The two tasks the job check adds beside its plain one: prose of the kind a
// model writes every day, with a comma and a space, a colon, and an apostrophe
// in it. A live run found the real store refusing such a task while the fake
// took it, because a job task's due date used to be written after the last
// comma of its line; the check adds the prose so that neither store can drift
// from the other on it again. The second is dated as well, so that the date
// keeps its place after text that carries every mark a sentence carries.
const (
	theProseTasksText      = "post the tweet, then link it: the team's page"
	theDatedProseTasksText = "write the summary, then send it: the user's inbox"
)

// theHeadStartOfTheDatedTask is how long before the check first asks for work
// the dated task is dated: half an hour, so that it is a date to wait for when
// it is added and a date that has come by the time the check asks.
const theHeadStartOfTheDatedTask = 30 * time.Minute

// theJobUnderCheck is what CheckJob knows about the one job it makes, handed
// from one step of the check to the next.
type theJobUnderCheck struct {
	jobID string
	// plain is the first task, whose text carries no punctuation and which the
	// put-down steps are run on.
	plain string
	// dated and prose are the two tasks whose text carries prose punctuation;
	// dated has a date as well, and comes first on the list so that adding
	// prose rewrites its line.
	dated, prose string
	// dueLine is the date the record showed on the dated task when it was
	// added, which every later read must show unchanged.
	dueLine string
	// waiting and behind are the two tasks the lifecycle steps add once the
	// plain task has finished: waiting is dated for a day the check never
	// reaches, and behind is undated and runs ahead of it.
	waiting, behind string
	// now is the moment the check first asks for work. Every later moment is
	// counted on from it.
	now time.Time
}

// afterThePutDownSteps is the moment the check asks for work once the put-down
// steps are over: their last ask was two claims' time on from the first, so
// this is a claim's time later still.
func (under theJobUnderCheck) afterThePutDownSteps() time.Time {
	return under.now.Add(3 * theTimeAClaimIsGiven)
}

// CheckJob asserts what every job store promises: a job needs an ask, a job that
// is not there is an error, a created job is listed as running under the name it
// was given with the tasks that were added to it, its record carries that name,
// and a task's text is kept byte for byte whatever punctuation a sentence puts
// in it. It makes exactly one job, and the tests that call it count on that, so
// the other half of the name rule is CheckJobWithoutAName.
func CheckJob(ctx context.Context, jobs contract.Job) error {
	if _, err := jobs.Create(ctx, contract.NewJob{}); err == nil {
		return errors.New("creating a job with no ask returned no error, and the ask is the user's own words")
	}
	if err := jobs.Pause(ctx, "no-such-job"); err == nil {
		return errors.New("pausing a job that is not there returned no error, and it must name what is missing")
	}

	jobID, err := jobs.Create(ctx, contract.NewJob{Ask: theNamedJobsAsk, Name: theNamedJobsName, Why: "to check the contract"})
	if err != nil {
		return fmt.Errorf("creating a job failed: %w", err)
	}
	taskID, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the one task of the contract check"})
	if err != nil {
		return fmt.Errorf("adding a task failed: %w", err)
	}
	if _, valid := contract.ParseTaskID(taskID); !valid {
		return fmt.Errorf("the task identifier is %q, want the shape the design shows, such as t31", taskID)
	}
	under := theJobUnderCheck{jobID: jobID, plain: taskID, now: time.Now().Add(time.Hour)}
	if err := addTheProseTasks(ctx, jobs, &under); err != nil {
		return err
	}

	listed, err := jobs.List(ctx)
	if err != nil {
		return fmt.Errorf("listing the jobs failed: %w", err)
	}
	for _, summary := range listed {
		if summary.ID != jobID {
			continue
		}
		if summary.State != contract.JobRunning {
			return fmt.Errorf("a new job is %q, want %q", summary.State, contract.JobRunning)
		}
		if summary.TasksTotal != 3 {
			return fmt.Errorf("the job has %d tasks, want the three that were added", summary.TasksTotal)
		}
		if summary.Title != theNamedJobsName {
			return fmt.Errorf("the job is listed under the title %q, want its name %q, because a job with a name lists by the name and not by its ask", summary.Title, theNamedJobsName)
		}
		return checkJobRunsItsTask(ctx, jobs, under)
	}
	return errors.New("a job was created and then was not in the listing")
}

// addTheProseTasks puts the two prose tasks on the job's list, the dated one
// first, and reads the record back to hold each to its text byte for byte and
// the dated one to its date. A store that refuses the punctuation, or keeps
// only part of the text, or takes part of it for a date, fails here.
func addTheProseTasks(ctx context.Context, jobs contract.Job, under *theJobUnderCheck) error {
	dated, err := jobs.AddTask(ctx, contract.NewTask{JobID: under.jobID, Text: theDatedProseTasksText, DueAt: under.now.Add(-theHeadStartOfTheDatedTask)})
	if err != nil {
		return fmt.Errorf("adding the task %q with a date failed: %w; a task's text is prose, and a store takes every punctuation mark a sentence carries, keeping the record's own marks apart from it rather than refusing the text", theDatedProseTasksText, err)
	}
	prose, err := jobs.AddTask(ctx, contract.NewTask{JobID: under.jobID, Text: theProseTasksText})
	if err != nil {
		return fmt.Errorf("adding the task %q failed: %w; a task's text is prose, and a store takes every punctuation mark a sentence carries, keeping the record's own marks apart from it rather than refusing the text", theProseTasksText, err)
	}
	under.dated, under.prose = dated, prose
	dueLine, err := checkTheProseTasksReadBack(ctx, jobs, *under, false)
	if err != nil {
		return err
	}
	under.dueLine = dueLine
	return nil
}

// checkTheProseTasksReadBack loads the job's record and holds the two prose
// tasks to their text byte for byte, the dated one to its date and the other to
// none, and, once they have run, each to the report that finished it. It
// returns the date as the record shows it.
func checkTheProseTasksReadBack(ctx context.Context, jobs contract.Job, under theJobUnderCheck, finished bool) (string, error) {
	record, err := jobs.Load(ctx, under.jobID)
	if err != nil {
		return "", fmt.Errorf("loading the job record failed: %w", err)
	}
	dueLine := ""
	for _, wanted := range []struct {
		id, text string
		dated    bool
	}{{under.dated, theDatedProseTasksText, true}, {under.prose, theProseTasksText, false}} {
		task, found := jobTaskLabelled(record.Work.Tasks, wanted.id)
		if !found {
			return "", fmt.Errorf("the job record lists no task %s, want the one added with the text %q", wanted.id, wanted.text)
		}
		if task.Text != wanted.text {
			return "", fmt.Errorf("the job record lists the task %s as %q, want %q byte for byte, because a task's text is the model's own words and a store keeps every character of it, marking its own lines apart from the text rather than cutting the text", wanted.id, task.Text, wanted.text)
		}
		if wanted.dated && task.DueAt == "" {
			return "", fmt.Errorf("the task %s was added with a date and the job record shows none, so the date was lost; a store writes the date after the task's text and reads it back from there", wanted.id)
		}
		if !wanted.dated && task.DueAt != "" {
			return "", fmt.Errorf("the task %s was added with no date and the job record shows %q as one, so part of its text was taken for a date; a store marks the date apart from the text", wanted.id, task.DueAt)
		}
		if finished && (!task.Done || task.ReportID == "") {
			return "", fmt.Errorf("the task %s finished and the job record shows it as %+v, want it marked done against its report", wanted.id, task)
		}
		if wanted.dated {
			dueLine = task.DueAt
		}
	}
	return dueLine, nil
}

// jobTaskLabelled finds the task with that label on a job's list.
func jobTaskLabelled(tasks []contract.JobTask, taskID string) (contract.JobTask, bool) {
	for _, task := range tasks {
		if task.TaskID == taskID {
			return task, true
		}
	}
	return contract.JobTask{}, false
}

// checkJobRunsItsTask is the second half of CheckJob: the plain task is handed
// out as due, it can be put down and picked up again, its report lands in the
// job with a report id, and the record shows it; then the two prose tasks run
// and the job closes on all three.
func checkJobRunsItsTask(ctx context.Context, jobs contract.Job, under theJobUnderCheck) error {
	next, due, err := jobs.NextTask(ctx, under.now)
	if err != nil {
		return fmt.Errorf("asking for the next task failed: %w", err)
	}
	if !due || next.JobID != under.jobID || next.TaskID != under.plain {
		return fmt.Errorf("the next task is %+v (due %v), want task %s of job %s", next, due, under.plain, under.jobID)
	}
	if err := checkJobHoldsAPutDownTask(ctx, jobs, next, under.now); err != nil {
		return err
	}
	if err := checkJobPicksATaskUpOnce(ctx, jobs, next); err != nil {
		return err
	}
	reportID, err := jobs.FinishTask(ctx, under.jobID, under.plain, "the contract check finished it", false)
	if err != nil {
		return fmt.Errorf("finishing the task failed: %w", err)
	}
	if _, _, valid := contract.ParseReportID(reportID); !valid {
		return fmt.Errorf("the report id is %q, want the shape the design shows, such as j4.2", reportID)
	}
	record, err := jobs.Load(ctx, under.jobID)
	if err != nil {
		return fmt.Errorf("loading the job record failed: %w", err)
	}
	if record.Header.Kind != contract.RecordJob || record.Header.TasksDone != 1 {
		return fmt.Errorf("the job record's header is %+v, want a job with one task done", record.Header)
	}
	if record.Goal.Name != theNamedJobsName {
		return fmt.Errorf("the job record carries the name %q, want %q, because the name given on create is written once into the record and read back with it", record.Goal.Name, theNamedJobsName)
	}
	if len(record.Work.Results) != 1 || record.Work.Results[0].ID != reportID {
		return fmt.Errorf("the job record's reports are %+v, want one with id %s", record.Work.Results, reportID)
	}
	if _, err := jobs.Load(ctx, "no-such-job"); err == nil {
		return errors.New("loading a job that is not there returned no error, and it must name what is missing")
	}
	if err := checkJobRefusesAPutDownOnAFinishedTask(ctx, jobs, under); err != nil {
		return err
	}
	if err := addTheWaitingTasks(ctx, jobs, &under); err != nil {
		return err
	}
	if err := checkJobRunsItsProseTasks(ctx, jobs, under); err != nil {
		return err
	}
	if err := checkJobReadsPastATaskThatWaits(ctx, jobs, under); err != nil {
		return err
	}
	if err := checkAJobThatIsNotRunningDoesNotClose(ctx, jobs, under); err != nil {
		return err
	}
	return checkJobClosesOnItsTasks(ctx, jobs, under.jobID)
}

// checkJobRunsItsProseTasks runs the two prose tasks after the plain one: each
// is handed out in its turn, the dated one first because its date has come,
// carrying its text byte for byte, and once both have finished the record still
// shows the texts, the date as it was first shown, and a report on each.
func checkJobRunsItsProseTasks(ctx context.Context, jobs contract.Job, under theJobUnderCheck) error {
	for _, wanted := range []struct{ id, text string }{{under.dated, theDatedProseTasksText}, {under.prose, theProseTasksText}} {
		next, due, err := jobs.NextTask(ctx, under.afterThePutDownSteps())
		if err != nil {
			return fmt.Errorf("asking for the next task after the plain one finished failed: %w", err)
		}
		if !due || next.TaskID != wanted.id {
			return fmt.Errorf("after the plain task finished the next task is %+v (due %v), want %s, the next on the list whose date has come", next, due, wanted.id)
		}
		if next.Text != wanted.text {
			return fmt.Errorf("the task %s was handed out reading %q, want %q byte for byte, because the loop runs the task on the words the model wrote", wanted.id, next.Text, wanted.text)
		}
		if _, err := jobs.FinishTask(ctx, under.jobID, wanted.id, "the contract check finished it", false); err != nil {
			return fmt.Errorf("finishing the task %s failed: %w", wanted.id, err)
		}
	}
	dueLine, err := checkTheProseTasksReadBack(ctx, jobs, under, true)
	if err != nil {
		return err
	}
	if dueLine != under.dueLine {
		return fmt.Errorf("after the tasks ran the dated task's date reads %q, and it read %q when it was added; the date on a task's line is written once and every later change to the list keeps it", dueLine, under.dueLine)
	}
	return nil
}

// checkJobClosesOnItsTasks is the last part of CheckJob: the last of the job's
// tasks has finished, and a job the model gave no done list closes when its last
// task finishes on one done line per task, in the task's own words and pointing
// at that task's report. A store that never closes such a job leaves every job
// waiting forever, and one that closes it with nothing behind the done list has
// said done without proof.
func checkJobClosesOnItsTasks(ctx context.Context, jobs contract.Job, jobID string) error {
	listed, err := jobs.List(ctx)
	if err != nil {
		return fmt.Errorf("listing the jobs after the last task finished failed: %w", err)
	}
	for _, summary := range listed {
		if summary.ID == jobID && summary.State != contract.JobDone {
			return fmt.Errorf("the job is %q after its last task finished, want %q, because a job with no done list of its own closes on one done line per task", summary.State, contract.JobDone)
		}
	}
	record, err := jobs.Load(ctx, jobID)
	if err != nil {
		return fmt.Errorf("loading the finished job failed: %w", err)
	}
	if record.Header.Status != contract.StatusDone {
		return fmt.Errorf("the finished job's record stands at %q, want %q", record.Header.Status, contract.StatusDone)
	}
	if len(record.Goal.DoneWhen) != len(record.Work.Tasks) {
		return fmt.Errorf("the finished job's done list is %+v, want one line for each of its %d tasks", record.Goal.DoneWhen, len(record.Work.Tasks))
	}
	for at, task := range record.Work.Tasks {
		line := record.Goal.DoneWhen[at]
		if !line.Done || line.ResultID != task.ReportID || line.Text != task.Text {
			return fmt.Errorf("the finished job's done line %+v stands for the task %+v, want it in the task's own words and proved by the task's report", line, task)
		}
	}
	return nil
}

// theNamelessJobsAsk is the ask of the job CheckJobWithoutAName makes, which is
// the whole of what a job with no name is listed by.
const theNamelessJobsAsk = "The contract check made this job without a name."

// CheckJobWithoutAName asserts the other half of the name rule: a job made with
// no name is listed by its whole ask, so that an older job or one the model did
// not name still lists as something, and its record carries no name at all. It
// stands apart from CheckJob because it needs a job of its own, and the tests
// that call CheckJob count the one job it makes. Its job has a schedule as
// well, because CheckJob's job must close and a job with a schedule never
// does, and the run-now rule for a scheduled job is held here on it.
func CheckJobWithoutAName(ctx context.Context, jobs contract.Job) error {
	jobID, err := jobs.Create(ctx, contract.NewJob{
		Ask:          theNamelessJobsAsk,
		Why:          "to check the contract",
		Schedule:     &contract.Schedule{Kind: contract.ScheduleEvery, Every: time.Hour},
		TaskTemplate: theNamelessJobsTemplate,
	})
	if err != nil {
		return fmt.Errorf("creating a job with no name failed: %w", err)
	}
	listed, err := jobs.List(ctx)
	if err != nil {
		return fmt.Errorf("listing the jobs failed: %w", err)
	}
	found := false
	before := time.Time{}
	for _, summary := range listed {
		if summary.ID != jobID {
			continue
		}
		found = true
		before = summary.NextRun
		if summary.Title != theNamelessJobsAsk {
			return fmt.Errorf("the job with no name is listed under the title %q, want its whole ask, because a job given no name lists by its ask so that it still lists as something", summary.Title)
		}
	}
	if !found {
		return errors.New("a job with no name was created and then was not in the listing")
	}
	record, err := jobs.Load(ctx, jobID)
	if err != nil {
		return fmt.Errorf("loading the record of the job with no name failed: %w", err)
	}
	if record.Goal.Name != "" {
		return fmt.Errorf("the record of a job given no name carries the name %q, want none, because the name is only ever the one given on create", record.Goal.Name)
	}
	return checkRunNowPullsTheTickForward(ctx, jobs, jobID, before)
}
