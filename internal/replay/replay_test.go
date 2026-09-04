package replay_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/replay"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestReplayReproducesARecordingOfACleanRun(t *testing.T) {
	made := runScript(t, twoRoundScript())

	result, err := replay.Run(context.Background(), theReplayOptions(t, made.store), made.outcome.TaskID)
	if err != nil {
		t.Fatalf("cannot replay task %s: %v", made.outcome.TaskID, err)
	}

	if !result.Passed {
		t.Fatalf("the replay of a clean run did not reproduce it: %s", result.Report)
	}
	if result.Difference != "" {
		t.Errorf("the replay passed and still named the difference %q", result.Difference)
	}
	if result.Status != contract.StatusDone {
		t.Errorf("the replayed task ended %q and the recording ended done", result.Status)
	}
	if result.Rounds != 2 {
		t.Errorf("the replay played %d rounds and the recording holds two", result.Rounds)
	}
	if !strings.Contains(result.Report, "reproduced") {
		t.Errorf("the report is %q and it must say the recording was reproduced", result.Report)
	}
}

func TestReplayReproducesARecordingWithACorrectionInIt(t *testing.T) {
	made := runScriptWithACorrection(t)

	result, err := replay.Run(context.Background(), theReplayOptions(t, made.store), made.outcome.TaskID)
	if err != nil {
		t.Fatalf("cannot replay the corrected task: %v", err)
	}
	if !result.Passed {
		t.Fatalf("the replay lost the user's correction: %s", result.Report)
	}
}

func TestReplayNamesTheFirstStepThatDiffered(t *testing.T) {
	made := runScript(t, twoRoundScript())
	options := theReplayOptions(t, made.store)
	refusing := testkit.NewFakePermission(contract.RulingAllow)
	refusing.Rule("search", contract.PermissionDecision{
		Ruling: contract.RulingDeny, Reason: "this machine no longer allows a search",
	})
	options.Permission = refusing

	result, err := replay.Run(context.Background(), options, made.outcome.TaskID)
	if err != nil {
		t.Fatalf("cannot replay task %s against the changed rules: %v", made.outcome.TaskID, err)
	}

	if result.Passed {
		t.Fatal("the replay reproduced a recording it could not have reproduced, because every search was refused")
	}
	if !strings.Contains(result.Difference, "search") {
		t.Errorf("the first difference is %q and the search is what changed", result.Difference)
	}
	if !strings.Contains(result.Report, result.Difference) {
		t.Errorf("the report is %q and it must carry the difference %q", result.Report, result.Difference)
	}
}

// TestReplayOfARecordedFailingTaskPassesAfterTheFix is the case design section
// 4 asks for: a task that failed is replayed as a test once somebody has made a
// fix. The fixture log in testdata holds a task that failed because its round
// budget ran out one round before it could report, with its done list already
// true. Replayed on a machine with that same small budget it fails in exactly
// the same way, which is what makes the replay a test worth trusting. The fix
// is the flag this test flips, which is the budget, and with it the same
// recording ends where it was meant to end: done, with the done-check passing.
func TestReplayOfARecordedFailingTaskPassesAfterTheFix(t *testing.T) {
	recorded := loadFixtureLog(t, theBudgetTaskFixture)

	beforeTheFix := theReplayOptions(t, recorded)
	beforeTheFix.Caps = tooSmallABudget()
	before, err := replay.Run(context.Background(), beforeTheFix, theFixtureTaskID)
	if err != nil {
		t.Fatalf("cannot replay the recorded failing task: %v", err)
	}
	if !before.Passed {
		t.Fatalf("the replay did not reproduce the recorded failure, so the recording and the code disagree: %s", before.Report)
	}
	if before.Status != contract.StatusStopped {
		t.Fatalf("the replayed task ended %q and the recording ended stopped", before.Status)
	}

	after, err := replay.Run(context.Background(), theReplayOptions(t, recorded), theFixtureTaskID)
	if err != nil {
		t.Fatalf("cannot replay the recorded failing task after the fix: %v", err)
	}
	if after.Status != contract.StatusDone {
		t.Fatalf("the fix was applied and the task still ended %q: %s", after.Status, after.Report)
	}
	if after.DoneCheck != "" {
		t.Errorf("the done-check says %q after the fix, and it must pass", after.DoneCheck)
	}
	if after.Passed {
		t.Error("the replay says it reproduced a recording the fix deliberately changed")
	}
}

func TestReplayRefusesOptionsWithNothingToRunOn(t *testing.T) {
	missing := []struct {
		what    string
		options replay.Options
	}{
		{"the log to read from", replay.Options{Into: testkit.NewFakeStore()}},
		{"the log to write into", replay.Options{From: testkit.NewFakeStore()}},
	}
	for _, one := range missing {
		t.Run(one.what, func(t *testing.T) {
			if _, err := replay.Run(context.Background(), one.options, "1"); err == nil {
				t.Fatalf("a replay with no %s gave no error", one.what)
			}
		})
	}
}
