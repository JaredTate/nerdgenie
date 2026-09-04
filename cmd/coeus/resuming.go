package main

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/record"
)

// maxScreensRemembered is how many screens the newest task is remembered for.
// One screen is one channel and one sender, so the terminal is one of them and
// every person who writes over Signal is another. Past this many, the screen
// that has been quiet longest is forgotten, and its next message starts a fresh
// task rather than picking an old one up.
const maxScreensRemembered = 64

// maxTasksLookedBackAt is how many of the newest waiting or stopped tasks a
// start reads out of the log to rebuild this memory: as many as there are
// screens to remember, because each of them can hold one task at most, so a
// long log of abandoned tasks cannot make the start slow.
const maxTasksLookedBackAt = maxScreensRemembered

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
	// interrupted is the numbers of the tasks this start found still at running
	// and reconciled: a shutdown cut them off, so the rebuild marked them stopped
	// and the first status a screen gets names them. It is set once at start and
	// read once by the wiring, so it needs no lock of its own.
	interrupted []string
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
// question and whatever the person typed next is the answer to it. A stopped or
// a failed task is carried on only when the person says one of the few words
// that mean "pick it up", because neither takes an answer and the next thing a
// person types is usually a new ask. A failure is a stop the agent did not
// choose, so it is picked up again the same way a stop is: the work behind it is
// not lost, and "continue" resumes it under the number it already had.
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
	case contract.StatusStopped, contract.StatusFailed:
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

// theStatusQuestions are the whole messages that ask only where the work stands.
// A person who wants to know what is happening says one of these and nothing
// else; anything longer, such as "update the readme to say where we are", is a
// real ask and is handled as one. Each is stored without an apostrophe and in
// lower case, because normalizeQuestion strips both before it looks a message up
// here.
var theStatusQuestions = []string{
	"status",
	"status update",
	"update",
	"update me",
	"any update",
	"where are we",
	"where are we at",
	"where do we stand",
	"what happened",
	"whats happening",
	"whats the status",
	"whats the update",
	"hows it going",
	"progress",
}

// saysStatusQuestion says whether the whole message is one of the ways of asking
// where the work stands, whatever case it was typed in, whether or not its
// apostrophes are there, however its words are spaced, and whatever punctuation
// it ends with. A message that only holds one of these words as part of a longer
// ask is not one, because the whole of it must match.
func saysStatusQuestion(said string) bool {
	return slices.Contains(theStatusQuestions, normalizeQuestion(said))
}

// normalizeQuestion folds a message to the plain form the status questions are
// stored in: lower case, apostrophes removed so that "what's" reads as "whats",
// its words joined by single spaces, and any trailing punctuation dropped.
func normalizeQuestion(said string) string {
	lowered := strings.ToLower(said)
	lowered = strings.ReplaceAll(lowered, "’", "")
	lowered = strings.ReplaceAll(lowered, "'", "")
	collapsed := strings.Join(strings.Fields(lowered), " ")
	return strings.TrimRight(collapsed, ".!?")
}

// rememberedFromTheLog rebuilds the memory of what each screen was last doing
// out of the event log, which is what a restart would otherwise have forgotten:
// for each screen, the newest task whose record stands at waiting, stopped, or
// failed, so that "continue" after a restart picks that task up under the number
// it already had rather than starting a fresh one.
//
// A task still at running was cut off by the shutdown this start recovers from,
// not left running by this session, which has not begun; tasksToPickUp marks it
// stopped and hands it back flagged, so it is carried on the same way and named
// in the interrupted set for the first status.
//
// The log says where each task stands in its newest checkpoint, and whose it is
// in the ask written under its number. A task whose ask is not there is passed
// over, because there is nothing to say which screen may pick it up. A log that
// cannot be read is said so, and the memory handed back is empty rather than
// missing, so the agent still starts and every message starts a fresh task.
func rememberedFromTheLog(ctx context.Context, store contract.Store) (*screenTasks, error) {
	tasks := newScreenTasks()
	standing, err := tasksToPickUp(ctx, store)
	if err != nil {
		return tasks, err
	}
	// The newest tasks are looked at first, as many as there are screens, and
	// then remembered oldest first, so that the newest of a screen's tasks is
	// what stands when two of them could be picked up.
	if len(standing) > maxTasksLookedBackAt {
		standing = standing[len(standing)-maxTasksLookedBackAt:]
	}
	wasInterrupted := map[string]bool{}
	for _, task := range standing {
		number := strconv.Itoa(task.number)
		if task.interrupted {
			wasInterrupted[number] = true
		}
		screen, found := screenOfTheTask(ctx, store, number)
		if !found {
			continue
		}
		tasks.remember(screen, loop.Outcome{TaskID: number, Status: task.status})
	}
	// Only the interrupted tasks that ended as a screen's newest are named,
	// because those are the ones "continue" now carries on: a screen whose newer
	// task is not resumable would be told to continue a task it cannot.
	tasks.interrupted = tasks.interruptedNewest(wasInterrupted)
	return tasks, nil
}

// interruptedNewest is the numbers of the interrupted tasks that each stand as
// their screen's newest, smallest number first, which are the ones "continue"
// now picks up and the first status names.
func (tasks *screenTasks) interruptedNewest(wasInterrupted map[string]bool) []string {
	tasks.guard.Lock()
	defer tasks.guard.Unlock()
	named := []string{}
	for _, held := range tasks.newest {
		if wasInterrupted[held.number] {
			named = append(named, held.number)
		}
	}
	sort.Slice(named, func(first int, second int) bool {
		firstNumber, _ := strconv.Atoi(named[first])
		secondNumber, _ := strconv.Atoi(named[second])
		return firstNumber < secondNumber
	})
	return named
}

// numberedTask is one task, where its record stands, and whether this start had
// to reconcile it from running to stopped because a shutdown cut it off.
type numberedTask struct {
	number      int
	status      contract.RecordStatus
	interrupted bool
}

// tasksToPickUp reads the log once and returns every task a start can pick up,
// oldest first, which is what the newest checkpoint of each task says. A waiting,
// a stopped, and a failed task are all here, because each is carried on again: a
// failure is a stop the agent did not choose, and "continue" must reach it after
// a restart. A task still at running was cut off by a shutdown, so it is marked
// stopped in the log here and handed back flagged interrupted, so a later start
// does not keep finding it running and this start names it. A job's checkpoints
// are left out, because a job's key begins with a letter and a task's is its
// number, and a checkpoint that does not read as a record is passed over rather
// than stopping the rebuild.
func tasksToPickUp(ctx context.Context, store contract.Store) ([]numberedTask, error) {
	saved, err := store.ByKind(ctx, contract.EventCheckpoint)
	if err != nil {
		return nil, fmt.Errorf("cannot read the checkpoints out of the log to see which tasks are waiting: %w", err)
	}
	newest := map[int]record.Checkpoint{}
	for _, event := range saved {
		number, err := strconv.Atoi(event.TaskID)
		if err != nil || number < 1 {
			continue
		}
		one := record.Checkpoint{}
		if err := json.Unmarshal(event.Body, &one); err != nil {
			continue
		}
		if held, known := newest[number]; !known || one.Number >= held.Number {
			newest[number] = one
		}
	}

	found := []numberedTask{}
	for number, checkpoint := range newest {
		held, err := record.Parse([]byte(checkpoint.Text))
		if err != nil {
			continue
		}
		switch held.Header.Status {
		case contract.StatusWaiting, contract.StatusStopped, contract.StatusFailed:
			found = append(found, numberedTask{number: number, status: held.Header.Status})
		case contract.StatusRunning:
			markInterrupted(ctx, store, strconv.Itoa(number))
			found = append(found, numberedTask{number: number, status: contract.StatusStopped, interrupted: true})
		}
	}
	sort.Slice(found, func(first int, second int) bool { return found[first].number < found[second].number })
	return found, nil
}

// markInterrupted marks one task stopped at its last checkpoint, which is how a
// task left running by a shutdown is reconciled on the next start: an unplanned
// stop is a stop, so it is written down as one, and "continue" then reaches it
// the way it reaches any stopped task, with a fresh budget to carry on with.
//
// A write that fails here is left for the next start to reconcile: the task is
// still picked up this session, because the memory holds it as stopped and the
// resume reads the record whatever it stands at, so nothing is lost by carrying
// on past the failure.
func markInterrupted(ctx context.Context, store contract.Store, number string) {
	keeper, err := record.Load(ctx, store, contract.RecordTask, number)
	if err != nil {
		return
	}
	_ = keeper.SetStatus(ctx, contract.StatusStopped)
}

// screenOfTheTask is the screen one task came from, read from the ask written
// under its number, and false when the log holds no such message.
func screenOfTheTask(ctx context.Context, store contract.Store, number string) (string, bool) {
	events, err := store.ByTask(ctx, number)
	if err != nil {
		return "", false
	}
	for _, event := range events {
		if event.Kind != contract.EventMessage {
			continue
		}
		message := contract.Inbound{}
		if err := json.Unmarshal(event.Body, &message); err != nil || message.Channel == "" {
			continue
		}
		return screenNamed(message.Channel, message.Sender), true
	}
	return "", false
}
