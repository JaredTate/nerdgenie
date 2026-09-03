// Writing down a resolved element descriptor so that the step can be acted out
// again later with no model call is Stagehand's observe-then-act idea, described
// in docs/research/16-browser-agent-spec.md sections 9.13 and its table of page
// representations. Coeus keeps the observe half in front of every action rather
// than only in front of the first one, because the page it is acting on may have
// been redrawn since the last snapshot.

package browser

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/skill"
)

// SkillSaver is the part of the skill store this package writes through. It is
// one method rather than the whole of contract.Skill, because a recorder that
// could also list, load, and run skills would be able to do more than it needs.
type SkillSaver interface {
	// Save writes a skill folder, whose files are keyed by their names, saying
	// who is saving it.
	Save(ctx context.Context, source contract.SkillSource, name string, files map[string][]byte) error
}

// Recorder drives a browser and writes down what it did, so that the same
// procedure can be replayed later with no model call at all.
//
// A step is written down only when it did what it was said it would do. A
// recording is a procedure that worked, so a click that changed nothing and a
// page that opened somewhere unexpected are handed back to the caller and left
// out of it.
type Recorder struct {
	worker contract.BrowserWorker

	guard sync.Mutex
	steps []Step
}

// NewRecorder returns a recorder over one browser worker.
func NewRecorder(worker contract.BrowserWorker) (*Recorder, error) {
	if worker == nil {
		return nil, errors.New("the recorder was built with no browser behind it, so pass the browser worker to record through")
	}
	return &Recorder{worker: worker}, nil
}

// Steps is everything written down so far, in the order it happened.
func (recorder *Recorder) Steps() []Step {
	recorder.guard.Lock()
	defer recorder.guard.Unlock()
	copied := make([]Step, len(recorder.steps))
	copy(copied, recorder.steps)
	return copied
}

// Open goes to a page and writes the step down when the page is the one the
// caller said to expect.
func (recorder *Recorder) Open(ctx context.Context, intent string, address string, expectation string) (contract.Snapshot, error) {
	if err := recorder.roomForAnother(); err != nil {
		return contract.Snapshot{}, err
	}
	page, err := recorder.worker.Open(ctx, address)
	if err != nil {
		return contract.Snapshot{}, fmt.Errorf("cannot open %s to record it: %w", address, err)
	}
	if StateMet(expectation, page) {
		recorder.write(Step{Intent: intent, Tool: contract.ToolBrowserOpen, Address: address, Expectation: expectation})
	}
	return page, nil
}

// Click clicks one element and writes the step down when the expectation was
// met, with the element's role and name read off the page before the click so
// that the step can be found again three ways.
func (recorder *Recorder) Click(ctx context.Context, intent string, ref string, expectation string) (contract.Diff, error) {
	element, err := recorder.describe(ctx, ref)
	if err != nil {
		return contract.Diff{}, err
	}
	change, err := recorder.worker.Click(ctx, ref, expectation)
	if err != nil {
		return contract.Diff{}, fmt.Errorf("cannot click %s to record it: %w", element, err)
	}
	if change.ExpectationMet {
		recorder.write(Step{Intent: intent, Tool: contract.ToolBrowserClick, Element: element, Expectation: expectation})
	}
	return change, nil
}

// Type types into one element and writes the step down when the expectation was
// met.
func (recorder *Recorder) Type(ctx context.Context, intent string, ref string, text string, expectation string) (contract.Diff, error) {
	element, err := recorder.describe(ctx, ref)
	if err != nil {
		return contract.Diff{}, err
	}
	change, err := recorder.worker.Type(ctx, ref, text, expectation)
	if err != nil {
		return contract.Diff{}, fmt.Errorf("cannot type into %s to record it: %w", element, err)
	}
	if change.ExpectationMet {
		recorder.write(Step{Intent: intent, Tool: contract.ToolBrowserType, Typed: text, Element: element, Expectation: expectation})
	}
	return change, nil
}

// describe reads the page and works out the three ways to find the element
// again. This is the observe half of observe-then-act: the descriptor written
// down is the one the page gave a moment before the action, never one carried
// over from an older snapshot.
func (recorder *Recorder) describe(ctx context.Context, ref string) (Descriptor, error) {
	if err := recorder.roomForAnother(); err != nil {
		return Descriptor{}, err
	}
	page, err := recorder.worker.Read(ctx, contract.ReadOptions{})
	if err != nil {
		return Descriptor{}, fmt.Errorf("cannot read the page before acting on %s, so there is nothing to write the step down from: %w", ref, err)
	}
	for _, element := range page.Elements {
		if element.Ref == ref {
			return Descriptor{Ref: ref, Role: element.Role, Name: element.Name, Shown: element.Name}, nil
		}
	}
	return Descriptor{Ref: ref}, nil
}

// roomForAnother refuses a recording that has already grown past what a skill
// folder holds, so that a run nobody stopped cannot fill the disk.
func (recorder *Recorder) roomForAnother() error {
	recorder.guard.Lock()
	defer recorder.guard.Unlock()
	if len(recorder.steps) >= MaxRecordedSteps {
		return fmt.Errorf("this recording already holds %d steps, which is all a skill folder keeps, so stop it and save what there is",
			MaxRecordedSteps)
	}
	return nil
}

// write adds one step to the recording and numbers it by its place in the list.
func (recorder *Recorder) write(step Step) {
	recorder.guard.Lock()
	defer recorder.guard.Unlock()
	step.Number = len(recorder.steps) + 1
	recorder.steps = append(recorder.steps, step)
}

// Save writes the recording into a skill folder through the store, which keeps
// the copy it replaced and adds the line to the changelog. The dry run is given
// the address the recording opened, so that a recorded skill can be dry run
// without anybody having to write its arguments out again.
func (recorder *Recorder) Save(ctx context.Context, saver SkillSaver, definition skill.Definition) error {
	if saver == nil {
		return errors.New("the recording has no skill store to save into, so pass the store to save through")
	}
	steps := recorder.Steps()
	if len(steps) == 0 {
		return errors.New("this recording has no steps in it, so there is nothing to save and nothing to replay")
	}
	files := map[string][]byte{
		skill.DescriptionFile: skill.RenderDescriptionFile(definition),
		skill.StepsFile:       RenderSteps(steps),
		skill.TestFile:        skill.RenderTestFile(definition.Name, skill.DryRunPlan{Arguments: firstAddress(steps)}),
	}
	return saver.Save(ctx, contract.SkillSavedByPerson, definition.Name, files)
}

// firstAddress is the page the recording opened first, which is what its dry run
// replays against.
func firstAddress(steps []Step) string {
	for _, step := range steps {
		if step.Tool == contract.ToolBrowserOpen {
			return step.Address
		}
	}
	return ""
}
