package record

import (
	"encoding/json"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestATaskAndAJobWithTheSameNumberKeepSeparateCheckpoints proves the one thing
// the log key is for. Task numbers and job numbers are counted separately, so
// task 17 and job 17 can both exist, and everything either of them writes to the
// log goes under a key that says which of the two it was.
func TestATaskAndAJobWithTheSameNumberKeepSeparateCheckpoints(t *testing.T) {
	store := testkit.NewFakeStore()
	ctx := t.Context()

	task, err := New(ctx, store, Start{
		Kind: contract.RecordTask, ID: "17", Origin: "Signal",
		Ask: "post the anniversary tweet", RoundsLeft: 100, MinutesLeft: 60,
	})
	if err != nil {
		t.Fatalf("cannot create task 17: %v", err)
	}
	job, err := New(ctx, store, Start{
		Kind: contract.RecordJob, ID: "17", Origin: "Signal", Ask: "run the anniversary campaign",
	})
	if err != nil {
		t.Fatalf("cannot create job 17: %v", err)
	}
	if task.LogKey() == job.LogKey() {
		t.Fatalf("task 17 and job 17 share the log key %q, so their events would mix", task.LogKey())
	}

	result, err := task.AddResult(ctx, "read the product notes", "the whole of the product notes")
	if err != nil {
		t.Fatalf("cannot add the task's result: %v", err)
	}
	report, err := job.AddReport(ctx, "the first post went up", "the whole of the first report")
	if err != nil {
		t.Fatalf("cannot add the job's report: %v", err)
	}

	loadedTask, err := Load(ctx, store, contract.RecordTask, "17")
	if err != nil {
		t.Fatalf("cannot load task 17 back: %v", err)
	}
	loadedJob, err := Load(ctx, store, contract.RecordJob, "17")
	if err != nil {
		t.Fatalf("cannot load job 17 back: %v", err)
	}
	if loadedTask.Text() != task.Text() {
		t.Errorf("task 17 came back as something else.\n--- want ---\n%s\n--- got ---\n%s", task.Text(), loadedTask.Text())
	}
	if loadedJob.Text() != job.Text() {
		t.Errorf("job 17 came back as something else.\n--- want ---\n%s\n--- got ---\n%s", job.Text(), loadedJob.Text())
	}
	if loadedTask.LatestCheckpoint() != 2 || loadedJob.LatestCheckpoint() != 2 {
		t.Errorf("task 17 stands at checkpoint %d and job 17 at %d, and each saved two of its own",
			loadedTask.LatestCheckpoint(), loadedJob.LatestCheckpoint())
	}

	if text, err := loadedTask.Read(ctx, result); err != nil || text != "the whole of the product notes" {
		t.Errorf("the task's result %s reads back as %q with the error %v", result, text, err)
	}
	if text, err := loadedJob.Read(ctx, report); err != nil || text != "the whole of the first report" {
		t.Errorf("the job's report %s reads back as %q with the error %v", report, text, err)
	}
	if _, err := loadedTask.Read(ctx, report); err == nil {
		t.Errorf("task 17 read the job's report %s, and the two keep separate stretches of the log", report)
	}
}

// TestWindingOneBackLeavesTheOtherAlone proves two records of the same number are
// wound back separately, which is what "/tasks 17 back 3" must not disturb.
func TestWindingOneBackLeavesTheOtherAlone(t *testing.T) {
	store := testkit.NewFakeStore()
	ctx := t.Context()

	task, err := New(ctx, store, Start{Kind: contract.RecordTask, ID: "17", Ask: "post the tweet", RoundsLeft: 100, MinutesLeft: 60})
	if err != nil {
		t.Fatalf("cannot create task 17: %v", err)
	}
	job, err := New(ctx, store, Start{Kind: contract.RecordJob, ID: "17", Ask: "run the campaign"})
	if err != nil {
		t.Fatalf("cannot create job 17: %v", err)
	}
	if err := task.Apply(ctx, Update{Why: "the task's own why"}); err != nil {
		t.Fatalf("cannot write the task's why: %v", err)
	}
	if err := job.Apply(ctx, Update{Why: "the job's own why"}); err != nil {
		t.Fatalf("cannot write the job's why: %v", err)
	}

	wound, err := Back(ctx, store, contract.RecordTask, "17", 1)
	if err != nil {
		t.Fatalf("cannot wind task 17 back: %v", err)
	}
	if wound.Record().Goal.Why != "" {
		t.Errorf("winding task 17 back left the why %q behind", wound.Record().Goal.Why)
	}
	stillThere, err := Load(ctx, store, contract.RecordJob, "17")
	if err != nil {
		t.Fatalf("cannot load job 17 after task 17 was wound back: %v", err)
	}
	if held := stillThere.Record().Goal.Why; held != "the job's own why" {
		t.Errorf("winding task 17 back changed job 17, whose why now reads %q", held)
	}
}

// TestRefusesACheckpointOfTheOtherKind proves a log that has somehow put a job's
// record under a task's key says so, rather than handing back a record that is not
// the one that was asked for.
func TestRefusesACheckpointOfTheOtherKind(t *testing.T) {
	store := testkit.NewFakeStore()
	ctx := t.Context()

	job, err := New(ctx, store, Start{Kind: contract.RecordJob, ID: "17", Ask: "run the campaign"})
	if err != nil {
		t.Fatalf("cannot create job 17: %v", err)
	}
	body, err := json.Marshal(Checkpoint{Number: 1, Text: job.Text()})
	if err != nil {
		t.Fatalf("cannot build the checkpoint to misfile: %v", err)
	}
	misfiled := contract.Event{
		TaskID: contract.RecordLogKey(contract.RecordTask, "17"),
		Kind:   contract.EventCheckpoint,
		Body:   body,
	}
	if _, err := store.Append(ctx, misfiled); err != nil {
		t.Fatalf("cannot write the misfiled checkpoint: %v", err)
	}
	if _, err := Load(ctx, store, contract.RecordTask, "17"); err == nil {
		t.Error("a job's record loaded as task 17, and the two are different records")
	}
}
