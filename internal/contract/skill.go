package contract

import (
	"context"
	"errors"
	"time"
)

// SkillSummary is a skill's name and its one-line description, which is all that
// rides in the prompt. The body loads only when the skill is used.
type SkillSummary struct {
	// Name is the skill's folder name.
	Name string
	// Description is the one line from its SKILL.md file.
	Description string
}

// SkillMatch is what the trigger matcher found for one inbound message.
type SkillMatch struct {
	// Name is the skill whose trigger words matched.
	Name string
	// Matched says whether any skill matched at all. When it is false the
	// message goes to the model as a task instead.
	Matched bool
	// Rounds is how many model calls a task run under this skill may make, and
	// is zero when the skill sets none, which means the caps in the
	// configuration apply. Design section 3, rule 3: every task has a budget,
	// and a skill can set its own.
	Rounds int
	// Time is how long such a task may take, and is zero the same way.
	Time time.Duration
}

// SkillSource says who saved a skill. It rides with every save, because a skill
// the model wrote may be a skill a page the model read asked it to write, and
// such a skill is trusted with less than one a person saved.
type SkillSource string

const (
	// SkillSavedByPerson is a skill the person at the screen saved or said yes
	// to.
	SkillSavedByPerson SkillSource = "person"
	// SkillSavedByModel is a skill the model wrote, through its own skill tool
	// or by learning from a page.
	SkillSavedByModel SkillSource = "model"
)

// KnownSkillSource says whether the source is one of the two a save may have.
func KnownSkillSource(source SkillSource) bool {
	return source == SkillSavedByPerson || source == SkillSavedByModel
}

// Skill is a saved procedure for one kind of job, stored as a folder holding a
// description, the steps or a script, a dry-run test, and a changelog.
type Skill interface {
	// List returns every skill's name and one-liner for the prompt.
	List(ctx context.Context) ([]SkillSummary, error)
	// Load returns one skill's body.
	Load(ctx context.Context, name string) (string, error)
	// Run replays a skill without calling the model, and calls the model only
	// when a step fails.
	Run(ctx context.Context, name string, arguments string) (string, error)
	// Save writes a skill folder, whose files are keyed by their names, saying
	// who is saving it.
	Save(ctx context.Context, source SkillSource, name string, files map[string][]byte) error
	// Match is what the router calls on every message to see whether a skill's
	// trigger words fire.
	Match(ctx context.Context, text string) (SkillMatch, error)
}

// ErrNothingToDryRun is what a skill's dry run wraps when the skill has
// nothing to replay: a skill that is its SKILL.md alone, or one that takes an
// argument its test file does not give. The nightly self-check reads it to
// note the skill rather than count it broken.
var ErrNothingToDryRun = errors.New("there is nothing to dry-run")
