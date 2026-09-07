package loop_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aJobFromAWorkOrder makes a job the way a work order does: a name, a why,
// done lines with checks, rules, and one task that answers at once.
func aJobFromAWorkOrder(t *testing.T, built *harness, lines []string) string {
	t.Helper()
	jobID, err := built.jobs.Create(t.Context(), contract.NewJob{
		Ask:      "build the game and prove it",
		Name:     "Tater Tots Tetris",
		Why:      "A playable Tetris game for players in a browser, every rule proved by a test.",
		DoneWhen: lines,
		Rules:    []string{"Tests first: write the test, watch it fail, write the code, watch it pass.", "Serve on port 8091; 8090 is in use."},
	})
	if err != nil {
		t.Fatalf("cannot create the job: %v", err)
	}
	if _, err := built.jobs.AddTask(t.Context(), contract.NewTask{JobID: jobID, Text: "build the game"}); err != nil {
		t.Fatalf("cannot add the task: %v", err)
	}
	return jobID
}

// aFinishingJob is a harness whose one job task answers at once and whose
// checks all pass, so the job finishes done.
func aFinishingJob(t *testing.T) (*harness, string) {
	t.Helper()
	page := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolBrowserOpen, Description: "A browser the test scripted."},
		"http://127.0.0.1:8091/ Tater Tots Tetris", "http://127.0.0.1:8091/ Tater Tots Tetris")
	built := newHarness(t, []testkit.Step{
		answerStep("The game is built. What changed: the engine. What I checked: the tests. What is left: nothing."),
		aReviewReply("Keep the config in one file."),
	}, page)
	built.sandbox.Script("sh -c npm test", contract.SandboxResult{StandardOutput: []byte(theGreenRun)})
	aJobFromAWorkOrder(t, built, []string{
		"Every test passes. [tests pass: npm test]",
		`The game loads. [shows: "Tater Tots Tetris" at http://127.0.0.1:8091]`,
	})
	if err := os.WriteFile(filepath.Join(built.workFolder, loop.ArchitectureFile), []byte("# Architecture\n\n## Engine\n\nThe engine.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return built, filepath.Join(built.workFolder, loop.StandingOrderFile)
}

// TestAFinishedJobWritesAgentsMdOnceAndNeverOverwrites: a job that finishes
// done in a folder with no NERDGENIE.md writes one from its record, under sixty
// lines: the name, the why, how to run and test it from the first checked done
// line, the rules, and where the other documents are. A file that exists is
// never touched.
func TestAFinishedJobWritesAgentsMdOnceAndNeverOverwrites(t *testing.T) {
	built, path := aFinishingJob(t)

	runTheJobToTheEnd(t, built.loop, built.channel)

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the finished job wrote no %s: %v", loop.StandingOrderFile, err)
	}
	text := string(written)
	for _, want := range []string{
		"# Tater Tots Tetris",
		"A playable Tetris game for players in a browser, every rule proved by a test.",
		"## Run",
		"http://127.0.0.1:8091",
		"## Test",
		"`npm test`",
		"## Rules",
		"- Tests first: write the test, watch it fail, write the code, watch it pass.",
		"- Serve on port 8091; 8090 is in use.",
		"## Where things are",
		"ARCHITECTURE.md",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the standing order lacks %q; it reads:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "REPO_MAP.md") {
		t.Errorf("the standing order does not point at the map the finish wrote beside it:\n%s", text)
	}
	if lines := strings.Count(text, "\n"); lines > 60 {
		t.Errorf("the standing order is %d lines, want under sixty", lines)
	}

	// A second job finishing in the same folder leaves the file byte for byte.
	again, _ := aFinishingJob(t)
	if err := os.WriteFile(path, written, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(again.workFolder, loop.StandingOrderFile), []byte("# Mine\n\nThe person's own rules.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTheJobToTheEnd(t, again.loop, again.channel)
	kept, err := os.ReadFile(filepath.Join(again.workFolder, loop.StandingOrderFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(kept) != "# Mine\n\nThe person's own rules.\n" {
		t.Errorf("a standing order the person wrote was changed:\n%s", string(kept))
	}
}

// TestAJobWhoseDoneListIsNotProvedWritesNoAgentsMd: a job that runs every
// task and still has a red check is not finished, and writes nothing.
// TestAJobsStandingOrderIsThereFromItsFirstTasksEndEvenWhileACheckIsRed holds
// that the standing order does not wait for the finish: it is written from
// the job's record at the end of the first task, red check or not, because a
// long job needs its rules and commands in front of every task from the
// second one on. Only the never-overwrite rule guards a person's own file.
func TestAJobsStandingOrderIsThereFromItsFirstTasksEndEvenWhileACheckIsRed(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		answerStep("The game is built. What changed: the engine. What I checked: the tests. What is left: nothing."),
		aReviewReply("Keep the config in one file."),
	})
	built.sandbox.Script("sh -c npm test", contract.SandboxResult{StandardOutput: []byte(theRedRun), ExitCode: 1})
	aJobFromAWorkOrder(t, built, []string{"Every test passes. [tests pass: npm test]"})

	runTheJobToTheEnd(t, built.loop, built.channel)

	if _, err := os.Stat(filepath.Join(built.workFolder, loop.StandingOrderFile)); err != nil {
		t.Errorf("the standing order is not there after the job's first task, though a later task needs it: %v", err)
	}
}

// TestTheStandingOrderAlwaysTellsTheModelToLookInTheMapFirst holds that the
// standing order a job writes carries the map rule whatever the job's own
// rules were: look a file or a function up in the map before searching or
// reading for it, which is what the map is for.
func TestTheStandingOrderAlwaysTellsTheModelToLookInTheMapFirst(t *testing.T) {
	built, path := aFinishingJob(t)

	runTheJobToTheEnd(t, built.loop, built.channel)

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), loop.TheMapRule) {
		t.Errorf("the standing order lacks the map rule %q; it reads:\n%s", loop.TheMapRule, string(written))
	}
}
