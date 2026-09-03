package main

import (
	"context"
	"fmt"

	"github.com/JaredTate/coeus/internal/contract"
)

// skillsBox holds the skill store once it exists. The tool registry is built
// before the store, because the store replays through the registry, so the
// registry is handed this box and the box is filled a moment later. Every method
// answers plainly while it is empty rather than failing.
type skillsBox struct {
	store contract.Skill
}

// fill puts the real store in the box, which happens once at startup, before
// anything asks the box a question.
func (box *skillsBox) fill(store contract.Skill) { box.store = store }

// List is the skills there are, and none while the box is empty.
func (box *skillsBox) List(ctx context.Context) ([]contract.SkillSummary, error) {
	if box.store == nil {
		return nil, nil
	}
	return box.store.List(ctx)
}

// Load is one skill's body.
func (box *skillsBox) Load(ctx context.Context, name string) (string, error) {
	if box.store == nil {
		return "", fmt.Errorf("the skills are not open yet, so %q cannot be loaded", name)
	}
	return box.store.Load(ctx, name)
}

// Run replays one skill.
func (box *skillsBox) Run(ctx context.Context, name string, arguments string) (string, error) {
	if box.store == nil {
		return "", fmt.Errorf("the skills are not open yet, so %q cannot be run", name)
	}
	return box.store.Run(ctx, name, arguments)
}

// Save writes one skill folder.
func (box *skillsBox) Save(ctx context.Context, name string, files map[string][]byte) error {
	if box.store == nil {
		return fmt.Errorf("the skills are not open yet, so %q was not written", name)
	}
	return box.store.Save(ctx, name, files)
}

// Match says whether a message's words fire a saved skill.
func (box *skillsBox) Match(ctx context.Context, text string) (contract.SkillMatch, error) {
	if box.store == nil {
		return contract.SkillMatch{}, nil
	}
	return box.store.Match(ctx, text)
}
