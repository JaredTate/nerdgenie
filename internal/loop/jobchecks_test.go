package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aJobWithCheckedDoneLines makes a job the way a work order does: with the
// person's done lines, some of them checked by the harness, and two tasks.
func aJobWithCheckedDoneLines(t *testing.T, built *harness, lines ...string) string {
	t.Helper()
	jobID, err := built.jobs.Create(t.Context(), contract.NewJob{
		Ask: "build the game and prove it", Name: "Tater Tots Tetris", Why: "the player wants a game", DoneWhen: lines,
	})
	if err != nil {
		t.Fatalf("cannot create the job: %v", err)
	}
	for _, text := range []string{"build the game", "polish it"} {
		if _, err := built.jobs.AddTask(t.Context(), contract.NewTask{JobID: jobID, Text: text}); err != nil {
			t.Fatalf("cannot add a task to job %s: %v", jobID, err)
		}
	}
	return jobID
}

// TestAJobTasksEndRunsTheJobsChecks: at the end of every task of a job the
// harness runs the job's bracketed done lines, marks the ones that pass with
// the task's report, and the job summary counts them, so a job that runs all
// night is measured after every task and closes on the person's checks.
func TestAJobTasksEndRunsTheJobsChecks(t *testing.T) {
	page := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolBrowserOpen, Description: "A browser the test scripted."},
		"http://127.0.0.1:8091/ Tater Tots Tetris", "http://127.0.0.1:8091/ Tater Tots Tetris")
	built := newHarness(t, []testkit.Step{
		answerStep("The game is built. What changed: the engine. What I checked: the tests. What is left: polish."),
		answerStep("It is polished. What changed: the shell. What I checked: the tests. What is left: nothing."),
		aReviewReply("Keep the config in one file."),
	}, page)
	built.sandbox.Script("sh -c npm test", contract.SandboxResult{StandardOutput: []byte(theGreenRun)})
	jobID := aJobWithCheckedDoneLines(t, built,
		"Every test passes. [tests pass: npm test]",
		`The game loads. [shows: "Tater Tots Tetris" at http://127.0.0.1:8091]`,
		"A whole game has been played to game over.")

	ran := runTheJobToTheEnd(t, built.loop, built.channel)

	if ran != 2 {
		t.Fatalf("the driver ran %d tasks, want the two", ran)
	}
	held, err := built.jobs.Load(t.Context(), jobID)
	if err != nil {
		t.Fatal(err)
	}
	printed := string(record.Print(held))
	if !strings.Contains(printed, "done lines proved: 2 of 2") {
		t.Errorf("the job's header does not count its proved checks, and the record reads:\n%s", printed)
	}
	for at, line := range held.Goal.DoneWhen {
		if !line.Done || !strings.HasPrefix(line.ResultID, "j"+jobID+".") {
			t.Errorf("done line %d reads %+v, want it marked with a report of the job", at+1, line)
		}
	}
	if held.Header.Status != contract.StatusDone {
		t.Errorf("the job reads %q with every line proved, want done", held.Header.Status)
	}
	if !sentSomethingLike(built.channel.Sent(), "Done lines proved: 2 of 2") {
		t.Errorf("the person was not told the count after a task, and the channel got %v", built.channel.Sent())
	}
}

// TestTheJobsFinishIsRefusedWhileACheckFails: a job's checked line that stays
// red is never marked; when the last task ends, the job gives itself one
// more task to make the line true, up to three times, and after that its
// done list stays unproved and the person is told which line, so no job
// closes done on a check the person wrote and the harness saw fail.
func TestTheJobsFinishIsRefusedWhileACheckFails(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		answerStep("The game is built. What changed: the engine. What I checked: the tests. What is left: polish."),
		answerStep("It is polished. What changed: the shell. What I checked: the tests. What is left: nothing."),
		answerStep("I tried the row clear again. What changed: a line. What I checked: the tests. What is left: the same test."),
		answerStep("I tried another route. What changed: a line. What I checked: the tests. What is left: the same test."),
		answerStep("I tried once more. What changed: a line. What I checked: the tests. What is left: the same test."),
		aReviewReply("The row clear needs a different design."),
	})
	built.sandbox.Script("sh -c npm test", contract.SandboxResult{StandardOutput: []byte(theRedRun), ExitCode: 1})
	jobID := aJobWithCheckedDoneLines(t, built,
		"Every test passes. [tests pass: npm test]",
		"A whole game has been played to game over.")

	ran := runTheJobToTheEnd(t, built.loop, built.channel)

	if ran != 5 {
		t.Fatalf("the driver ran %d tasks, want the two and three the job gave itself for the red check", ran)
	}
	held, err := built.jobs.Load(t.Context(), jobID)
	if err != nil {
		t.Fatal(err)
	}
	fixes := 0
	for _, task := range held.Work.Tasks {
		if strings.HasPrefix(task.Text, "Done line 1 is not true yet") {
			fixes++
		}
	}
	if fixes != 3 {
		t.Errorf("the job holds %d tasks for the red check, want three, one per try: %+v", fixes, held.Work.Tasks)
	}
	if held.Goal.DoneWhen[0].Done {
		t.Errorf("done line 1 reads %+v after a red check every time, want it unmarked", held.Goal.DoneWhen[0])
	}
	if !held.Goal.DoneWhen[1].Done {
		t.Errorf("done line 2 reads %+v, want the unchecked line proved by the tasks at the close", held.Goal.DoneWhen[1])
	}
	if held.Header.Status == contract.StatusDone {
		t.Errorf("the job closed done with its checked line red")
	}
	sent := built.channel.Sent()
	if !sentSomethingLike(sent, "done list is not proven yet") || !sentSomethingLike(sent, "2 failing of 10") {
		t.Errorf("the person was not told which check stays red, and the channel got %v", sent)
	}
}

// TestAFailedCheckSaysWhyOnTheTasksReport: on run 24 the shows and looks
// checks failed after the board task because nothing answered on the port,
// and the next task saw two unproved lines and no reason. The progress line
// every report carries names the line and the check's own words, bounded,
// so the next task knows what to put right.
func TestAFailedCheckSaysWhyOnTheTasksReport(t *testing.T) {
	page := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolBrowserOpen, Description: "A browser the test scripted."},
		"cannot open the page http://127.0.0.1:8091: connection refused", "cannot open the page http://127.0.0.1:8091: connection refused")
	built := newHarness(t, []testkit.Step{
		answerStep("The game is built. What changed: the engine. What I checked: the tests. What is left: polish."),
		aReviewReply("none"),
		answerStep("It is polished. What changed: the shell. What I checked: the tests. What is left: nothing."),
		aReviewReply("none"),
	}, page)
	built.sandbox.Script("sh -c npm test", contract.SandboxResult{StandardOutput: []byte(theGreenRun)})
	aJobWithCheckedDoneLines(t, built,
		"Every test passes. [tests pass: npm test]",
		`The game loads. [shows: "Tater Tots Tetris" at http://127.0.0.1:8091]`,
		"A whole game has been played to game over.")

	runTheJobToTheEnd(t, built.loop, built.channel)

	if !sentSomethingLike(built.channel.Sent(), "Not proved: line 2") {
		t.Errorf("the report does not name the line whose check failed; the channel got %v", built.channel.Sent())
	}
	if !sentSomethingLike(built.channel.Sent(), "connection refused") {
		t.Errorf("the report does not carry the check's own reason; the channel got %v", built.channel.Sent())
	}
}
