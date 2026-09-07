//go:build integration

// This file is the integration test for jobs: the real event log, the real
// SQLite file in a temporary home, and a second real process reaching for the
// same task. It runs under the integration build tag, which is what "make test"
// and "make check" pass.
package job

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/clock"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/log"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// claimHelperVariable names the environment variable that turns this test binary
// into the second process. Re-invoking the test binary with a variable set is
// the standard Go way to get another process that shares the code under test.
const claimHelperVariable = "NERDGENIE_JOB_CLAIM_HELPER_HOME"

// The two lines the helper prints, so that the parent can read what happened
// without having to guess from an exit code.
const (
	// helperClaimedLine is printed when the helper won the task, with the task's
	// identifier after it.
	helperClaimedLine = "the helper claimed"
	// helperFoundNothingLine is printed when nothing was there to claim.
	helperFoundNothingLine = "the helper found nothing due"
)

// helperWaitLimit is how long the parent gives the helper before giving up on
// it, so that a helper which hangs cannot hang the test run.
const helperWaitLimit = 60 * time.Second

func TestMain(tests *testing.M) {
	if root := os.Getenv(claimHelperVariable); root != "" {
		os.Exit(reachForATask(root))
	}
	os.Exit(tests.Run())
}

// reachForATask is the second process. It opens the same database file the
// parent is using and asks for the next task, printing what it was given.
func reachForATask(root string) int {
	ctx := context.Background()
	home := contract.NewHome(root)
	eventLog, err := log.Open(ctx, home.DatabaseFile())
	if err != nil {
		fmt.Fprintf(os.Stderr, "the claim helper cannot open the log in %s: %v\n", root, err)
		return 1
	}
	defer eventLog.Close()

	jobs, err := Open(ctx, home, eventLog, clock.System())
	if err != nil {
		fmt.Fprintf(os.Stderr, "the claim helper cannot open the jobs in %s: %v\n", root, err)
		return 1
	}
	defer jobs.Close()

	next, due, err := jobs.NextTask(ctx, time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "the claim helper cannot ask for a task: %v\n", err)
		return 1
	}
	if !due {
		fmt.Println(helperFoundNothingLine)
		return 0
	}
	fmt.Println(helperClaimedLine + " " + next.TaskID)
	return 0
}

// openTheRealThing opens an event log and a job store on a real database file in
// a temporary home, on the machine's own clock.
func openTheRealThing(t *testing.T) (contract.Home, *Jobs) {
	t.Helper()
	ctx := t.Context()
	home := testkit.NewTempHome(t)
	eventLog, err := log.Open(ctx, home.DatabaseFile())
	if err != nil {
		t.Fatalf("cannot open the event log: %v", err)
	}
	t.Cleanup(func() { _ = eventLog.Close() })

	jobs, err := Open(ctx, home, eventLog, clock.System())
	if err != nil {
		t.Fatalf("cannot open the jobs: %v", err)
	}
	t.Cleanup(func() { _ = jobs.Close() })
	return home, jobs
}

// askTheHelper runs the second process against the same home and returns the one
// line it printed.
func askTheHelper(t *testing.T, home contract.Home) string {
	t.Helper()
	ctx, stop := context.WithTimeout(t.Context(), helperWaitLimit)
	defer stop()

	helper := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	helper.Env = append(os.Environ(), claimHelperVariable+"="+home.Root)
	helper.Stderr = os.Stderr
	spoken, err := helper.StdoutPipe()
	if err != nil {
		t.Fatalf("cannot listen to the claim helper: %v", err)
	}
	if err := helper.Start(); err != nil {
		t.Fatalf("cannot start the claim helper: %v", err)
	}
	line, err := bufio.NewReader(spoken).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("the claim helper said nothing: %v", err)
	}
	if err := helper.Wait(); err != nil {
		t.Fatalf("the claim helper did not finish cleanly: %v", err)
	}
	return line
}

func TestTwoProcessesCannotClaimOneTask(t *testing.T) {
	ctx := t.Context()
	home, jobs := openTheRealThing(t)
	jobID, err := jobs.Create(ctx, contract.NewJob{Ask: "Do the one task twice over.", Why: "to prove it cannot be"})
	if err != nil {
		t.Fatalf("cannot create the job: %v", err)
	}
	taskID, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the one task both processes want"})
	if err != nil {
		t.Fatalf("cannot add the task: %v", err)
	}

	said := askTheHelper(t, home)

	if said != helperClaimedLine+" "+taskID+"\n" {
		t.Fatalf("the helper said %q, want it to have claimed %s", said, taskID)
	}
	next, due, err := jobs.NextTask(ctx, time.Now())
	if err != nil {
		t.Fatalf("cannot ask for the next task: %v", err)
	}
	if due {
		t.Errorf("this process was given %+v as well, and a task another process is running is nobody else's to start", next)
	}
}

func TestATaskThisProcessHoldsIsNotOfferedToAnother(t *testing.T) {
	ctx := t.Context()
	home, jobs := openTheRealThing(t)
	jobID, err := jobs.Create(ctx, contract.NewJob{Ask: "Do the one task twice over.", Why: "to prove it cannot be"})
	if err != nil {
		t.Fatalf("cannot create the job: %v", err)
	}
	if _, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the one task both processes want"}); err != nil {
		t.Fatalf("cannot add the task: %v", err)
	}
	if _, due, err := jobs.NextTask(ctx, time.Now()); err != nil || !due {
		t.Fatalf("this process was given no task to hold (due %v, error %v)", due, err)
	}

	said := askTheHelper(t, home)

	if said != helperFoundNothingLine+"\n" {
		t.Errorf("the helper said %q, want it to have found nothing, because this process holds the only task", said)
	}
}

// TestTwoStoresReachingForOneTaskTogetherClaimItOnce goes straight at the claim
// itself, which the two tests above never do: each of them reads the claims
// table first and passes over a task somebody holds, so the one line that
// decides who won, the deadline in the claim's WHERE clause, is never asked. The
// real race is two processes that both find an empty claims table and then both
// insert, and this is that race.
func TestTwoStoresReachingForOneTaskTogetherClaimItOnce(t *testing.T) {
	ctx := t.Context()
	home, first := openTheRealThing(t)
	jobID, err := first.Create(ctx, contract.NewJob{Ask: "Do the one task twice over.", Why: "to prove it cannot be"})
	if err != nil {
		t.Fatalf("cannot create the job: %v", err)
	}
	taskID, err := first.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the one task both processes want"})
	if err != nil {
		t.Fatalf("cannot add the task: %v", err)
	}
	second := reopenIn(t, home)
	second.owner = "the other process"

	now := time.Now()
	together := make(chan bool, 2)
	start := make(chan struct{})
	for _, reaching := range []*Jobs{first, second} {
		go func() {
			<-start
			won, err := reaching.claim(ctx, jobID, taskID, now)
			if err != nil {
				t.Errorf("reaching for task %s of job %s failed: %v", taskID, jobID, err)
			}
			together <- won
		}()
	}
	close(start)

	won := 0
	for range 2 {
		if <-together {
			won++
		}
	}
	if won != 1 {
		t.Fatalf("%d of the two stores claimed task %s of job %s, and one task claimed twice is one task run twice", won, taskID, jobID)
	}
	if again, err := second.claim(ctx, jobID, taskID, now.Add(TaskBudget-time.Second)); err != nil || again {
		t.Errorf("the claim was taken again a second before the budget ran out (claimed %v, error %v)", again, err)
	}
	if again, err := second.claim(ctx, jobID, taskID, now.Add(TaskBudget)); err != nil || !again {
		t.Errorf("the claim was not taken once the budget had run out (claimed %v, error %v), and a task whose process died is nobody's", again, err)
	}
}

func TestAWholeJobOnTheRealDatabaseSurvivesARestart(t *testing.T) {
	ctx := t.Context()
	home, jobs := openTheRealThing(t)
	if err := testkit.CheckJob(ctx, jobs); err != nil {
		t.Fatalf("the job store on the real database does not keep the job contract: %v", err)
	}

	jobID, err := jobs.Create(ctx, contract.NewJob{
		Ask:          "Post the morning update every weekday.",
		Schedule:     &contract.Schedule{Kind: contract.ScheduleCron, Cron: "0 7 * * 1-5", Timezone: "America/New_York"},
		TaskTemplate: "write and post today's message",
	})
	if err != nil {
		t.Fatalf("cannot create the scheduled job: %v", err)
	}
	if err := jobs.AppendNote(ctx, jobID, "the last version seen was 8.22.2"); err != nil {
		t.Fatalf("cannot write a note: %v", err)
	}
	if err := jobs.Close(); err != nil {
		t.Fatalf("cannot close the jobs: %v", err)
	}

	reopened := reopenIn(t, home)
	listed, err := reopened.List(ctx)
	if err != nil {
		t.Fatalf("cannot list the jobs after the restart: %v", err)
	}
	// The contract check makes two jobs of its own, the named one and the
	// work order's, and this test makes the scheduled one.
	if len(listed) != 3 {
		t.Fatalf("there are %d jobs after the restart, want the three that were made", len(listed))
	}
	notepad, err := reopened.Notepad(ctx, jobID)
	if err != nil || notepad != "the last version seen was 8.22.2\n" {
		t.Errorf("the notepad after the restart reads %q with error %v", notepad, err)
	}
	if summary := listed[len(listed)-1]; summary.ID != jobID || summary.NextRun.IsZero() {
		t.Errorf("the scheduled job lost its next run across the restart: %+v", summary)
	}
}

// theTimeARestartedStoreIsGiven is how long a store opened after a restart is
// given to hand the cut-off task out: it is one read of the claims table on
// open, so a second is generous.
const theTimeARestartedStoreIsGiven = time.Second

// TestARestartReleasesTheClaimAProcessThatDiedHeld is a real restart in the
// middle of a task: the helper process claims the task and exits, and a store
// opened after it hands the task out within a second rather than after the
// hour the dead claim used to hold it for.
func TestARestartReleasesTheClaimAProcessThatDiedHeld(t *testing.T) {
	ctx := t.Context()
	home, jobs := openTheRealThing(t)
	jobID, err := jobs.Create(ctx, contract.NewJob{Ask: "Do the one task across a restart.", Why: "to prove a restart does not stall it"})
	if err != nil {
		t.Fatalf("cannot create the job: %v", err)
	}
	taskID, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the task the restart cut off"})
	if err != nil {
		t.Fatalf("cannot add the task: %v", err)
	}
	if said := askTheHelper(t, home); said != helperClaimedLine+" "+taskID+"\n" {
		t.Fatalf("the helper said %q, want it to have claimed %s before it died", said, taskID)
	}

	opened := time.Now()
	reopened := reopenIn(t, home)

	next, due, err := reopened.NextTask(ctx, time.Now())
	if err != nil {
		t.Fatalf("cannot ask the reopened store for the next task: %v", err)
	}
	if !due || next.TaskID != taskID {
		t.Errorf("after the restart the next task is %+v (due %v), want %s, which the dead process's claim must not hold for an hour", next, due, taskID)
	}
	if took := time.Since(opened); took > theTimeARestartedStoreIsGiven {
		t.Errorf("the restarted store took %s to hand the task out, want within %s", took.Round(time.Millisecond), theTimeARestartedStoreIsGiven)
	}
}

// TestARestartLeavesAPutDownJobPausedOnItsTask is the put-down rule across a
// real restart: the helper holds the task the person stopped the job on and
// dies, the store opened after it releases that claim, and the job is still
// paused on that task with nothing of it started.
func TestARestartLeavesAPutDownJobPausedOnItsTask(t *testing.T) {
	ctx := t.Context()
	home, jobs := openTheRealThing(t)
	jobID, err := jobs.Create(ctx, contract.NewJob{Ask: "Do the one task the person stopped.", Why: "to prove a restart leaves it put down"})
	if err != nil {
		t.Fatalf("cannot create the job: %v", err)
	}
	taskID, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the task the person stopped"})
	if err != nil {
		t.Fatalf("cannot add the task: %v", err)
	}
	if said := askTheHelper(t, home); said != helperClaimedLine+" "+taskID+"\n" {
		t.Fatalf("the helper said %q, want it to have claimed %s before it died", said, taskID)
	}
	if err := jobs.Pause(ctx, jobID); err != nil {
		t.Fatalf("cannot pause the job on its put-down task: %v", err)
	}

	reopened := reopenIn(t, home)

	listed, err := reopened.List(ctx)
	if err != nil {
		t.Fatalf("cannot list the jobs after the restart: %v", err)
	}
	if listed[0].State != contract.JobPaused {
		t.Errorf("after the restart the put-down job is %q, want it still %q on its task", listed[0].State, contract.JobPaused)
	}
	if next, due, err := reopened.NextTask(ctx, time.Now()); err != nil || due {
		t.Errorf("the reopened store handed out %+v (due %v, error %v), and nothing of a paused job runs on its own", next, due, err)
	}
	if err := reopened.Resume(ctx, jobID); err != nil {
		t.Fatalf("cannot set the job running again: %v", err)
	}
	if next, due, err := reopened.NextTask(ctx, time.Now()); err != nil || !due || next.TaskID != taskID {
		t.Errorf("once the job runs again the next task is %+v (due %v, error %v), want %s at once", next, due, err, taskID)
	}
}

// TestAPutDownMarkIsReadBackAfterARealRestart is the put-down across a real
// restart, the way the person meets it: the helper process runs the task and
// dies, the person had stopped the job on it, and the store opened afterwards
// still says which task the job holds on and which run to pick up, with the
// dead process's claim gone so that the run can carry on at once.
func TestAPutDownMarkIsReadBackAfterARealRestart(t *testing.T) {
	ctx := t.Context()
	home, jobs := openTheRealThing(t)
	jobID, err := jobs.Create(ctx, contract.NewJob{Ask: "Do the one task the person stopped.", Why: "to prove the mark survives"})
	if err != nil {
		t.Fatalf("cannot create the job: %v", err)
	}
	taskID, err := jobs.AddTask(ctx, contract.NewTask{JobID: jobID, Text: "the task the person stopped"})
	if err != nil {
		t.Fatalf("cannot add the task: %v", err)
	}
	if said := askTheHelper(t, home); said != helperClaimedLine+" "+taskID+"\n" {
		t.Fatalf("the helper said %q, want it to have claimed %s before it died", said, taskID)
	}
	mark := contract.PutDownMark{Task: contract.TaskToRun{JobID: jobID, TaskID: taskID, Text: "the task the person stopped"}, Run: "3", HasRecord: true}
	if err := jobs.PutDown(ctx, mark); err != nil {
		t.Fatalf("cannot put the job down on its task: %v", err)
	}

	reopened := reopenIn(t, home)

	held, there, err := reopened.PutDownTask(ctx)
	if err != nil || !there || held != mark {
		t.Errorf("after the restart the put-down task is %+v (there %v, error %v), want %+v", held, there, err, mark)
	}
	if err := reopened.RunNow(ctx, jobID); err != nil {
		t.Fatalf("cannot set the job running again: %v", err)
	}
	if next, due, err := reopened.NextTask(ctx, time.Now()); err != nil || !due || next.TaskID != taskID {
		t.Errorf("once the job runs again the next task is %+v (due %v, error %v), want %s at once, the dead claim gone", next, due, err, taskID)
	}
}

func TestOpeningOnAFileThatIsNotTheEventLogIsRefused(t *testing.T) {
	home := contract.NewHome(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(home.DatabaseFile()), contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the folder for the database file: %v", err)
	}

	_, err := Open(t.Context(), home, testkit.NewFakeStore(), clock.System())

	if err == nil {
		t.Fatal("the jobs opened on a file no event log had made its tables in")
	}
}

// reopenIn opens the log and the jobs again on a home that already holds them.
func reopenIn(t *testing.T, home contract.Home) *Jobs {
	t.Helper()
	ctx := t.Context()
	eventLog, err := log.Open(ctx, home.DatabaseFile())
	if err != nil {
		t.Fatalf("cannot open the event log again: %v", err)
	}
	t.Cleanup(func() { _ = eventLog.Close() })

	jobs, err := Open(ctx, home, eventLog, clock.System())
	if err != nil {
		t.Fatalf("cannot open the jobs again: %v", err)
	}
	t.Cleanup(func() { _ = jobs.Close() })
	return jobs
}
