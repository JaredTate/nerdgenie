// A scheduled job used as a heartbeat that checks the agent on itself is Prime
// Agent's design, from the scheduled runs at
// ~/Code/prime-agent/packages/coding-agent/src/core/scheduler.ts, where a job
// that fires on a clock reports one line rather than a page. The Go here is
// written fresh, and what it checks is Coeus's own memory and skills.

package replay

import (
	"context"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// What the nightly self-check is and when it runs.
const (
	// NightlyAsk is the job's ask, word for word. It is also how the job is
	// found again, so that starting the agent twice does not make two of them.
	NightlyAsk = "Check yourself every night: ask the memory twenty questions and run every skill's dry run."
	// NightlyWhy is the one line on why the user wants it.
	NightlyWhy = "so that a memory or a skill that has quietly stopped working is found before the user needs it"
	// NightlyTaskText is the text of the task the schedule makes each night, and
	// is how the running agent tells that task from a task for the model.
	NightlyTaskText = "Run the nightly self-check."
	// NightlyCron is when it fires: four in the morning, which is an hour after
	// the nightly backup, so the two never run over each other.
	NightlyCron = "0 4 * * *"
)

// How much the nightly self-check does.
const (
	// TwentyQuestions is how many facts the memory is asked about, which is the
	// twenty-question test of docs/WORK_PLAN.md brief 4.5.
	TwentyQuestions = 20
	// AnswersLookedAt is how far down a search a fact may be and still count as
	// found. Three is what the memory hint carries, so a fact below the third
	// would never reach the model on its own.
	AnswersLookedAt = 3
	// MaxSkillsChecked is how many skills one night's run will dry-run, which is
	// the cap internal/skill puts on how many skills there may be.
	MaxSkillsChecked = 200
	// MaxQuestionRunes is how much of a fact is used as the question about it.
	MaxQuestionRunes = 200
	// MaxFailuresNamed is how many failures the one line names before it says
	// how many more there are, because it is one line.
	MaxFailuresNamed = 5
	// MaxNotesKept is how many notes about what could not be checked the one
	// line carries, for the same reason.
	MaxNotesKept = 2
)

// NightlySettings is everything the nightly self-check needs. The job store and
// the way to reach the user are required; the memory, the skills and their dry
// run are each a part of the check that is left out when it is missing. There
// is no clock here, because the job store owns the schedule and reads the time
// for it.
type NightlySettings struct {
	// Jobs is where the nightly job is registered.
	Jobs contract.Job
	// Memory is what the twenty questions are asked of, and is nil on an agent
	// with no memory.
	Memory contract.Memory
	// Skills is the list of skills to dry-run, and is nil on an agent with none.
	Skills contract.Skill
	// DryRun runs one skill's dry run, which contract.Skill does not carry, so
	// cmd/coeus/serve.go passes the skill store's own method here.
	DryRun func(ctx context.Context, name string) (string, error)
	// Send puts the one line in front of the user, on whichever channel they are
	// normally talked to on.
	Send func(ctx context.Context, text string) error
}

// Check is one thing the nightly self-check tried.
type Check struct {
	// Name is what was checked, such as a fact's id or a skill's name.
	Name string
	// Passed says it worked.
	Passed bool
	// Detail says what went wrong when it did not.
	Detail string
}

// Report is what one night's self-check found.
type Report struct {
	// Passed is how many checks worked.
	Passed int
	// Failed is how many did not.
	Failed int
	// Checks is every check, in the order they were made.
	Checks []Check
	// Notes are the few things the self-check could not look at, such as a
	// memory with nothing in it yet, which the one line says out loud so that a
	// night with nothing to check does not read like a night that went well.
	Notes []string
	// Line is the one line the user is sent.
	Line string
}

// Nightly is the self-check and the job that fires it.
type Nightly struct {
	// settings is everything it was built with.
	settings NightlySettings
}

// NewNightly builds the nightly self-check and says which setting is missing
// when one is.
func NewNightly(settings NightlySettings) (*Nightly, error) {
	for _, needed := range []struct {
		name  string
		there bool
	}{
		{"a job store to register the nightly job in", settings.Jobs != nil},
		{"a way to send the user its one line", settings.Send != nil},
	} {
		if !needed.there {
			return nil, fmt.Errorf("the nightly self-check needs %s, so pass one in its settings", needed.name)
		}
	}
	return &Nightly{settings: settings}, nil
}

// Register makes the nightly job if it is not there already and returns its
// number. It is safe to call at every start, because a job whose ask is the
// nightly one is the nightly one.
func (nightly *Nightly) Register(ctx context.Context) (string, error) {
	listed, err := nightly.settings.Jobs.List(ctx)
	if err != nil {
		return "", fmt.Errorf("cannot look for the nightly job among the jobs already there: %w", err)
	}
	for _, summary := range listed {
		if summary.Title == NightlyAsk {
			return summary.ID, nil
		}
	}
	schedule := contract.Schedule{Kind: contract.ScheduleCron, Cron: NightlyCron}
	jobID, err := nightly.settings.Jobs.Create(ctx, contract.NewJob{
		Ask: NightlyAsk, Why: NightlyWhy, Schedule: &schedule, TaskTemplate: NightlyTaskText,
	})
	if err != nil {
		return "", fmt.Errorf("cannot make the nightly job: %w", err)
	}
	return jobID, nil
}

// Handles says whether a task the job store handed out is the nightly
// self-check's own, which is code to run rather than work for the model.
func (nightly *Nightly) Handles(task contract.TaskToRun) bool {
	return strings.TrimSpace(task.Text) == NightlyTaskText
}

// Run makes the night's checks, sends the user one line, and writes that line
// back into the job as the task's report. A task with no job behind it, which
// is what a test or a run by hand gives, is checked and reported and nothing
// more.
func (nightly *Nightly) Run(ctx context.Context, task contract.TaskToRun) (Report, error) {
	report := nightly.check(ctx)
	if err := nightly.settings.Send(ctx, report.Line); err != nil {
		return report, fmt.Errorf("the nightly self-check ran and its line did not reach the user: %w", err)
	}
	if task.JobID == "" || task.TaskID == "" {
		return report, nil
	}
	if _, err := nightly.settings.Jobs.FinishTask(ctx, task.JobID, task.TaskID, report.Line, report.Failed > 0); err != nil {
		return report, fmt.Errorf("the nightly self-check ran and its report did not reach job %s: %w", task.JobID, err)
	}
	return report, nil
}

// check makes every check of one night and writes the one line.
func (nightly *Nightly) check(ctx context.Context) Report {
	report := Report{}
	report.add(nightly.askTheMemory(ctx))
	report.add(nightly.dryRunTheSkills(ctx))
	report.Line = oneLine(report)
	return report
}

// add counts a run of checks into the report and keeps the few notes a passing
// check left behind.
func (report *Report) add(checks []Check) {
	for _, check := range checks {
		report.Checks = append(report.Checks, check)
		if !check.Passed {
			report.Failed++
			continue
		}
		report.Passed++
		if check.Detail != "" && len(report.Notes) < MaxNotesKept {
			report.Notes = append(report.Notes, check.Detail)
		}
	}
}

// oneLine is what the user is sent: how many passed, how many failed, and the
// names of the failures.
func oneLine(report Report) string {
	said := fmt.Sprintf("nightly self-check: %d passed, %d failed", report.Passed, report.Failed)
	if len(report.Notes) > 0 {
		said += "; " + strings.Join(report.Notes, "; ")
	}
	if report.Failed == 0 {
		return said
	}
	named := []string{}
	for _, check := range report.Checks {
		if check.Passed || len(named) >= MaxFailuresNamed {
			continue
		}
		named = append(named, check.Name)
	}
	said += "; these failed: " + strings.Join(named, ", ")
	if report.Failed > len(named) {
		said += fmt.Sprintf(" and %d more", report.Failed-len(named))
	}
	return said
}
