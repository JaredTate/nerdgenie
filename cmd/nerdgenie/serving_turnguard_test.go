package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/reliability"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aMomentToTakeALease is how long a test waits for a lease it expects to be
// held before it takes the wait itself as the answer.
const aMomentToTakeALease = 200 * time.Millisecond

// aMomentForATurnToBeCut is how long a test waits for a turn the deadline
// should have cut before it calls the turn unbounded.
const aMomentForATurnToBeCut = 5 * time.Second

// anAgentWithATurnLimit opens a whole agent over a temporary home whose
// configuration sets time_per_turn, which is the only way a turn gets a
// deadline at all.
func anAgentWithATurnLimit(t *testing.T, limit time.Duration) *agent {
	t.Helper()
	home := aHomeWithNoModelServer(t)
	settings, err := os.ReadFile(home.ConfigFile())
	if err != nil {
		t.Fatalf("reading the configuration back failed: %v", err)
	}
	settings = append(settings, []byte(fmt.Sprintf("\n[caps]\ntime_per_turn = %q\n", limit.String()))...)
	if err := os.WriteFile(home.ConfigFile(), settings, contract.DataFileMode); err != nil {
		t.Fatalf("writing the turn limit into the configuration failed: %v", err)
	}
	ctx, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)
	running, err := openAgent(ctx, home, func(string) {})
	if err != nil {
		t.Fatalf("opening the agent failed: %v", err)
	}
	if running.settings.Caps.TimePerTurn != limit {
		t.Fatalf("the agent read a turn limit of %s, want the %s the configuration set", running.settings.Caps.TimePerTurn, limit)
	}
	return running
}

// aModelThatWaitsToBeStopped never answers on its own: it waits until the
// context it was called under is cancelled, writes down why, and hands the
// cancellation back, which is what a model call that has hung looks like from
// the loop. The give-up channel is the test's own way out, so a turn nothing
// ever cuts does not hold the test's goroutines forever.
type aModelThatWaitsToBeStopped struct {
	contract.Model
	stoppedBecause chan error
	giveUp         chan struct{}
}

// newModelThatWaitsToBeStopped returns a model with room to write down why
// its calls were stopped.
func newModelThatWaitsToBeStopped() *aModelThatWaitsToBeStopped {
	return &aModelThatWaitsToBeStopped{
		Model:          testkit.NewFakeModel(testkit.Script{Name: "local-coder", ContextLength: 262144}),
		stoppedBecause: make(chan error, 4),
		giveUp:         make(chan struct{}),
	}
}

// Send waits to be stopped and says why it was.
func (model *aModelThatWaitsToBeStopped) Send(ctx context.Context, _ contract.Request, _ func(delta string)) (contract.Reply, error) {
	select {
	case <-ctx.Done():
		model.stoppedBecause <- context.Cause(ctx)
		return contract.Reply{}, ctx.Err()
	case <-model.giveUp:
		return contract.Reply{}, errors.New("the test gave up waiting for the turn to be cut")
	}
}

// whyItWasStopped is the cause the model's call was stopped with, or a failure
// when no call was stopped within the wait.
func (model *aModelThatWaitsToBeStopped) whyItWasStopped(t *testing.T) error {
	t.Helper()
	select {
	case cause := <-model.stoppedBecause:
		return cause
	case <-time.After(aMomentForATurnToBeCut):
		t.Fatal("the model's call was never stopped, so the turn ran with no deadline on it")
		return nil
	}
}

// TestAJobsTaskHoldsTheTurnLeaseWhileItRunsAndReleasesItAfter pins that a
// job's task runs under the same turn lease a person's task does: while the
// task runs, the lease for its session cannot be taken, and once it has ended
// the lease is free again.
func TestAJobsTaskHoldsTheTurnLeaseWhileItRunsAndReleasesItAfter(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()
	aScreenAttachedTo(t, running)
	jobID := aJobWithOneTaskDue(t, running, "post the tweet")
	session := jobSession(contract.TaskToRun{JobID: jobID, TaskID: "t1"})

	takenMeanwhile := false
	running.watched.use(&aModelThatCallsBack{
		Model: testkit.NewFakeModel(testkit.Script{Name: "local-coder", ContextLength: 262144, Steps: []testkit.Step{
			{Text: "The tweet is posted.", Finish: contract.FinishEnd},
			{Text: theReviewAnswer, Finish: contract.FinishEnd},
		}}),
		// A second turn on the same session takes the lease this way, and must
		// find it held for as long as the job's task runs.
		during: func() {
			ctx, giveUp := context.WithTimeout(context.Background(), aMomentToTakeALease)
			defer giveUp()
			if lease, err := running.guard.AcquireTurn(ctx, session); err == nil {
				takenMeanwhile = true
				lease.Release()
			}
		},
	})

	started, err := running.runWhatIsDue(context.Background())
	if err != nil {
		t.Fatalf("running the due task failed: %v", err)
	}
	if !started {
		t.Fatal("the job's task was not started, and it was due")
	}
	if takenMeanwhile {
		t.Errorf("the turn lease for %q could be taken while the job's task was running, so a second turn could write the same record beside it", session)
	}
	lease, err := running.guard.AcquireTurn(context.Background(), session)
	if err != nil {
		t.Fatalf("the turn lease for %q is still held after the job's task ended: %v", session, err)
	}
	lease.Release()
}

// TestAJobsTaskThatOverrunsTheTurnLimitIsStoppedTheWayAPersonsIs pins that the
// turn deadline the user sets holds a job's task exactly as it holds a
// person's: a model call that never answers is cut off by the deadline, with
// the deadline's own error as the cause, the driver comes back, and the loop
// is free for the next task.
func TestAJobsTaskThatOverrunsTheTurnLimitIsStoppedTheWayAPersonsIs(t *testing.T) {
	running := anAgentWithATurnLimit(t, 300*time.Millisecond)
	defer func() { _ = running.close() }()
	aScreenAttachedTo(t, running)
	aJobWithOneTaskDue(t, running, "post the tweet")

	forTheJob := newModelThatWaitsToBeStopped()
	defer close(forTheJob.giveUp)
	running.watched.use(forTheJob)

	cameBack := make(chan struct{})
	go func() {
		defer close(cameBack)
		// A task cut off before its first tool call has no record to mark, so
		// the cut comes back as the driver's error and is not read here: the
		// cause the model saw is what says how the turn ended.
		_, _ = running.runWhatIsDue(context.Background())
	}()
	select {
	case <-cameBack:
	case <-time.After(aMomentForATurnToBeCut):
		t.Fatal("the job driver has not come back after the turn limit, so the job's task ran with no deadline")
	}
	jobsCause := forTheJob.whyItWasStopped(t)
	if !errors.Is(jobsCause, reliability.ErrDeadlineExpired) {
		t.Errorf("the job's task was stopped because %v, want the turn limit the user set", jobsCause)
	}
	if !running.takeTheLoop() {
		t.Fatal("the loop is still held after the job's task was cut off, so nothing could start")
	}
	running.freeTheLoop()

	// A person's task under the same limit is the yardstick: it is cut off
	// the same way, with the same cause.
	forThePerson := newModelThatWaitsToBeStopped()
	defer close(forThePerson.giveUp)
	running.watched.use(forThePerson)
	message := contract.Inbound{Channel: contract.TerminalChannelName, Sender: contract.TerminalChannelName, Text: "post the tweet"}
	if err := running.startTask(context.Background(), newScreenTasks(), message); err != nil {
		t.Fatalf("starting the person's task failed: %v", err)
	}
	personsCause := forThePerson.whyItWasStopped(t)
	waitUntilNotBusy(t, running)
	if !errors.Is(personsCause, reliability.ErrDeadlineExpired) {
		t.Errorf("the person's task was stopped because %v, want the turn limit the user set", personsCause)
	}
	if !errors.Is(jobsCause, personsCause) && !errors.Is(personsCause, jobsCause) {
		t.Errorf("the job's task was stopped because %v and the person's because %v, want the two cut off the same way", jobsCause, personsCause)
	}
}
