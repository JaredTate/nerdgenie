package channel

import (
	"context"
	"errors"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

func TestASkillTriggerRunsTheSkillWithoutTheModel(t *testing.T) {
	harness := newRouterHarness(t)
	harness.skills.Add(contract.SkillSummary{
		Name:        "weekly-note",
		Description: "posts the weekly note",
	}, "the steps", "weekly note")

	if kind := harness.route(t, "please post the weekly note"); kind != KindSkill {
		t.Errorf("the router made a skill trigger a %s, want a skill", kind)
	}
	if runs := harness.skills.Runs(); len(runs) != 1 || runs[0] != "weekly-note" {
		t.Errorf("the skills that ran were %v, want the weekly note skill", runs)
	}
	if sent := harness.terminal.Sent(); len(sent) != 1 {
		t.Errorf("the skill's answer went out as %v, want one message", sent)
	}
	if len(harness.tasks) != 0 {
		t.Errorf("a skill trigger was also sent to the loop as a task: %v", harness.tasks)
	}
}

func TestASkillThatFailsTellsTheUserAndSaysSo(t *testing.T) {
	harness := newRouterHarness(t)
	router, err := NewRouter(Routes{
		RunCommand:  harness.router.routes.RunCommand,
		FindChannel: harness.router.routes.FindChannel,
		StartTask:   harness.router.routes.StartTask,
		StopTask:    harness.router.routes.StopTask,
		Skills:      alwaysMatchingSkills{},
	})
	if err != nil {
		t.Fatalf("building the router failed: %v", err)
	}

	kind, err := router.Route(context.Background(), anInbound("post the note"))
	if err == nil {
		t.Fatal("a skill that would not run was routed without an error")
	}
	if kind != KindSkill {
		t.Errorf("a failed skill was a %s, want a skill", kind)
	}
	if sent := harness.terminal.Sent(); len(sent) != 1 {
		t.Errorf("the user was told %v, want one line saying the skill could not run", sent)
	}
}

func TestASkillThatAnswersWithNothingSendsNothing(t *testing.T) {
	harness := newRouterHarness(t)
	router, err := NewRouter(Routes{
		RunCommand:  harness.router.routes.RunCommand,
		FindChannel: harness.router.routes.FindChannel,
		StartTask:   harness.router.routes.StartTask,
		StopTask:    harness.router.routes.StopTask,
		Skills:      silentSkill{},
	})
	if err != nil {
		t.Fatalf("building the router failed: %v", err)
	}

	if _, err := router.Route(context.Background(), anInbound("post the note")); err != nil {
		t.Fatalf("routing a skill that answers with nothing failed: %v", err)
	}
	if sent := harness.terminal.Sent(); len(sent) != 0 {
		t.Errorf("a skill that answered with nothing still sent %v", sent)
	}
}

func TestASkillStoreThatCannotAnswerStillLetsTheMessageThrough(t *testing.T) {
	harness := newRouterHarness(t)
	router, err := NewRouter(Routes{
		RunCommand:  harness.router.routes.RunCommand,
		FindChannel: harness.router.routes.FindChannel,
		StartTask:   harness.router.routes.StartTask,
		StopTask:    harness.router.routes.StopTask,
		Skills:      brokenSkills{},
	})
	if err != nil {
		t.Fatalf("building the router failed: %v", err)
	}

	kind, err := router.Route(context.Background(), anInbound("book me a flight"))
	if kind != KindTask {
		t.Errorf("a message the skill store could not answer about became a %s, want a task", kind)
	}
	if err == nil {
		t.Error("the skill store's trouble was swallowed, and the caller has to be told the shortcut is broken")
	}
	if len(harness.tasks) != 1 {
		t.Errorf("the loop was given %d messages, want the one the skill store could not answer about", len(harness.tasks))
	}
}

// brokenSkills is a skill store that cannot answer anything, which is what a
// broken skill folder looks like from the router's side.
type brokenSkills struct{}

// List cannot answer.
func (brokenSkills) List(context.Context) ([]contract.SkillSummary, error) {
	return nil, errors.New("the skills folder cannot be read, so check that it is still there")
}

// Load cannot answer.
func (brokenSkills) Load(context.Context, string) (string, error) {
	return "", errors.New("the skills folder cannot be read, so check that it is still there")
}

// Run cannot answer.
func (brokenSkills) Run(context.Context, string, string) (string, error) {
	return "", errors.New("the skills folder cannot be read, so check that it is still there")
}

// Save cannot answer.
func (brokenSkills) Save(context.Context, contract.SkillSource, string, map[string][]byte) error {
	return errors.New("the skills folder cannot be written, so check that it is still there")
}

// Match cannot answer, which is the one this test needs.
func (brokenSkills) Match(context.Context, string) (contract.SkillMatch, error) {
	return contract.SkillMatch{}, errors.New("the skills folder cannot be read, so check that it is still there")
}

// alwaysMatchingSkills says every message is a skill trigger and then cannot run
// the skill, which is a skill folder whose steps are broken.
type alwaysMatchingSkills struct{ brokenSkills }

// Match says the weekly note skill fired.
func (alwaysMatchingSkills) Match(context.Context, string) (contract.SkillMatch, error) {
	return contract.SkillMatch{Name: "weekly-note", Matched: true}, nil
}

// silentSkill matches every message and runs without anything to say, which is a
// skill whose work is all on the machine.
type silentSkill struct{ alwaysMatchingSkills }

// Run does the work and says nothing.
func (silentSkill) Run(context.Context, string, string) (string, error) {
	return "  ", nil
}
