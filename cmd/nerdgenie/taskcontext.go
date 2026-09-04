package main

import (
	"context"
	"sync"

	workingcontext "github.com/JaredTate/nerdgenie/internal/context"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
)

// perTaskContext is the working-context builder the loop is given, with a fresh
// builder underneath for every task.
//
// The boundary that marks where a tool result begins and ends is made when a
// builder is made. Made once for the whole program, it would be the same mark in
// every task for as long as the agent ran, and a web page that read it out of one
// task could write the closing mark itself in a later one and have whatever
// followed read as the harness speaking rather than as data. A boundary is only
// a boundary while nothing outside has seen it.
//
// It is rolled when a task starts rather than keyed on the task's number,
// because a task has no number until its first tool call and the calls before
// that belong to it too; a boundary that changed in the middle of a task would
// leave the results already wrapped marked with a boundary the prompt no longer
// names. The loop runs one task at a time, so one builder at a time is all there
// ever is.
//
// The skill list is read at the same moment, once per task, for the same
// reason: it rides above cache boundary A, where nothing may move mid-task, so
// a skill added while a task runs shows on the next task rather than this one.
type perTaskContext struct {
	home       contract.Home
	memoryCaps contract.MemoryCaps
	outputCap  int
	skills     contract.Skill
	note       func(string)

	guard   sync.Mutex
	current *workingcontext.Builder
}

// newPerTaskContext returns the builder the loop calls, over the skill store
// whose list the model is shown and the note function a failure to list is
// written through.
func newPerTaskContext(home contract.Home, settings contract.Config, skills contract.Skill, note func(string)) *perTaskContext {
	return &perTaskContext{
		home:       home,
		memoryCaps: settings.MemoryCaps,
		outputCap:  settings.Caps.OutputTokensPerCall,
		skills:     skills,
		note:       note,
	}
}

// startingATask throws the last task's builder away, so that the next call makes
// a fresh one with a boundary of its own.
func (perTask *perTaskContext) startingATask() {
	perTask.guard.Lock()
	defer perTask.guard.Unlock()
	perTask.current = nil
}

// Build assembles one call's working context, through the builder this task has.
func (perTask *perTaskContext) Build(ctx context.Context, input loop.BuildInput) (contract.Request, error) {
	built, err := perTask.builder(ctx)
	if err != nil {
		return contract.Request{}, err
	}
	return loop.TheWorkingContext(built).Build(ctx, input)
}

// builder is this task's own builder, made the first time the task asks for it,
// with the skill list as it stands at that moment.
func (perTask *perTaskContext) builder(ctx context.Context) (*workingcontext.Builder, error) {
	perTask.guard.Lock()
	defer perTask.guard.Unlock()

	if perTask.current != nil {
		return perTask.current, nil
	}
	made, err := workingcontext.New(workingcontext.Options{
		Home:            perTask.home,
		MemoryCaps:      perTask.memoryCaps,
		MaxOutputTokens: perTask.outputCap,
		Skills:          perTask.listTheSkills(ctx),
	})
	if err != nil {
		return nil, err
	}
	perTask.current = made
	return made, nil
}

// listTheSkills asks the store what skills there are. A folder that cannot be
// listed costs this task its skill list and nothing more: the model still gets
// called, and the reason goes where the person can read it.
func (perTask *perTaskContext) listTheSkills(ctx context.Context) []contract.SkillSummary {
	if perTask.skills == nil {
		return nil
	}
	listed, err := perTask.skills.List(ctx)
	if err != nil && perTask.note != nil {
		perTask.note("the skills could not be listed, so this task's prompt names none of them: " + err.Error())
	}
	if err != nil {
		return nil
	}
	return listed
}
