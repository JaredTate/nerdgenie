package main

import (
	"context"
	"sync"

	workingcontext "github.com/JaredTate/coeus/internal/context"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
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
type perTaskContext struct {
	home       contract.Home
	memoryCaps contract.MemoryCaps
	outputCap  int

	guard   sync.Mutex
	current *workingcontext.Builder
}

// newPerTaskContext returns the builder the loop calls.
func newPerTaskContext(home contract.Home, settings contract.Config) *perTaskContext {
	return &perTaskContext{
		home:       home,
		memoryCaps: settings.MemoryCaps,
		outputCap:  settings.Caps.OutputTokensPerCall,
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
	built, err := perTask.builder()
	if err != nil {
		return contract.Request{}, err
	}
	return loop.TheWorkingContext(built).Build(ctx, input)
}

// builder is this task's own builder, made the first time the task asks for it.
func (perTask *perTaskContext) builder() (*workingcontext.Builder, error) {
	perTask.guard.Lock()
	defer perTask.guard.Unlock()

	if perTask.current != nil {
		return perTask.current, nil
	}
	made, err := workingcontext.New(workingcontext.Options{
		Home:            perTask.home,
		MemoryCaps:      perTask.memoryCaps,
		MaxOutputTokens: perTask.outputCap,
	})
	if err != nil {
		return nil, err
	}
	perTask.current = made
	return made, nil
}
