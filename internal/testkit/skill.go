package testkit

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/JaredTate/coeus/internal/contract"
)

// fakeSkillEntry is one skill the fake holds.
type fakeSkillEntry struct {
	summary  contract.SkillSummary
	body     string
	triggers []string
	files    map[string][]byte
}

// FakeSkill holds skills in memory, matches their trigger words, and records
// which of them were run, so that a test can prove a skill ran without the model
// being called.
type FakeSkill struct {
	guard   sync.Mutex
	order   []string
	entries map[string]*fakeSkillEntry
	runs    []string
}

// NewFakeSkill returns an empty skill store.
func NewFakeSkill() *FakeSkill {
	return &FakeSkill{entries: map[string]*fakeSkillEntry{}}
}

// Add puts one skill in the store, with the words that trigger it.
func (skills *FakeSkill) Add(summary contract.SkillSummary, body string, triggers ...string) {
	skills.guard.Lock()
	defer skills.guard.Unlock()
	if _, held := skills.entries[summary.Name]; !held {
		skills.order = append(skills.order, summary.Name)
	}
	skills.entries[summary.Name] = &fakeSkillEntry{summary: summary, body: body, triggers: triggers}
}

// Files is what one saved skill's folder holds, which a test uses to check that
// a learned skill was written properly.
func (skills *FakeSkill) Files(name string) map[string][]byte {
	skills.guard.Lock()
	defer skills.guard.Unlock()
	entry, held := skills.entries[name]
	if !held {
		return nil
	}
	return entry.files
}

// Runs is the name of every skill that was run, in order.
func (skills *FakeSkill) Runs() []string {
	skills.guard.Lock()
	defer skills.guard.Unlock()
	copied := make([]string, len(skills.runs))
	copy(copied, skills.runs)
	return copied
}

// List returns every skill's name and one-liner for the prompt.
func (skills *FakeSkill) List(_ context.Context) ([]contract.SkillSummary, error) {
	skills.guard.Lock()
	defer skills.guard.Unlock()
	listed := make([]contract.SkillSummary, 0, len(skills.order))
	for _, name := range skills.order {
		listed = append(listed, skills.entries[name].summary)
	}
	return listed, nil
}

// Load returns one skill's body.
func (skills *FakeSkill) Load(_ context.Context, name string) (string, error) {
	skills.guard.Lock()
	defer skills.guard.Unlock()
	entry, held := skills.entries[name]
	if !held {
		return "", fmt.Errorf("there is no skill named %q, so list the skills to see what there is", name)
	}
	return entry.body, nil
}

// Run replays a skill and records that it ran.
func (skills *FakeSkill) Run(_ context.Context, name string, arguments string) (string, error) {
	skills.guard.Lock()
	defer skills.guard.Unlock()
	entry, held := skills.entries[name]
	if !held {
		return "", fmt.Errorf("there is no skill named %q, so list the skills to see what there is", name)
	}
	skills.runs = append(skills.runs, name)
	return strings.TrimSpace(entry.summary.Description + " " + arguments), nil
}

// Save writes a skill folder and makes the skill available at once.
func (skills *FakeSkill) Save(_ context.Context, name string, files map[string][]byte) error {
	if name == "" {
		return fmt.Errorf("a skill needs a name before it can be saved, so give this one a folder name")
	}
	if len(files) == 0 {
		return fmt.Errorf("the skill %q has no files, so a skill folder needs at least a SKILL.md in it", name)
	}
	skills.guard.Lock()
	defer skills.guard.Unlock()
	held := skills.entries[name]
	if held == nil {
		skills.order = append(skills.order, name)
	}

	saved := &fakeSkillEntry{
		summary: contract.SkillSummary{Name: name, Description: descriptionIn(files["SKILL.md"])},
		body:    string(files["SKILL.md"]),
		files:   files,
	}
	// Saving over a skill rewrites what the folder holds and nothing else. The
	// trigger words are what the router matches on, and a save that dropped them
	// would quietly switch the skill off.
	if held != nil {
		saved.triggers = held.triggers
		if saved.summary.Description == "" {
			saved.summary.Description = held.summary.Description
		}
	}
	skills.entries[name] = saved
	return nil
}

// Match says which skill's trigger words the message holds, if any.
func (skills *FakeSkill) Match(_ context.Context, text string) (contract.SkillMatch, error) {
	skills.guard.Lock()
	defer skills.guard.Unlock()
	lowered := strings.ToLower(text)
	for _, name := range skills.order {
		for _, trigger := range skills.entries[name].triggers {
			if trigger != "" && strings.Contains(lowered, strings.ToLower(trigger)) {
				return contract.SkillMatch{Name: name, Matched: true}, nil
			}
		}
	}
	return contract.SkillMatch{}, nil
}

// descriptionIn returns the one-line description a skill folder keeps in its
// SKILL.md: the first line that is neither blank nor a heading. The heading is
// the skill's own name, and the name is already the folder's, so a description
// taken from the heading would say nothing at all.
func descriptionIn(content []byte) string {
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return trimmed
	}
	return ""
}
