package main

import (
	"slices"
	"strings"
	"sync"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
)

// maxScreensRemembered is how many screens the newest task is remembered for.
// One screen is one channel and one sender, so the terminal is one of them and
// every person who writes over Signal is another. Past this many, the screen
// that has been quiet longest is forgotten, and its next message starts a fresh
// task rather than picking an old one up.
const maxScreensRemembered = 64

// theWordsForCarryingOn are the whole messages that mean "pick up the task you
// put down". A person who wants the stopped task back says one of these and
// nothing else; anything longer is a new ask, even when it begins with one of
// these words.
var theWordsForCarryingOn = []string{"continue", "go on", "carry on", "keep going"}

// endedTask is where one task ended: the number of its record, and whether it is
// waiting for an answer, was stopped, or is finished. The number is empty when
// the task answered without ever making a record, and there is then nothing to
// pick up again.
type endedTask struct {
	number string
	status contract.RecordStatus
}

// screenTasks remembers the newest task each screen started and where that task
// ended, so that the next message from that screen can carry it on instead of
// starting a task of its own.
//
// Only the newest task of each screen is kept. A person answering a question is
// answering the question they were just asked, and a person saying "continue" is
// carrying on the work that just stopped; looking further back would let a
// message land on a task the person had forgotten about.
type screenTasks struct {
	guard  sync.Mutex
	newest map[string]endedTask
	// spokenLast is the screens in the order they last sent something, oldest
	// first, so that the cap forgets the one that has been quiet longest.
	spokenLast []string
}

// newScreenTasks returns an empty memory of what each screen was last doing.
func newScreenTasks() *screenTasks {
	return &screenTasks{newest: map[string]endedTask{}}
}

// screenNamed is the name one screen goes by in this memory: the channel and
// the sender, joined, so that the terminal is one screen and every person
// writing over Signal is another.
func screenNamed(channel string, sender string) string {
	return channel + ":" + sender
}

// forget drops what this screen's newest task was doing, so that the next
// message from that screen starts a fresh task whatever the old one was waiting
// for. It is what the clear command does, because a person who has emptied the
// screen does not want their next words read as the answer to a question they
// can no longer see.
func (tasks *screenTasks) forget(screen string) {
	tasks.guard.Lock()
	defer tasks.guard.Unlock()
	delete(tasks.newest, screen)
	tasks.spokenLast = slices.DeleteFunc(tasks.spokenLast, func(named string) bool { return named == screen })
}

// remember writes down where this screen's newest task ended. It is called with
// whatever the loop came back with, so a task that made no record and a task
// that failed both replace what was there before and leave nothing to pick up.
func (tasks *screenTasks) remember(screen string, outcome loop.Outcome) {
	tasks.guard.Lock()
	defer tasks.guard.Unlock()

	if _, known := tasks.newest[screen]; !known {
		tasks.spokenLast = append(tasks.spokenLast, screen)
	}
	tasks.newest[screen] = endedTask{number: outcome.TaskID, status: outcome.Status}
	for len(tasks.spokenLast) > maxScreensRemembered {
		delete(tasks.newest, tasks.spokenLast[0])
		tasks.spokenLast = tasks.spokenLast[1:]
	}
}

// taskToCarryOn is the number of the task this message picks up again, and is
// empty when the message starts a fresh task.
//
// A waiting task is carried on by any message at all, because the model asked a
// question and whatever the person typed next is the answer to it. A stopped
// task is carried on only when the person says one of the few words that mean
// "pick it up", because a stopped task takes no answer and the next thing a
// person types is usually a new ask.
func (tasks *screenTasks) taskToCarryOn(screen string, said string) string {
	tasks.guard.Lock()
	defer tasks.guard.Unlock()

	last, known := tasks.newest[screen]
	if !known || last.number == "" {
		return ""
	}
	switch last.status {
	case contract.StatusWaiting:
		return last.number
	case contract.StatusStopped:
		if saysCarryOn(said) {
			return last.number
		}
	}
	return ""
}

// howManyScreens is how many screens are remembered, which is what the bound is
// held against.
func (tasks *screenTasks) howManyScreens() int {
	tasks.guard.Lock()
	defer tasks.guard.Unlock()
	return len(tasks.newest)
}

// saysCarryOn says whether the whole message is one of the ways of asking for
// the stopped task back, whatever case it was typed in and whatever punctuation
// it ends with.
func saysCarryOn(said string) bool {
	trimmed := strings.TrimRight(strings.ToLower(strings.TrimSpace(said)), ".!? ")
	return slices.Contains(theWordsForCarryingOn, trimmed)
}
