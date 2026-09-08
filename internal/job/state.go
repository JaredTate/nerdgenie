package job

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// stateMarker is the word every state event of this package carries. The event
// log holds record changes written by other parts of the agent too, so an event
// without this word under a job's key is somebody else's and is skipped.
const stateMarker = "job state"

// taskFacts are the two things about one task on a job's list that the record
// does not carry exactly: the moment it may start, and whether a schedule made
// it rather than a person.
type taskFacts struct {
	// DueAt is when the task may start, and is the zero time when it may start
	// as soon as the tasks before it are done.
	DueAt time.Time `json:"dueAt,omitzero"`
	// Unattended says a schedule made this task, so nobody is there to answer a
	// preview.
	Unattended bool `json:"unattended,omitempty"`
	// PickedUp says the job has picked this task up itself once, after the
	// harness's guard stopped it, so a second such stop sets it aside.
	PickedUp bool `json:"pickedUp,omitempty"`
	// Deferred says the job has set this task aside once, after the guard
	// stopped it a second time, so NextTask passes it over while another
	// task of the job is unfinished, and a third such stop puts it down.
	Deferred bool `json:"deferred,omitempty"`
	// StartedAt is when the task was first handed out to run, and the zero
	// time while it waits. A task handed out again after a guard stop keeps
	// its first start, so that its total time covers the whole task.
	StartedAt time.Time `json:"startedAt,omitzero"`
	// FinishedAt is when the task's report was taken, and the zero time
	// until then.
	FinishedAt time.Time `json:"finishedAt,omitzero"`
}

// jobState is everything about a job that lives beside its record: where it
// stands, its schedule, how its tasks have been going, and what it has learned
// to carry between them.
type jobState struct {
	// State is running, paused, off, or done.
	State contract.JobState `json:"state"`
	// Schedule is the job's schedule, or nil when it has none.
	Schedule *contract.Schedule `json:"schedule,omitempty"`
	// Template is the text a tick turns into one task.
	Template string `json:"template,omitempty"`
	// Monitor says the job watches for a change rather than reporting every run,
	// so a tick whose report reads the same as the last one is kept quiet.
	Monitor bool `json:"monitor,omitempty"`
	// FailuresInARow counts consecutive failed tasks.
	FailuresInARow int `json:"failuresInARow,omitempty"`
	// KeepRunning says this job is never paused or switched off for failing,
	// however many of its tasks fail in a row.
	KeepRunning bool `json:"keepRunning,omitempty"`
	// StartedAt is when the job was made, and the zero time for a job made
	// before this was kept.
	StartedAt time.Time `json:"startedAt,omitzero"`
	// FinishedAt is when the job closed because every task was done, and the
	// zero time while it runs.
	FinishedAt time.Time `json:"finishedAt,omitzero"`
	// LastRun is when a task of this job last finished.
	LastRun time.Time `json:"lastRun,omitzero"`
	// NextRun is when the schedule fires next.
	NextRun time.Time `json:"nextRun,omitzero"`
	// Backoff is how far the last failed tick pushed the next one back.
	Backoff time.Duration `json:"backoff,omitempty"`
	// LastOutputHash is the hash of the last report a monitored job wrote.
	LastOutputHash string `json:"lastOutputHash,omitempty"`
	// LastOutputReportID is the report that hash belongs to, which is what a
	// quiet tick is marked done against.
	LastOutputReportID string `json:"lastOutputReportId,omitempty"`
	// Incidents is one entry per distinct failure this job has met.
	Incidents []Incident `json:"incidents,omitempty"`
	// Tasks holds the facts about each task that the record does not carry.
	Tasks map[string]taskFacts `json:"tasks,omitempty"`
	// PutDown is the mark the job carries while it is put down on one of its
	// tasks, and nil when it is not. It is in the snapshot so that a restart
	// reads back which task the person's next word or answer picks up.
	PutDown *contract.PutDownMark `json:"putDown,omitempty"`
	// Notepad is what the job's tasks have written down for the ones after them.
	// It is not part of a snapshot, because a note is appended on its own.
	Notepad string `json:"-"`
}

// stateEvent is one change to a job's state. Everything but the notepad is
// written as a whole snapshot, because the state is a few hundred bytes and a
// replay that takes the last snapshot cannot drift; a note is written on its own
// so that a sixteen-kilobyte notepad is not copied into the log on every change.
type stateEvent struct {
	// Marker is the word that says this event belongs to this package.
	Marker string `json:"marker"`
	// Snapshot is the whole of the job's state after the change, or nil when the
	// event is only a note.
	Snapshot *jobState `json:"snapshot,omitempty"`
	// Note is one line appended to the notepad, or empty.
	Note string `json:"note,omitempty"`
}

// facts returns what is known about one task, and the zero facts when the task
// was added before this package knew about it.
func (state jobState) facts(taskID string) taskFacts {
	return state.Tasks[taskID]
}

// withTask returns the state with one task's facts written into it, copying the
// map so that a snapshot already written to the log is never changed behind it.
func (state jobState) withTask(taskID string, facts taskFacts) jobState {
	copied := map[string]taskFacts{}
	for id, held := range state.Tasks {
		copied[id] = held
	}
	copied[taskID] = facts
	state.Tasks = copied
	return state
}

// saveState writes a job's state into the log as a snapshot and keeps it in
// memory only once the log has taken it, so that the two never disagree about
// where a job stands. The caller holds the lock.
func (jobs *Jobs) saveState(ctx context.Context, jobID string, held *heldJob, changed jobState) error {
	changed.Notepad = held.state.Notepad
	if err := jobs.appendState(ctx, jobID, stateEvent{Marker: stateMarker, Snapshot: &changed}); err != nil {
		return err
	}
	held.state = changed
	return nil
}

// appendState writes one state event into the log under the job's key.
func (jobs *Jobs) appendState(ctx context.Context, jobID string, event stateEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("cannot write the state of job %s as JSON: %w", jobID, err)
	}
	logKey := contract.RecordLogKey(contract.RecordJob, jobID)
	_, err = jobs.eventLog.Append(ctx, contract.Event{TaskID: logKey, Kind: contract.EventRecordChange, Body: body})
	if err != nil {
		return fmt.Errorf("cannot write the state of job %s to the log: %w", jobID, err)
	}
	return nil
}

// rebuild reads every job back out of the event log at startup. The log is the
// truth: the record comes from its latest checkpoint and the state around it
// from the last snapshot written, so nothing about a job is ever held only in
// memory.
func (jobs *Jobs) rebuild(ctx context.Context) error {
	states, err := jobs.replayStates(ctx)
	if err != nil {
		return err
	}
	numbers := make([]int, 0, len(states))
	for jobID := range states {
		number, valid := jobNumber(jobID)
		if !valid {
			continue
		}
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)

	for _, number := range numbers {
		jobID := fmt.Sprintf("%d", number)
		keeper, err := record.Load(ctx, jobs.eventLog, contract.RecordJob, jobID)
		if err != nil {
			return fmt.Errorf("cannot rebuild job %s from the log: %w", jobID, err)
		}
		jobs.order = append(jobs.order, jobID)
		jobs.held[jobID] = &heldJob{keeper: keeper, state: states[jobID]}
		jobs.nextJob = number + 1
	}
	return nil
}

// replayStates hands every event of the log past and keeps the last state
// snapshot of each job together with every note appended to its notepad.
func (jobs *Jobs) replayStates(ctx context.Context) (map[string]jobState, error) {
	states := map[string]jobState{}
	err := jobs.eventLog.Replay(ctx, func(event contract.Event) error {
		jobID, itIsAJob := jobIDOfLogKey(event.TaskID)
		if !itIsAJob || event.Kind != contract.EventRecordChange {
			return nil
		}
		change := stateEvent{}
		if err := json.Unmarshal(event.Body, &change); err != nil || change.Marker != stateMarker {
			return nil
		}
		held := states[jobID]
		if change.Snapshot != nil {
			notepad := held.Notepad
			held = *change.Snapshot
			held.Notepad = notepad
		}
		if change.Note != "" {
			held.Notepad = appendToNotepad(held.Notepad, change.Note)
		}
		states[jobID] = held
		if len(states) > MaxJobs {
			return fmt.Errorf("the log holds more than %d jobs, which is more than one agent ever keeps, so start from a fresh database file", MaxJobs)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("cannot replay the event log to rebuild the jobs: %w", err)
	}
	return states, nil
}

// jobIDOfLogKey reads a job's number out of the key its events are logged under,
// which is "j" and the number. A task's events are logged under a bare number
// and belong to no job here.
func jobIDOfLogKey(logKey string) (string, bool) {
	if len(logKey) < 2 || logKey[0] != 'j' {
		return "", false
	}
	jobID := logKey[1:]
	if _, valid := jobNumber(jobID); !valid {
		return "", false
	}
	return jobID, true
}
