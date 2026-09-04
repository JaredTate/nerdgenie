package testkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// noSuchProgram is a program name no machine has, which every sandbox must
// refuse whether it is a fake with nothing scripted for it or the real fence
// asking the kernel to run it.
const noSuchProgram = "no-such-program-is-installed-on-any-machine"

// CheckSandbox asserts what every sandbox promises: when it says it is
// unavailable it refuses to run anything, and when it says it can run it still
// refuses a program that is not there rather than reporting a success it never
// had. The second half is what stops a sandbox whose Run is a constant success
// from passing.
func CheckSandbox(ctx context.Context, sandbox contract.Sandbox) error {
	missing := contract.SandboxCommand{Program: noSuchProgram}

	if sandbox.Available() != nil {
		if _, err := sandbox.Run(ctx, missing); err == nil {
			return errors.New("the sandbox says it is unavailable but still ran a command, and an unavailable sandbox must refuse")
		}
		return nil
	}

	result, err := sandbox.Run(ctx, missing)
	if err == nil && result.ExitCode == 0 {
		return fmt.Errorf("the sandbox reported %q as a success, and a program that is on no machine cannot have run", noSuchProgram)
	}
	if reason := sandbox.Available(); reason != nil {
		return fmt.Errorf("the sandbox stopped being available after one command it could not run: %w", reason)
	}
	return nil
}

// CheckSecrets asserts what every vault promises: a reference that is not a
// reference is refused, a name it does not hold is refused, and redacting text
// with no secret in it changes nothing.
func CheckSecrets(ctx context.Context, secrets contract.Secrets) error {
	for _, reference := range []string{"", "not-a-reference", contract.SecretReferencePrefix} {
		if _, err := secrets.Resolve(ctx, reference); err == nil {
			return fmt.Errorf("resolving %q returned no error, and it is not a secret reference", reference)
		}
	}
	if _, err := secrets.Resolve(ctx, contract.SecretReferencePrefix+"no-such-secret-in-the-vault"); err == nil {
		return errors.New("resolving a name the vault does not hold returned no error, and it must name what is missing")
	}

	plain := "there is nothing secret in this sentence"
	if redacted := secrets.Redact(plain); redacted != plain {
		return fmt.Errorf("redacting text with no secret in it changed it to %q", redacted)
	}
	return nil
}

// CheckPermission asserts what every permission function promises: it rules
// allow, ask, or deny and nothing else; a ruling of ask carries a preview; and
// an answer it does not understand is refused.
func CheckPermission(ctx context.Context, permission contract.Permission) error {
	request := contract.PermissionRequest{ToolName: contract.ToolRead, Input: json.RawMessage(`{}`)}

	decision, err := permission.Decide(ctx, request)
	if err != nil {
		return fmt.Errorf("deciding on a read failed: %w", err)
	}
	switch decision.Ruling {
	case contract.RulingAllow, contract.RulingAsk, contract.RulingDeny:
	default:
		return fmt.Errorf("the ruling came back as %q, want allow, ask, or deny", decision.Ruling)
	}
	if decision.Ruling == contract.RulingAsk && decision.PreviewText == "" {
		return errors.New("a ruling of ask came back with no preview text, and the user must see what is about to happen")
	}
	if decision.Ruling == contract.RulingDeny && decision.Reason == "" {
		return errors.New("a ruling of deny came back with no reason, and the model must be told why")
	}

	if err := permission.Remember(request, "maybe", ""); err == nil {
		return errors.New("an answer of \"maybe\" was accepted, and the three answers are once, always, and reject")
	}
	return checkTheThreeAnswers(ctx, permission, request, decision.Ruling)
}

// checkTheThreeAnswers holds the promise contract.Permission makes about
// Remember: always allows the same request for the session, reject denies it
// with the same reason, and once covers one call and changes nothing.
func checkTheThreeAnswers(ctx context.Context, permission contract.Permission, once contract.PermissionRequest, before contract.PermissionRuling) error {
	if err := permission.Remember(once, contract.AnswerOnce, ""); err != nil {
		return fmt.Errorf("remembering an answer of once failed: %w", err)
	}
	after, err := permission.Decide(ctx, once)
	if err != nil {
		return fmt.Errorf("deciding after an answer of once failed: %w", err)
	}
	if after.Ruling != before {
		return fmt.Errorf("an answer of once changed the ruling from %q to %q, and once covers one call only", before, after.Ruling)
	}

	allowed := contract.PermissionRequest{ToolName: contract.ToolWrite, CommandPrefix: "write the contract check's file"}
	if err := permission.Remember(allowed, contract.AnswerAlways, ""); err != nil {
		return fmt.Errorf("remembering an answer of always failed: %w", err)
	}
	decision, err := permission.Decide(ctx, allowed)
	if err != nil {
		return fmt.Errorf("deciding after an answer of always failed: %w", err)
	}
	if decision.Ruling != contract.RulingAllow {
		return fmt.Errorf("the ruling after the user said always is %q, and always allows the same request for the session", decision.Ruling)
	}

	return checkARejectSticks(ctx, permission)
}

// checkARejectSticks is the last half of the answer check: a rejected request
// stays refused, and the refusal carries the user's own reason.
func checkARejectSticks(ctx context.Context, permission contract.Permission) error {
	reason := "the contract check said never to run this"
	refused := contract.PermissionRequest{ToolName: contract.ToolShell, CommandPrefix: "the contract check's command"}

	if err := permission.Remember(refused, contract.AnswerReject, reason); err != nil {
		return fmt.Errorf("remembering an answer of reject failed: %w", err)
	}
	decision, err := permission.Decide(ctx, refused)
	if err != nil {
		return fmt.Errorf("deciding after an answer of reject failed: %w", err)
	}
	if decision.Ruling != contract.RulingDeny {
		return fmt.Errorf("the ruling after the user rejected it is %q, and a reject refuses the same request", decision.Ruling)
	}
	if !strings.Contains(decision.Reason, reason) {
		return fmt.Errorf("the refusal says %q, and it must carry the user's own reason", decision.Reason)
	}
	return nil
}

// CheckToolRegistry asserts what every tool registry promises: every listed tool
// can be found by name, every description is inside the forty-word cap, and a
// name nobody registered is not found.
func CheckToolRegistry(_ context.Context, registry contract.ToolRegistry) error {
	for _, spec := range registry.Specs() {
		if spec.Name == "" {
			return errors.New("a listed tool has no name, and the model calls a tool by its name")
		}
		if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
			return fmt.Errorf("the description of %q is %d words, and the cap is %d", spec.Name, words, contract.MaxToolDescriptionWords)
		}
		if _, found := registry.Lookup(spec.Name); !found {
			return fmt.Errorf("the registry lists %q but cannot find it by name", spec.Name)
		}
	}
	if _, found := registry.Lookup("no-such-tool-was-ever-registered"); found {
		return errors.New("the registry found a tool nobody registered")
	}
	return nil
}

// CheckSkill asserts what every skill store promises: a skill that is not there
// is an error, a save with no files is refused, a save says whether the person
// or the model is saving and is refused when it says neither, and a saved skill
// is listed afterwards.
func CheckSkill(ctx context.Context, skills contract.Skill) error {
	if _, err := skills.Load(ctx, "no-such-skill"); err == nil {
		return errors.New("loading a skill that is not there returned no error, and it must name what is missing")
	}
	if err := skills.Save(ctx, contract.SkillSavedByPerson, "contract-check", nil); err == nil {
		return errors.New("saving a skill with no files returned no error, and a skill folder needs a SKILL.md")
	}
	if err := skills.Save(ctx, contract.SkillSource("nobody"), "contract-check", theContractChecksSkillFolder("contract-check")); err == nil {
		return errors.New("saving a skill that nobody saved returned no error, and every save says whether the person or the model is saving")
	}
	if err := skills.Save(ctx, contract.SkillSavedByModel, "contract-check-by-the-model", theContractChecksSkillFolder("contract-check-by-the-model")); err != nil {
		return fmt.Errorf("saving a skill the model wrote failed: %w", err)
	}

	err := skills.Save(ctx, contract.SkillSavedByPerson, "contract-check", theContractChecksSkillFolder("contract-check"))
	if err != nil {
		return fmt.Errorf("saving a skill folder failed: %w", err)
	}

	listed, err := skills.List(ctx)
	if err != nil {
		return fmt.Errorf("listing the skills failed: %w", err)
	}
	for _, summary := range listed {
		if summary.Name == "contract-check" {
			return nil
		}
	}
	return errors.New("a skill was saved and then was not in the listing")
}

// theContractChecksSkillFolder is the smallest folder a skill store will take,
// which is the one the contract check saves three times over.
func theContractChecksSkillFolder(name string) map[string][]byte {
	return map[string][]byte{
		"SKILL.md": []byte("# " + name + "\nWritten by the contract check.\n"),
	}
}

// The named job the job checks make: a short name, and an ask long enough that
// a store listing by the ask instead of the name shows up in the title.
const (
	theNamedJobsName = "Tater Tots Tetris"
	theNamedJobsAsk  = "Build a Tetris game where every piece is a tater tot, with a score board, a pause key, and a sound when a row clears, then put it on the web so that my nephew can play it on his tablet."
)

// CheckJob asserts what every job store promises: a job needs an ask, a job that
// is not there is an error, a created job is listed as running under the name it
// was given with the tasks that were added to it, and its record carries that
// name. It makes exactly one job, and the tests that call it count on that, so
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
		if summary.TasksTotal != 1 {
			return fmt.Errorf("the job has %d tasks, want the one that was added", summary.TasksTotal)
		}
		if summary.Title != theNamedJobsName {
			return fmt.Errorf("the job is listed under the title %q, want its name %q, because a job with a name lists by the name and not by its ask", summary.Title, theNamedJobsName)
		}
		return checkJobRunsItsTask(ctx, jobs, jobID, taskID)
	}
	return errors.New("a job was created and then was not in the listing")
}

// checkJobRunsItsTask is the second half of CheckJob: the one task is handed out
// as due, it can be put down and picked up again, its report lands in the job
// with a report id, and the record shows it.
func checkJobRunsItsTask(ctx context.Context, jobs contract.Job, jobID string, taskID string) error {
	next, due, err := jobs.NextTask(ctx, time.Now().Add(time.Hour))
	if err != nil {
		return fmt.Errorf("asking for the next task failed: %w", err)
	}
	if !due || next.JobID != jobID || next.TaskID != taskID {
		return fmt.Errorf("the next task is %+v (due %v), want task %s of job %s", next, due, taskID, jobID)
	}
	if err := checkJobHoldsAPutDownTask(ctx, jobs, next); err != nil {
		return err
	}
	reportID, err := jobs.FinishTask(ctx, jobID, taskID, "the contract check finished it", false)
	if err != nil {
		return fmt.Errorf("finishing the task failed: %w", err)
	}
	if _, _, valid := contract.ParseReportID(reportID); !valid {
		return fmt.Errorf("the report id is %q, want the shape the design shows, such as j4.2", reportID)
	}
	record, err := jobs.Load(ctx, jobID)
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
	return checkJobClosesOnItsTasks(ctx, jobs, jobID, reportID)
}

// theRunTheContractCheckPutsDown is the number the contract check says it ran
// the task under when it puts the task down.
const theRunTheContractCheckPutsDown = "7"

// checkJobHoldsAPutDownTask is the middle of CheckJob: a job put down on its
// task is paused and carries the mark, the mark comes back as the one most
// recently put down, and a job set running again forgets it. A store that
// forgets the mark leaves the person's "continue" with nothing to pick up after
// a restart, and one that keeps it past a run-now would pick a task up that is
// already running.
func checkJobHoldsAPutDownTask(ctx context.Context, jobs contract.Job, task contract.TaskToRun) error {
	if err := jobs.PutDown(ctx, contract.PutDownMark{Task: contract.TaskToRun{JobID: "no-such-job", TaskID: task.TaskID}}); err == nil {
		return errors.New("putting down a task of a job that is not there returned no error, and it must name what is missing")
	}
	mark := contract.PutDownMark{Task: task, Run: theRunTheContractCheckPutsDown, HasRecord: true}
	if err := jobs.PutDown(ctx, mark); err != nil {
		return fmt.Errorf("putting the task down failed: %w", err)
	}
	if state, err := stateOfJob(ctx, jobs, task.JobID); err != nil || state != contract.JobPaused {
		return fmt.Errorf("a job put down on its task is %q (error %v), want %q, because nothing of a put-down job runs on its own", state, err, contract.JobPaused)
	}
	held, there, err := jobs.PutDownTask(ctx)
	if err != nil {
		return fmt.Errorf("asking for the put-down task failed: %w", err)
	}
	if !there || held != mark {
		return fmt.Errorf("the put-down task is %+v (there %v), want the mark that was just written, %+v", held, there, mark)
	}
	if err := jobs.RunNow(ctx, task.JobID); err != nil {
		return fmt.Errorf("setting the put-down job running again failed: %w", err)
	}
	if held, there, err := jobs.PutDownTask(ctx); err != nil || there {
		return fmt.Errorf("after the job was set running again the put-down task is %+v (there %v, error %v), want none, because a job set running again forgets its mark", held, there, err)
	}
	return nil
}

// stateOfJob reads one job's state out of the listing.
func stateOfJob(ctx context.Context, jobs contract.Job, jobID string) (contract.JobState, error) {
	listed, err := jobs.List(ctx)
	if err != nil {
		return "", fmt.Errorf("listing the jobs failed: %w", err)
	}
	for _, summary := range listed {
		if summary.ID == jobID {
			return summary.State, nil
		}
	}
	return "", fmt.Errorf("the job %s is not in the listing", jobID)
}

// checkJobClosesOnItsTasks is the last part of CheckJob: the one task was the
// job's last, and a job the model gave no done list closes when its last task
// finishes on one done line per task, each pointing at that task's report. A
// store that never closes such a job leaves every job waiting forever, and one
// that closes it with nothing behind the done list has said done without proof.
func checkJobClosesOnItsTasks(ctx context.Context, jobs contract.Job, jobID string, reportID string) error {
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
	if len(record.Goal.DoneWhen) != 1 {
		return fmt.Errorf("the finished job's done list is %+v, want one line for its one task", record.Goal.DoneWhen)
	}
	if line := record.Goal.DoneWhen[0]; !line.Done || line.ResultID != reportID {
		return fmt.Errorf("the finished job's done line is %+v, want it proved by the task's report %s", line, reportID)
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
// that call CheckJob count the one job it makes.
func CheckJobWithoutAName(ctx context.Context, jobs contract.Job) error {
	jobID, err := jobs.Create(ctx, contract.NewJob{Ask: theNamelessJobsAsk, Why: "to check the contract"})
	if err != nil {
		return fmt.Errorf("creating a job with no name failed: %w", err)
	}
	listed, err := jobs.List(ctx)
	if err != nil {
		return fmt.Errorf("listing the jobs failed: %w", err)
	}
	found := false
	for _, summary := range listed {
		if summary.ID != jobID {
			continue
		}
		found = true
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
	return nil
}
