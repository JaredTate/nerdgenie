package record

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// taskStart is the task the tests in this package start from: the tweet the
// forty-step fixture is built on.
func taskStart() Start {
	return Start{
		Kind:        contract.RecordTask,
		ID:          "17",
		Origin:      "Signal",
		Ask:         "Post a tweet about the DigiByte anniversary.",
		RoundsLeft:  100,
		MinutesLeft: 60,
	}
}

// jobStart is the job the tests start from.
func jobStart() Start {
	return Start{Kind: contract.RecordJob, ID: "4", Origin: "Signal", Ask: "Run the anniversary campaign this month."}
}

// newKeeper makes a keeper over an empty log, and fails the test if it cannot.
func newKeeper(t *testing.T, start Start) (*Keeper, *testkit.FakeStore) {
	t.Helper()
	keeper, err := newKeeperOrError(t, start)
	if err != nil {
		t.Fatalf("cannot create the %s record: %v", start.Kind, err)
	}
	return keeper, keeper.store.(*testkit.FakeStore)
}

// newKeeperOrError makes a keeper over an empty log and hands back whatever
// happened, for the tests that expect the creation itself to be refused.
func newKeeperOrError(t *testing.T, start Start) (*Keeper, error) {
	t.Helper()
	return New(t.Context(), testkit.NewFakeStore(), start)
}

// TestCreatesARecordOnTheFirstToolCall proves rule six: a record is created with
// the task's number, where the ask came from, the ask itself, and the budget.
func TestCreatesARecordOnTheFirstToolCall(t *testing.T) {
	keeper, store := newKeeper(t, taskStart())
	held := keeper.Record()

	if held.Header.Kind != contract.RecordTask || held.Header.ID != "17" || held.Header.Origin != "Signal" {
		t.Errorf("the header is %+v, and it should say task 17 from Signal", held.Header)
	}
	if held.Header.Status != contract.StatusRunning {
		t.Errorf("a new record stands at %q, and it should be running", held.Header.Status)
	}
	if held.Header.RoundsLeft != 100 || held.Header.MinutesLeft != 60 {
		t.Errorf("the budget is %d rounds and %d minutes, and it should be the one it was given",
			held.Header.RoundsLeft, held.Header.MinutesLeft)
	}
	if held.Goal.Ask != taskStart().Ask {
		t.Errorf("the ask is %q, and the user wrote %q", held.Goal.Ask, taskStart().Ask)
	}
	if keeper.LatestCheckpoint() != 1 {
		t.Errorf("a new record stands at checkpoint %d, and creating one saves the first", keeper.LatestCheckpoint())
	}
	if store.Count() != 1 {
		t.Errorf("creating a record wrote %d events to the log, and it should write one checkpoint", store.Count())
	}
}

// TestAJobIsCreatedWithNoBudgetOnIt proves a record never holds anything its own
// text does not print, because the two must always say the same thing: a job
// carries progress where a task carries a budget, so a budget handed to a job is
// dropped rather than kept where nothing would ever show it.
func TestAJobIsCreatedWithNoBudgetOnIt(t *testing.T) {
	start := jobStart()
	start.RoundsLeft, start.MinutesLeft = 100, 60
	keeper, _ := newKeeper(t, start)

	held := keeper.Record()
	if held.Header.RoundsLeft != 0 || held.Header.MinutesLeft != 0 {
		t.Errorf("the job holds a budget of %d rounds and %d minutes, which its text never prints",
			held.Header.RoundsLeft, held.Header.MinutesLeft)
	}
	parsed, err := Parse([]byte(keeper.Text()))
	if err != nil {
		t.Fatalf("the job record does not read back: %v", err)
	}
	if !reflect.DeepEqual(parsed, held) {
		t.Errorf("the job record and its text do not say the same thing.\nheld   %+v\nparsed %+v", held, parsed)
	}
}

// TestSavesTheRecordItselfIntoEveryCheckpoint proves a checkpoint holds the
// printed record, so that reloading one needs nothing but the log.
func TestSavesTheRecordItselfIntoEveryCheckpoint(t *testing.T) {
	keeper, store := newKeeper(t, taskStart())
	saved := latestCheckpoint(t, store, "17")
	if saved.Number != 1 {
		t.Errorf("the first checkpoint is numbered %d", saved.Number)
	}
	if saved.Text != keeper.Text() {
		t.Errorf("the checkpoint holds text the keeper does not.\n--- saved ---\n%s\n--- held ---\n%s", saved.Text, keeper.Text())
	}
}

// TestRefusesToCreateARecordWithoutWhatItNeeds walks the bad ways to start one.
func TestRefusesToCreateARecordWithoutWhatItNeeds(t *testing.T) {
	cases := map[string]Start{
		"a kind that is neither task nor job": {Kind: "thing", ID: "1", Ask: "do it"},
		"no number":                           {Kind: contract.RecordTask, Ask: "do it"},
		"a number that is not a number":       {Kind: contract.RecordJob, ID: "four", Ask: "do it"},
		"a number below one":                  {Kind: contract.RecordTask, ID: "0", Ask: "do it"},
		"no ask":                              {Kind: contract.RecordTask, ID: "1"},
		"a budget below zero":                 {Kind: contract.RecordTask, ID: "1", Ask: "do it", RoundsLeft: -1},
	}
	for name, start := range cases {
		if _, err := New(t.Context(), testkit.NewFakeStore(), start); err == nil {
			t.Errorf("a record was created with %s", name)
		}
	}
	if _, err := New(t.Context(), nil, taskStart()); err == nil {
		t.Error("a record was created with no log behind it")
	}
}

// TestTheHarnessWritesTheHeaderAndTheSituation proves rules seven and eight: the
// budget, the cost, and the facts the harness can check are written by code, and
// each change saves the next checkpoint.
func TestTheHarnessWritesTheHeaderAndTheSituation(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()

	if err := keeper.SetBudget(ctx, 86, 51); err != nil {
		t.Fatalf("cannot write the budget: %v", err)
	}
	if err := keeper.SetCost(ctx, contract.CostLine{InputTokens: 6147, CachedInputTokens: 5200, OutputTokens: 400}); err != nil {
		t.Fatalf("cannot write the cost: %v", err)
	}
	if err := keeper.SetSituation(ctx, []string{"browser tab t1: x.com/compose", "files changed in this task: none"}); err != nil {
		t.Fatalf("cannot write the situation: %v", err)
	}

	held := keeper.Record()
	if held.Header.RoundsLeft != 86 || held.Header.MinutesLeft != 51 {
		t.Errorf("the budget is %d rounds and %d minutes", held.Header.RoundsLeft, held.Header.MinutesLeft)
	}
	if held.Header.Cost.InputTokens != 6100 {
		t.Errorf("the cost line holds %d tokens in, and the record keeps them to the tenth of a thousand it prints",
			held.Header.Cost.InputTokens)
	}
	if len(held.Work.Situation) != 2 {
		t.Errorf("the situation has %d lines, and two were written", len(held.Work.Situation))
	}
	if keeper.LatestCheckpoint() != 4 {
		t.Errorf("three changes left the record at checkpoint %d, and every change saves one", keeper.LatestCheckpoint())
	}
}

// TestRefusesAHeaderTheOtherKindOfRecordCarries proves a job has no budget and a
// task has no progress line.
func TestRefusesAHeaderTheOtherKindOfRecordCarries(t *testing.T) {
	task, _ := newKeeper(t, taskStart())
	job, _ := newKeeper(t, jobStart())
	ctx := t.Context()

	if err := task.SetProgress(ctx, 1, 2, "task 31 today"); err == nil {
		t.Error("a task took a progress line, and only a job has one")
	}
	if err := job.SetBudget(ctx, 5, 5); err == nil {
		t.Error("a job took a budget, and only a task has one")
	}
	if err := job.SetCost(ctx, contract.CostLine{InputTokens: 100}); err == nil {
		t.Error("a job took a cost line, and only a task has one")
	}
	if err := job.SetProgress(ctx, 3, 12, "task 31 today at 14:00"); err != nil {
		t.Errorf("a job would not take its own progress line: %v", err)
	}
	if held := job.Record().Header; held.TasksDone != 3 || held.TasksTotal != 12 || held.NextDue != "task 31 today at 14:00" {
		t.Errorf("the job header is %+v after its progress was written", held)
	}
}

// TestRefusesABudgetOrACostBelowZero proves the two header numbers are bounded.
func TestRefusesABudgetOrACostBelowZero(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()
	if err := keeper.SetBudget(ctx, -1, 5); err == nil {
		t.Error("the budget took a count of rounds below zero")
	}
	if err := keeper.SetCost(ctx, contract.CostLine{InputTokens: -1}); err == nil {
		t.Error("the cost line took a count of tokens below zero")
	}
	if err := keeper.SetSituation(ctx, []string{""}); err == nil {
		t.Error("the situation took an empty line")
	}
}

// TestAddsACorrectionInTheUsersOwnWords proves rule one: corrections are only
// ever appended, each with the next label, in the words the user wrote.
func TestAddsACorrectionInTheUsersOwnWords(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()

	first, err := keeper.AddCorrection(ctx, "no, lead with the date not the features")
	if err != nil {
		t.Fatalf("cannot add the first correction: %v", err)
	}
	second, err := keeper.AddCorrection(ctx, "actually, add the logo")
	if err != nil {
		t.Fatalf("cannot add the second correction: %v", err)
	}
	if first != "C1" || second != "C2" {
		t.Errorf("the corrections are labelled %q and %q, and they count upwards from C1", first, second)
	}
	held := keeper.Record().Rules.Corrections
	if len(held) != 2 || held[0].Text != "no, lead with the date not the features" {
		t.Errorf("the corrections read %+v, and they are the user's own words", held)
	}
	if _, err := keeper.AddCorrection(ctx, ""); !errors.Is(err, ErrCorrectionIsFixed) {
		t.Error("an empty correction was added, and a correction is something the user said")
	}
}

// TestAddsAResultAndReadsItBack proves rule four: every result gets the next
// label and one line in the record, and its full text goes to the log where it
// can be read back long after it left the model's window.
func TestAddsAResultAndReadsItBack(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()

	first, err := keeper.AddResult(ctx, "read memory/product.md, 2,100 characters", strings.Repeat("the whole file. ", 50))
	if err != nil {
		t.Fatalf("cannot add the first result: %v", err)
	}
	second, err := keeper.AddResult(ctx, "draft post, 236 characters", "the whole draft")
	if err != nil {
		t.Fatalf("cannot add the second result: %v", err)
	}
	if first != "r1" || second != "r2" {
		t.Errorf("the results are labelled %q and %q, and they count upwards from r1", first, second)
	}

	held := keeper.Record().Work.Results
	if len(held) != 2 || held[0].Summary != "read memory/product.md, 2,100 characters" {
		t.Errorf("the record holds %+v, and it should hold one line for each result", held)
	}
	text, err := keeper.Read(ctx, "r1")
	if err != nil {
		t.Fatalf("cannot read r1 back: %v", err)
	}
	if text != strings.Repeat("the whole file. ", 50) {
		t.Errorf("r1 reads back as %q", text)
	}
	if _, err := keeper.Read(ctx, "r9"); err == nil {
		t.Error("a result nobody wrote was read back")
	}
	if _, err := keeper.AddResult(ctx, "", "the text"); err == nil {
		t.Error("a result with no summary was added")
	}
}

// TestAddsAReportOnAJob proves a job's results are the reports of its finished
// tasks, labelled inside the job.
func TestAddsAReportOnAJob(t *testing.T) {
	keeper, _ := newKeeper(t, jobStart())
	ctx := t.Context()

	first, err := keeper.AddReport(ctx, "posted, 236 characters, link saved", "the whole report")
	if err != nil {
		t.Fatalf("cannot add the first report: %v", err)
	}
	if first != "j4.1" {
		t.Errorf("the first report of job 4 is labelled %q", first)
	}
	if text, err := keeper.Read(ctx, "j4.1"); err != nil || text != "the whole report" {
		t.Errorf("j4.1 reads back as %q with the error %v", text, err)
	}
	if _, err := keeper.AddResult(ctx, "a result", "the text"); err == nil {
		t.Error("a job took a task result, and a job holds reports")
	}
	task, _ := newKeeper(t, taskStart())
	if _, err := task.AddReport(ctx, "a report", "the text"); err == nil {
		t.Error("a task took a report, and a task holds results")
	}
}

// latestCheckpoint reads the last checkpoint of a record out of the log.
func latestCheckpoint(t *testing.T, store contract.Store, recordID string) Checkpoint {
	t.Helper()
	events, err := store.ByTask(context.Background(), recordID)
	if err != nil {
		t.Fatalf("cannot read the log for %s: %v", recordID, err)
	}
	saved := Checkpoint{}
	found := false
	for _, event := range events {
		if event.Kind != contract.EventCheckpoint {
			continue
		}
		if err := json.Unmarshal(event.Body, &saved); err != nil {
			t.Fatalf("cannot read a checkpoint of %s out of the log: %v", recordID, err)
		}
		found = true
	}
	if !found {
		t.Fatalf("the log holds no checkpoint of %s at all", recordID)
	}
	return saved
}
