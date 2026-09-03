package job

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"

	// The pure-Go SQLite driver, registered under the name "sqlite". It is the
	// driver internal/log opens the one database file with, and this package
	// opens the same file to keep the small table of claims on running tasks.
	_ "modernc.org/sqlite"
)

// The bounds this package works inside. Every one of them is here rather than
// spread through the code, because a reader asking "how big can a job get" must
// be able to answer it from one place.
const (
	// MaxJobs is the most jobs one agent holds. A job is a month of work, so a
	// machine holding more than this has a job that is making jobs.
	MaxJobs = 500
	// MaxTasksPerJob is the most tasks one job's list holds. A finished task is
	// never taken off the list, so a schedule that has ticked this many times has
	// filled the job, and the job is switched off with a message asking for a
	// fresh one. Two hundred is where a job record stops fitting in the one to
	// three thousand tokens the design gives it.
	MaxTasksPerJob = 200
	// NotepadBytes is the size of the notepad each job carries between its tasks.
	// The oldest lines are dropped to make room for new ones.
	NotepadBytes = 16 * 1024
	// MaxIncidents is the most distinct failures one job remembers. The oldest is
	// dropped to make room.
	MaxIncidents = 50
	// TaskBudget is how long a claimed task may run before the claim is released
	// and the task counted as a failure. It is the hard stop a task record has.
	TaskBudget = time.Hour
	// TimerClamp is the longest the job store ever sleeps before looking for work
	// again, however far away the next tick is.
	TimerClamp = 60 * time.Second
	// RestBetweenWaits is the shortest time Wait ever takes to come back a second
	// time. Work already past its moment makes the wait nothing at all, so
	// without this rest a driver that cannot take that work yet would ask again
	// as fast as the processor allows.
	RestBetweenWaits = time.Second
	// FailuresThatPause is how many tasks may fail in a row before a plain job is
	// paused, because somebody is there to look at it.
	FailuresThatPause = 3
	// FailuresThatSwitchOff is how many tasks may fail in a row before a
	// scheduled job is switched off, because a schedule is meant to survive a bad
	// afternoon.
	FailuresThatSwitchOff = 10
)

// maxConnections is how many connections to the database file this package
// keeps. A claim is one short statement, so a handful is plenty.
const maxConnections = 4

// databaseOptions are the settings every connection is opened with, and they
// are the ones internal/log and internal/memory use, because all three open the
// same file: wait rather than fail when somebody else holds the write lock, keep
// the write-ahead file so readers never block the writer, and let the operating
// system decide when to flush.
const databaseOptions = "?_pragma=busy_timeout(5000)" +
	"&_pragma=journal_mode(WAL)" +
	"&_pragma=synchronous(NORMAL)"

// Jobs is the agent's jobs: their records, their task lists, the claims on the
// tasks that are running, and the schedules of the ones that have one. It is
// what internal/contract calls a Job.
type Jobs struct {
	home          contract.Home
	eventLog      contract.Store
	clock         contract.Clock
	database      *sql.DB
	owner         string
	guard         sync.Mutex
	order         []string
	held          map[string]*heldJob
	nextJob       int
	closed        bool
	lastWaitEnded time.Time
}

// The job store is the one real job store, so the compiler is asked to say at
// once if it ever stops matching the contract every other package writes
// against.
var _ contract.Job = (*Jobs)(nil)

// heldJob is one job in memory: the keeper that owns its record, and the state
// around the record that the record itself does not carry.
type heldJob struct {
	keeper *record.Keeper
	state  jobState
}

// Open opens the jobs in a home folder: it makes its own table in the one SQLite
// file and rebuilds every job by replaying the event log. The event log must
// already have been opened on the same file, because internal/log owns that
// file's identity, and opening a database that is not a Coeus event log is
// refused with an error saying so.
func Open(ctx context.Context, home contract.Home, eventLog contract.Store, clock contract.Clock) (*Jobs, error) {
	if eventLog == nil {
		return nil, errors.New("cannot open the jobs without an event log, so open internal/log first and pass it in")
	}
	if clock == nil {
		return nil, errors.New("cannot open the jobs without a clock, so pass the clock from internal/clock or a test clock")
	}
	path := home.DatabaseFile()
	if strings.ContainsRune(path, '?') {
		return nil, fmt.Errorf("cannot keep the job claims in %s, because the database driver reads a question mark in a path as the start of its own options, so choose a path without one", path)
	}

	database, err := sql.Open("sqlite", path+databaseOptions)
	if err != nil {
		return nil, fmt.Errorf("cannot open the job claims in %s: %w", path, err)
	}
	database.SetMaxOpenConns(maxConnections)
	database.SetMaxIdleConns(maxConnections)

	opened := &Jobs{
		home: home, eventLog: eventLog, clock: clock, database: database,
		owner: thisProcess(), held: map[string]*heldJob{}, nextJob: 1,
	}
	if err := opened.prepare(ctx); err != nil {
		_ = opened.Close()
		return nil, err
	}
	return opened, nil
}

// Close lets go of every connection to the database file. Closing a job store
// that is already closed does nothing and is not an error.
func (jobs *Jobs) Close() error {
	jobs.guard.Lock()
	defer jobs.guard.Unlock()
	if jobs.closed {
		return nil
	}
	jobs.closed = true
	if err := jobs.database.Close(); err != nil {
		return fmt.Errorf("cannot close the job claims in %s: %w", jobs.home.DatabaseFile(), err)
	}
	return nil
}

// prepare checks that the file really is the event log's file, makes this
// package's own table, and rebuilds every job from the log.
func (jobs *Jobs) prepare(ctx context.Context) error {
	if err := jobs.checkTheEventLogIsThere(ctx); err != nil {
		return err
	}
	if err := jobs.createTables(ctx); err != nil {
		return err
	}
	return jobs.rebuild(ctx)
}

// checkTheEventLogIsThere refuses a database file the event log has not made its
// tables in, because internal/log decides what file this is and a job store on a
// file that is not the log would write claims nobody will ever read.
func (jobs *Jobs) checkTheEventLogIsThere(ctx context.Context) error {
	name := ""
	row := jobs.database.QueryRowContext(ctx,
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'events'")
	switch err := row.Scan(&name); {
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("the file %s is not a Coeus event log yet, so open internal/log on it before the jobs", jobs.home.DatabaseFile())
	case err != nil:
		return fmt.Errorf("cannot read the tables in %s to check that it is a Coeus event log: %w", jobs.home.DatabaseFile(), err)
	default:
		return nil
	}
}

// thisProcess is the name a claim is taken under. Two copies of the agent have
// two process numbers, which is all a claim needs to tell them apart.
func thisProcess() string {
	return "process " + strconv.Itoa(os.Getpid())
}

// jobNumber reads a job identifier as the whole number it is, and says no to
// anything else, so that a job is never looked up by a name the log never wrote.
func jobNumber(jobID string) (int, bool) {
	number, err := strconv.Atoi(jobID)
	if err != nil || number < 1 || jobID != strconv.Itoa(number) {
		return 0, false
	}
	return number, true
}

// find returns the job with that identifier, or an error naming what is missing.
// The caller holds the lock.
func (jobs *Jobs) find(jobID string) (*heldJob, error) {
	held, there := jobs.held[jobID]
	if !there {
		return nil, fmt.Errorf("there is no job numbered %q, so run /jobs to see what there is", jobID)
	}
	return held, nil
}
