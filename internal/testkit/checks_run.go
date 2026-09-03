package testkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
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

// CheckJob asserts what every job store promises: a job needs an ask, a job that
// is not there is an error, and a created job is listed as running with the
// tasks that were added to it.
func CheckJob(ctx context.Context, jobs contract.Job) error {
	if _, err := jobs.Create(ctx, contract.NewJob{}); err == nil {
		return errors.New("creating a job with no ask returned no error, and the ask is the user's own words")
	}
	if err := jobs.Pause(ctx, "no-such-job"); err == nil {
		return errors.New("pausing a job that is not there returned no error, and it must name what is missing")
	}

	jobID, err := jobs.Create(ctx, contract.NewJob{Ask: "The contract check made this job.", Why: "to check the contract"})
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
		return checkJobRunsItsTask(ctx, jobs, jobID, taskID)
	}
	return errors.New("a job was created and then was not in the listing")
}

// checkJobRunsItsTask is the second half of CheckJob: the one task is handed out
// as due, its report lands in the job with a report id, and the record shows it.
func checkJobRunsItsTask(ctx context.Context, jobs contract.Job, jobID string, taskID string) error {
	next, due, err := jobs.NextTask(ctx, time.Now().Add(time.Hour))
	if err != nil {
		return fmt.Errorf("asking for the next task failed: %w", err)
	}
	if !due || next.JobID != jobID || next.TaskID != taskID {
		return fmt.Errorf("the next task is %+v (due %v), want task %s of job %s", next, due, taskID, jobID)
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
	if len(record.Work.Results) != 1 || record.Work.Results[0].ID != reportID {
		return fmt.Errorf("the job record's reports are %+v, want one with id %s", record.Work.Results, reportID)
	}
	if _, err := jobs.Load(ctx, "no-such-job"); err == nil {
		return errors.New("loading a job that is not there returned no error, and it must name what is missing")
	}
	return nil
}
