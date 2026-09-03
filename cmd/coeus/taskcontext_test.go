package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theQualitySkill is the one skill a fresh home carries, as the store lists it.
var theQualitySkill = contract.SkillSummary{Name: "qa", Description: "Walks an app step by step and reports what it sees."}

// aSkillWrittenLater is a skill a person adds while the agent is running.
var aSkillWrittenLater = contract.SkillSummary{Name: "tidy-notes", Description: "Keeps the notes folder tidy."}

// TestThePerTaskBuilderTellsTheModelWhatSkillsThereAre is what the first human
// trial found: the store could list its skills and nothing asked it to, so the
// model could reach a skill only by already knowing its name. The builder the
// loop calls is handed the store's own list.
func TestThePerTaskBuilderTellsTheModelWhatSkillsThereAre(t *testing.T) {
	store := testkit.NewFakeSkill()
	store.Add(theQualitySkill, "the body")
	perTask := newPerTaskContext(testkit.NewTempHome(t), contract.DefaultConfig(), store, func(string) {})

	request := buildOneCall(t, perTask)

	if !strings.Contains(systemTextOf(request), "qa: Walks an app step by step and reports what it sees.") {
		t.Errorf("the prompt does not name the skill the store lists:\n%s", systemTextOf(request))
	}
}

// TestASkillAddedMidTaskShowsOnTheNextTask holds the two halves of the rule: the
// list is read once per task, because it sits above cache boundary A where
// nothing may move mid-task, and a task that starts later reads it again.
func TestASkillAddedMidTaskShowsOnTheNextTask(t *testing.T) {
	store := testkit.NewFakeSkill()
	store.Add(theQualitySkill, "the body")
	perTask := newPerTaskContext(testkit.NewTempHome(t), contract.DefaultConfig(), store, func(string) {})
	buildOneCall(t, perTask)

	store.Add(aSkillWrittenLater, "the body")
	sameTask := buildOneCall(t, perTask)
	if strings.Contains(systemTextOf(sameTask), "tidy-notes") {
		t.Error("a skill added mid-task moved the top of the prompt, which costs the provider everything under it")
	}

	perTask.startingATask()
	nextTask := buildOneCall(t, perTask)
	if !strings.Contains(systemTextOf(nextTask), "tidy-notes: Keeps the notes folder tidy.") {
		t.Errorf("the next task does not see the skill added before it started:\n%s", systemTextOf(nextTask))
	}
}

// TestASkillListThatCannotBeReadCostsTheTaskItsListAndNothingMore proves a
// skills folder that cannot be read leaves the model working without the list
// rather than unable to be called at all, and that the reason is written where
// the person can read it.
func TestASkillListThatCannotBeReadCostsTheTaskItsListAndNothingMore(t *testing.T) {
	noted := []string{}
	perTask := newPerTaskContext(testkit.NewTempHome(t), contract.DefaultConfig(), unlistableSkills{},
		func(line string) { noted = append(noted, line) })

	request := buildOneCall(t, perTask)

	if len(request.SystemBlocks) == 0 {
		t.Fatal("the call was built with no system prompt at all")
	}
	if strings.Contains(systemTextOf(request), "Your skills") {
		t.Errorf("a list that could not be read still wrote a heading:\n%s", systemTextOf(request))
	}
	if len(noted) == 0 || !strings.Contains(noted[0], "the disk is unplugged") {
		t.Errorf("the reason the skills could not be listed was not noted: %q", noted)
	}
}

// buildOneCall asks the per-task builder for one call's prompt with the least
// input a call can have.
func buildOneCall(t *testing.T, perTask *perTaskContext) contract.Request {
	t.Helper()
	request, err := perTask.Build(context.Background(), loop.BuildInput{
		ContextLength: 24000,
		Messages:      []contract.Message{{Role: contract.RoleUser, Text: "what can you do?"}},
	})
	if err != nil {
		t.Fatalf("cannot build one call's working context: %v", err)
	}
	return request
}

// systemTextOf is everything above the cache line, joined, for a check.
func systemTextOf(request contract.Request) string {
	parts := []string{}
	for _, block := range request.SystemBlocks {
		parts = append(parts, block.Text)
	}
	return strings.Join(parts, "\n\n")
}

// unlistableSkills is a skill store whose folder cannot be read. Nothing but
// List is ever called on it here.
type unlistableSkills struct{ contract.Skill }

// List fails the way the real store does when the folder cannot be read.
func (unlistableSkills) List(context.Context) ([]contract.SkillSummary, error) {
	return nil, errors.New("cannot read the skills folder, because the disk is unplugged")
}
