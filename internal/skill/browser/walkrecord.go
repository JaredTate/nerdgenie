package browser

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/skill"
)

// The bounds a recording keeps, because a person who forgets to type /walk stop
// must not leave something running for ever.
const (
	// LongestRecording is how long a recording watches the browser before it
	// stops itself. What it wrote down by then is still saved by /walk stop.
	LongestRecording = time.Hour
	// waitForTheLastEvents is how long /walk stop waits for the watcher to write
	// down what has already arrived before it saves.
	waitForTheLastEvents = 5 * time.Second
)

// What a recorded step says about itself. A person reads these lines in the
// steps.md afterwards, so they are written for a person and not for the model.
const (
	clickAnswered     = "the page answers the click"
	typingIsHeld      = "the box holds what is typed into it"
	pageIsOnTheScreen = "the page is on the screen"
	typedTextGoesHere = "Type what belongs here. When this was recorded it was %d characters, and what was typed was not written down."
	clickWhatTheySaw  = "Click %s."
	openWhereTheyWent = "Open %s."
	somethingUnnamed  = "the element"
)

// recording is one walk being written down while the person uses the browser
// themselves. There is one of these behind the /walk command at a time, because
// there is one browser window.
type recording struct {
	guard      sync.Mutex
	name       string
	recorder   *Recorder
	definition skill.Definition
	stop       context.CancelFunc
	done       chan struct{}
	problem    error
}

// record starts watching the browser. It writes the page the browser is on down
// as the first step, opens the stream of what the person does, and turns each
// thing they do into a step until they type /walk stop.
func (options WalkOptions) record(ctx context.Context, name string) (string, error) {
	if options.Skills == nil {
		return "", errors.New("the /walk command has no skill store behind it, so there is nowhere to save the walk")
	}
	if options.watching == nil {
		return "", errors.New("this /walk command was not built through WalkCommand, so it has nowhere to keep the recording")
	}
	if running := options.watching.nameOfTheRunningOne(); running != "" {
		return "", fmt.Errorf("the walk %q is already being recorded, so type /walk stop to finish it before starting another", running)
	}
	// The name is checked before anything is watched, so that a name no folder
	// can have is refused now rather than after the person has walked the whole
	// procedure.
	if err := skill.CheckName(name); err != nil {
		return "", fmt.Errorf("the walk cannot be recorded under the name %q: %w", name, err)
	}

	page, err := options.Browser.Read(ctx, contract.ReadOptions{})
	if err != nil {
		return "", fmt.Errorf("there is no page open in the browser to record, so open one first: %w", err)
	}
	recorder, err := NewRecorder(options.Browser)
	if err != nil {
		return "", err
	}
	if err := recorder.writeDown(openingStep(page.URL, "the page "+page.Title+" is on the screen")); err != nil {
		return "", err
	}

	watching, stopWatching := context.WithTimeout(context.Background(), LongestRecording)
	events, err := options.Browser.Events(watching)
	if err != nil {
		stopWatching()
		return "", fmt.Errorf("the browser cannot say what you do in its window, so a walk cannot be recorded by watching: %w", err)
	}
	options.watching.start(name, recorder, definitionOfARecordedWalk(name, page), stopWatching, events)
	return fmt.Sprintf("I am watching the browser window. Everything you click, type, and open is written down as a step of the walk %q, "+
		"starting from the page %s. Type /walk stop when you are done, and I will save it.", name, page.URL), nil
}

// start puts one recording in front of the /walk command and sets the watcher
// going.
func (holder *recording) start(name string, recorder *Recorder, definition skill.Definition,
	stop context.CancelFunc, events <-chan contract.BrowserEvent) {
	holder.guard.Lock()
	defer holder.guard.Unlock()
	holder.name, holder.recorder, holder.definition = name, recorder, definition
	holder.stop, holder.done, holder.problem = stop, make(chan struct{}), nil
	go holder.watch(recorder, holder.done, events)
}

// watch turns each thing the person did into a step, and ends when the person
// types /walk stop, when the recording has grown as large as a walk may be, or
// when the browser worker goes.
func (holder *recording) watch(recorder *Recorder, done chan struct{}, events <-chan contract.BrowserEvent) {
	defer close(done)
	for event := range events {
		step, worthKeeping := stepFor(event)
		if !worthKeeping {
			continue
		}
		if err := recorder.writeDown(step); err != nil {
			holder.sayWhatWentWrong(err)
			return
		}
	}
}

// sayWhatWentWrong keeps the first thing that stopped the watcher, which /walk
// stop passes on to the person.
func (holder *recording) sayWhatWentWrong(problem error) {
	holder.guard.Lock()
	defer holder.guard.Unlock()
	if holder.problem == nil {
		holder.problem = problem
	}
}

// nameOfTheRunningOne is the walk being recorded, and empty when none is.
func (holder *recording) nameOfTheRunningOne() string {
	holder.guard.Lock()
	defer holder.guard.Unlock()
	return holder.name
}

// stopRecording ends the watching, waits for what has already arrived to be
// written down, and hands back the recording so that it can be saved.
func (holder *recording) stopRecording() (string, *Recorder, skill.Definition, error) {
	holder.guard.Lock()
	name, recorder, definition := holder.name, holder.recorder, holder.definition
	stop, done := holder.stop, holder.done
	holder.name, holder.recorder, holder.stop, holder.done = "", nil, nil, nil
	holder.guard.Unlock()

	if name == "" {
		return "", nil, skill.Definition{}, errors.New("nothing is being recorded, so type /walk record with a name to start watching the browser")
	}
	stop()
	select {
	case <-done:
	case <-time.After(waitForTheLastEvents):
	}
	holder.guard.Lock()
	problem := holder.problem
	holder.problem = nil
	holder.guard.Unlock()
	return name, recorder, definition, problem
}

// stopAndSave is /walk stop: it stops watching the browser and saves what was
// written down.
func (options WalkOptions) stopAndSave(ctx context.Context) (string, error) {
	if options.watching == nil {
		return "", errors.New("this /walk command was not built through WalkCommand, so nothing could have been recorded")
	}
	name, recorder, definition, problem := options.watching.stopRecording()
	if recorder == nil {
		return "", problem
	}
	steps := recorder.Steps()
	if err := recorder.Save(ctx, options.Skills, definition); err != nil {
		return "", fmt.Errorf("the walk %q could not be saved: %w", name, err)
	}
	said := fmt.Sprintf("I saved the walk %q with %d steps in it. Read them in %s, fill in anything you typed, and then /walk replay %s walks it again.",
		name, len(steps), skill.StepsFile, name)
	if problem != nil {
		said += " The recording stopped early: " + problem.Error() + "."
	}
	return said, nil
}

// stepFor turns one thing the person did into the step that does it again, and
// says whether it is worth writing down at all.
func stepFor(event contract.BrowserEvent) (Step, bool) {
	switch event.Kind {
	case contract.BrowserEventClick:
		return clickStep(event), true
	case contract.BrowserEventType:
		return typingStep(event), true
	case contract.BrowserEventNavigate:
		if event.Address == "" {
			return Step{}, false
		}
		return openingStep(event.Address, pageIsOnTheScreen), true
	default:
		return Step{}, false
	}
}

// clickStep is what the person clicked, written down so that it can be found
// again by its reference or by the words it showed.
func clickStep(event contract.BrowserEvent) Step {
	what := event.Text
	if what == "" {
		what = somethingUnnamed
	}
	return Step{
		Intent: fmt.Sprintf(clickWhatTheySaw, what),
		Tool:   contract.ToolBrowserClick,
		Element: Descriptor{
			Ref:   event.Ref,
			Name:  event.Text,
			Shown: event.Text,
		},
		Expectation: clickAnswered,
	}
}

// typingStep is the box the person typed into, with the length they typed noted
// and the text left for them to fill in, because what was typed was never sent.
func typingStep(event contract.BrowserEvent) Step {
	return Step{
		Intent:      fmt.Sprintf(typedTextGoesHere, event.Length),
		Tool:        contract.ToolBrowserType,
		Element:     Descriptor{Ref: event.Ref},
		Expectation: typingIsHeld,
	}
}

// openingStep is a page to go to, which is both the first step of a recording
// and what a person taking the window somewhere else becomes.
func openingStep(address string, expectation string) Step {
	return Step{
		Intent:      fmt.Sprintf(openWhereTheyWent, address),
		Tool:        contract.ToolBrowserOpen,
		Address:     address,
		Expectation: expectation,
	}
}
