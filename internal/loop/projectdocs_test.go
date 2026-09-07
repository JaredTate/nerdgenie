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

// TestEveryJobTasksEndLeavesTheThreeDocumentsInTheProjectFolder holds that
// a long job does not wait for its finish to leave NERDGENIE.md, ARCHITECTURE.md
// and REPO_MAP.md in the project folder: the end of its first task writes
// all three, so the second task and every one after it start from them.
func TestEveryJobTasksEndLeavesTheThreeDocumentsInTheProjectFolder(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "src", "notes.js"), []byte("// notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	built := newHarness(t, []testkit.Step{
		callStep("I will scaffold the app.", callFor("c1", contract.ToolWrite, `{"path":"`+filepath.Join(project, "src", "notes.js")+`","content":"// notes"}`)),
		answerStep("The app is scaffolded. What changed: the files. What I checked: the tests. What is left: the list."),
		answerStep("## Storage\nNotes live in src/notes.js and are kept in local storage."),
	}, scriptedTool(contract.ToolWrite, "created the file"))
	built.sandbox.Script("sh -c cd", contract.SandboxResult{StandardOutput: []byte(theGreenRun)})
	if outcome := built.ask(t, aWorkOrderIn(project)); outcome.Status != contract.StatusDone {
		t.Fatalf("the lift ended %q: %s", outcome.Status, outcome.Report)
	}
	if more, err := built.loop.RunNextJobTask(t.Context(), built.channel); err != nil || !more {
		t.Fatalf("the job's first task did not run: more=%v err=%v", more, err)
	}

	for _, name := range []string{loop.StandingOrderFile, loop.ArchitectureFile, loop.MapFile} {
		if _, err := os.Stat(filepath.Join(project, name)); err != nil {
			t.Errorf("after the first task of two, the project folder has no %s", name)
		}
	}
	if page, err := os.ReadFile(filepath.Join(project, loop.ArchitectureFile)); err == nil && !strings.Contains(string(page), "## Storage") {
		t.Errorf("the architecture page lacks the section the task wrote:\n%s", string(page))
	}
	if held, err := built.jobs.Load(t.Context(), "1"); err != nil || held.Header.Status == contract.StatusDone {
		t.Fatalf("the job should still be running after one task of two: status %q err %v", held.Header.Status, err)
	}
}

// TestAJobWithoutAWhereLearnsItsFolderFromTheFilesItsFirstTaskWrote holds that
// a job the model made from a plain ask, with no Where, still gets its
// documents in the right place: the folder the first task's files share
// becomes the job's project folder, written into the job's record, and the
// documents go there.
func TestAJobWithoutAWhereLearnsItsFolderFromTheFilesItsFirstTaskWrote(t *testing.T) {
	project := t.TempDir()
	for _, path := range []string{"src/game.js", "test/game.test.js"} {
		whole := filepath.Join(project, path)
		if err := os.MkdirAll(filepath.Dir(whole), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(whole, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	built := newHarness(t, []testkit.Step{
		callStep("I will write the game.",
			callFor("c1", contract.ToolWrite, `{"path":"`+filepath.Join(project, "src", "game.js")+`","content":"x"}`),
			callFor("c2", contract.ToolWrite, `{"path":"`+filepath.Join(project, "test", "game.test.js")+`","content":"x"}`)),
		answerStep("The game is written. What changed: two files. What I checked: nothing yet. What is left: the rest."),
		answerStep("none"),
	}, scriptedTool(contract.ToolWrite, "created the file", "created the file"))
	jobID, err := built.jobs.Create(t.Context(), contract.NewJob{Ask: "build a game on the desktop", Name: "Game", Why: "for fun"})
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"write the game", "polish it"} {
		if _, err := built.jobs.AddTask(t.Context(), contract.NewTask{JobID: jobID, Text: text}); err != nil {
			t.Fatal(err)
		}
	}

	if more, err := built.loop.RunNextJobTask(t.Context(), built.channel); err != nil || !more {
		t.Fatalf("the job's first task did not run: more=%v err=%v", more, err)
	}

	held, err := built.jobs.Load(t.Context(), jobID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(held.Work.Situation, "\n"), contract.ProjectFolderLine+project) {
		t.Errorf("the job did not learn its folder from the files its task wrote; the situation reads %v", held.Work.Situation)
	}
	for _, name := range []string{loop.StandingOrderFile, loop.MapFile} {
		if _, err := os.Stat(filepath.Join(project, name)); err != nil {
			t.Errorf("the learned folder has no %s after the first task", name)
		}
	}
}
