package loop_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aWorkOrderIn is the notes work order with a Where that names the folder,
// which is not the home's work folder: a project on the Desktop, say.
func aWorkOrderIn(folder string) string {
	return strings.Replace(aWorkOrderAsk, "A new, empty folder.", "Create a folder at `"+folder+"` and put everything inside it.", 1)
}

// TestAJobMadeFromAWorkOrderWorksInTheFolderWhereNames holds that the folder
// Where names, not the home's work folder, is where a task of the job finds
// its bearings, reads the project's standing order, and runs the person's
// checks. On 7 September 2026 run sixteen built tic-tac-toe on the Desktop
// while the harness listed the work folder, read no AGENTS.md, and ran "npm
// test" where there was no package.json, unproving a line the model had
// just proved.
func TestAJobMadeFromAWorkOrderWorksInTheFolderWhereNames(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("# Notes\n\n- Serve on port 8097.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "notes.js"), []byte("// the notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	built := newHarness(t, []testkit.Step{
		answerStep("The app is scaffolded. What changed: the files. What I checked: the tests. What is left: the list."),
		aReviewReply("none"),
	})
	built.sandbox.Script("sh -c cd", contract.SandboxResult{StandardOutput: []byte(theGreenRun)})

	if outcome := built.ask(t, aWorkOrderIn(project)); outcome.Status != contract.StatusDone {
		t.Fatalf("the lift ended %q: %s", outcome.Status, outcome.Report)
	}
	held, err := built.jobs.Load(t.Context(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(held.Work.Situation, "\n"), "project folder: "+project) {
		t.Errorf("the job's situation does not name the project folder: %v", held.Work.Situation)
	}
	if more, err := built.loop.RunNextJobTask(t.Context(), built.channel); err != nil || !more {
		t.Fatalf("the job's first task did not run: more=%v err=%v", more, err)
	}

	first := wholeRequestText(built.model.Requests()[0])
	if !strings.Contains(first, "notes.js") {
		t.Errorf("the orientation does not list the project folder's files; the request reads:\n%s", first)
	}
	if !strings.Contains(first, "Serve on port 8097.") {
		t.Errorf("the project's AGENTS.md did not reach the model; the request reads:\n%s", first)
	}
	ranInTheFolder := false
	for _, command := range built.sandbox.Commands() {
		joined := strings.Join(command.Arguments, " ")
		if strings.Contains(joined, "npm test") && strings.Contains(joined, "cd '"+project+"'") {
			ranInTheFolder = true
		}
	}
	if !ranInTheFolder {
		t.Errorf("the tests-pass check did not run inside the project folder; the sandbox ran %v", built.sandbox.Commands())
	}
}
