package contract

import "context"

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
	// Save writes a skill folder, whose files are keyed by their names.
	Save(ctx context.Context, name string, files map[string][]byte) error
	// Match is what the router calls on every message to see whether a skill's
	// trigger words fire.
	Match(ctx context.Context, text string) (SkillMatch, error)
}
