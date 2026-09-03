package shell

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// FirstProcessID is the id the first command of a run is given. The ids count
// up from it, and they are the tool's own labels rather than the machine's
// process numbers, because a command inside the fence has a number of its own
// that means nothing outside it.
const FirstProcessID = "p1"

// entry is one command the tool started, whether it has finished or not.
type entry struct {
	id      string
	command string
	timeout time.Duration
	started time.Time
	done    chan struct{}
	stop    context.CancelFunc

	guard    sync.Mutex
	result   contract.SandboxResult
	err      error
	finished bool
	killed   bool
}

// table is the bounded set of commands this tool has started.
type table struct {
	guard   sync.Mutex
	order   []string
	entries map[string]*entry
	counted int
}

// newTable returns an empty table.
func newTable() *table {
	return &table{entries: map[string]*entry{}}
}

// add starts one command in the background and returns the entry standing for
// it, refusing when as many commands are already running as the table holds.
func (running *table) add(command string, timeout time.Duration, now time.Time, work func(ctx context.Context) (contract.SandboxResult, error)) (*entry, error) {
	running.guard.Lock()
	running.forgetFinished()
	if len(running.entries) >= MaxRunning {
		running.guard.Unlock()
		return nil, fmt.Errorf("%d commands are already running and the tool holds %d, so poll or kill one before starting another",
			len(running.entries), MaxRunning)
	}
	running.counted++
	started := &entry{
		id:      fmt.Sprintf("p%d", running.counted),
		command: command,
		timeout: timeoutOr(timeout),
		started: now,
		done:    make(chan struct{}),
	}
	running.order = append(running.order, started.id)
	running.entries[started.id] = started
	running.guard.Unlock()

	ctx, stop := context.WithTimeout(context.Background(), started.timeout)
	started.stop = stop
	go func() {
		defer stop()
		result, err := work(ctx)
		started.guard.Lock()
		started.result, started.err, started.finished = result, err, true
		started.guard.Unlock()
		close(started.done)
	}()
	return started, nil
}

// forgetFinished drops the oldest commands that have finished, so that the table
// stays inside its bound without ever losing one that is still running. The
// caller holds the lock.
func (running *table) forgetFinished() {
	if len(running.entries) < MaxRunning {
		return
	}
	kept := []string{}
	for _, id := range running.order {
		held := running.entries[id]
		if len(running.entries) > MaxRunning/2 && held.hasFinished() {
			delete(running.entries, id)
			continue
		}
		kept = append(kept, id)
	}
	running.order = kept
}

// find returns the entry with this id, or says there is none.
func (running *table) find(id string) (*entry, error) {
	running.guard.Lock()
	defer running.guard.Unlock()
	found, held := running.entries[id]
	if !held {
		return nil, fmt.Errorf("nothing is running under the id %q, and this tool knows about %s",
			id, strings.Join(running.order, ", "))
	}
	return found, nil
}

// poll says whether a command has finished, and what it did when it has. A
// command still running is reported with how long it has run, so that two
// polls never read the same and the loop's guard against a model asking the
// same thing over and over is not tripped by a slow build.
func (running *table) poll(id string, now time.Time) (contract.ToolOutput, error) {
	found, err := running.find(id)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if !found.hasFinished() {
		return contract.ToolOutput{Text: fmt.Sprintf("%s is still running after %s: %s\n", id, now.Sub(found.started).Round(time.Second), oneLine(found.command))}, nil
	}
	return contract.ToolOutput{Text: found.finishedText()}, nil
}

// tail returns what a command has written. The fence hands its output over when
// the command ends rather than as it goes, so a command that is still running
// has nothing to show yet, and this says so.
func (running *table) tail(id string) (contract.ToolOutput, error) {
	found, err := running.find(id)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if !found.hasFinished() {
		return contract.ToolOutput{
			Text: fmt.Sprintf("%s is still running and has written nothing back yet; poll it again in a moment\n", id),
		}, nil
	}
	return contract.ToolOutput{Text: found.finishedText()}, nil
}

// kill stops a command and everything it started.
func (running *table) kill(id string) (contract.ToolOutput, error) {
	found, err := running.find(id)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if found.hasFinished() {
		return contract.ToolOutput{Text: fmt.Sprintf("%s had already finished, so there was nothing to stop\n", id)}, nil
	}
	found.guard.Lock()
	found.killed = true
	found.guard.Unlock()
	if found.stop != nil {
		found.stop()
	}
	return contract.ToolOutput{Text: fmt.Sprintf("%s was stopped, along with anything it started\n", id)}, nil
}

// hasFinished says whether the command behind this entry has ended.
func (entry *entry) hasFinished() bool {
	entry.guard.Lock()
	defer entry.guard.Unlock()
	return entry.finished
}

// oneLine puts a command on a single line so that a line of the result stays one
// line however the model wrote it.
func oneLine(command string) string {
	return strings.Join(strings.Fields(command), " ")
}
