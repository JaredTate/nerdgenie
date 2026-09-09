// The tests for the "never quit" rule. Under yolo the agent is unattended, so
// a job's task that would stop to wait for a person is not put down: the job
// hands it back to the model on a fresh window with a bold reorient ask that
// breaks the trance of going round in circles, and keeps going, as many fresh
// starts as it takes, until the job is done. Tic-tac-toe run 36's task 1 ended
// with a handoff line the harness read as a question and sat waiting for a
// person who was not there; nothing should ever wait on nobody.
package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aKeepGoingJobOfTwoTasks builds the harness with yolo's "never wait" rule on
// and a job of two tasks over the script given.
func aKeepGoingJobOfTwoTasks(t *testing.T, steps []testkit.Step, reads ...string) (*harness, string) {
	t.Helper()
	built := newHarness(t, steps, scriptedTool("read", reads...))
	built.keepGoing = true
	built.loop = mustBuild(t, built)
	jobID := aJobOfTwoTasks(t, built)
	return built, jobID
}

// TestUnderKeepGoingAWaitingJobTaskIsReorientedInsteadOfPutDown is the whole
// rule at its simplest: the job's first task asks a question, which would put
// it down to wait for a person, and instead the job reorients it on a fresh
// window and it finishes, the second task finishes, and the job closes. The
// person is never asked to pick anything up, and no task is left put down.
func TestUnderKeepGoingAWaitingJobTaskIsReorientedInsteadOfPutDown(t *testing.T) {
	built, jobID := aKeepGoingJobOfTwoTasks(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("Which folder should the output go in?"),            // task 1 asks → would wait
		answerStep("A bold new approach worked: the tweet is posted."), // reoriented → finishes
		answerStep("The summary is written."),                          // task 2 finishes
		aReviewReply("Keep going without waiting for anyone."),         // the job's close review
	}, "the notes")

	ran := runTheJobToTheEnd(t, built.loop, built.channel)

	if ran != 2 {
		t.Errorf("the driver ran %d tasks, want 2: the reoriented first task counts once, on the same turn, then the second", ran)
	}
	if summary := theSummaryOf(t, built, jobID); summary.State != contract.JobDone || summary.FailuresInARow != 0 {
		t.Errorf("the job reads %+v, want it done with no failure counted: a reoriented task is neither failed nor put down", summary)
	}
	if _, there, _ := built.jobs.PutDownTask(t.Context()); there {
		t.Error("the store holds a put-down task, and the job reoriented its task instead of waiting")
	}
	for _, task := range built.jobs.Tasks(jobID) {
		if !task.Done || task.ReportID == "" {
			t.Errorf("the task %s reads %+v, want it done and pointing at its report", task.TaskID, task)
		}
	}
	sent := built.channel.Sent()
	if !sentSomethingLike(sent, "keeps working task t1 itself") || !sentSomethingLike(sent, "will not stop until the job is done") {
		t.Errorf("the person was sent %v, want the one line saying the job keeps working the task itself and never stops", sent)
	}
	for _, waiting := range []string{"Your next message picks this task up", "is waiting on task", "for your answer"} {
		if sentSomethingLike(sent, waiting) {
			t.Errorf("the person was sent %v, and under yolo the job never waits on a person (%q)", sent, waiting)
		}
	}
}

// TestUnderKeepGoingAJobTaskReorientsAgainAndAgainUntilDone: the reorient is
// not once, the way the guard pick-up is; it repeats for as many fresh starts
// as the task takes. The first task asks three times running, is reoriented
// three times, then finishes, and the job closes. This is the "never stops
// till the job is done" of the rule.
func TestUnderKeepGoingAJobTaskReorientsAgainAndAgainUntilDone(t *testing.T) {
	built, jobID := aKeepGoingJobOfTwoTasks(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("First question, which way?"),                       // waiting → reorient 1
		answerStep("Second question, which way?"),                      // waiting → reorient 2
		answerStep("Third question, which way?"),                       // waiting → reorient 3
		answerStep("The boldest move landed it: the tweet is posted."), // finishes
		answerStep("The summary is written."),                          // task 2
		aReviewReply("Keep going without waiting for anyone."),
	}, "the notes")

	ran := runTheJobToTheEnd(t, built.loop, built.channel)

	if ran != 2 {
		t.Errorf("the driver ran %d tasks, want 2: every reorient of the first task is on the same turn", ran)
	}
	if summary := theSummaryOf(t, built, jobID); summary.State != contract.JobDone || summary.FailuresInARow != 0 {
		t.Errorf("the job reads %+v, want it done with no failure counted after three reorients", summary)
	}
	if _, there, _ := built.jobs.PutDownTask(t.Context()); there {
		t.Error("the store holds a put-down task, and the job reoriented again and again instead of waiting")
	}
	if count := timesSent(built.channel.Sent(), "keeps working task t1 itself"); count != 3 {
		t.Errorf("the job said it keeps working the task itself %d times, want three: it reoriented after each of the three questions", count)
	}
	if _, count := requestsCarrying(built, "the boldest, most different"); count != 3 {
		t.Errorf("the bold reorient ask reached the model %d times, want three, one per fresh window", count)
	}
}

// TestTheBoldReorientAskGivesFreshEyesAndNamesTheFailures: the ask that opens
// a reoriented window breaks the trance. It carries the record's newest
// results so the model can read what it already tried, and it tells the model
// in plain words to drop all of that, look with fresh eyes, and take a
// fundamentally different, bold approach. This locks the words of the nudge.
func TestTheBoldReorientAskGivesFreshEyesAndNamesTheFailures(t *testing.T) {
	built, _ := aKeepGoingJobOfTwoTasks(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("Which folder should the output go in?"), // task 1 asks → reoriented
		answerStep("A bold new approach worked: the tweet is posted."),
		answerStep("The summary is written."),
		aReviewReply("Keep going without waiting for anyone."),
	}, "the notes")

	runTheJobToTheEnd(t, built.loop, built.channel)

	first, count := requestsCarrying(built, loop.TheBoldReorientAsk)
	if count == 0 {
		t.Fatal("the bold reorient ask never reached the model on the fresh window")
	}
	opening := wholeRequestText(built.model.Requests()[first])
	for _, words := range []string{
		"break out of it now",
		"read the record's failures",
		"do not repeat any of it",
		"completely fresh eyes",
		"fundamentally different approach",
		"boldest, most different thing that could still work",
	} {
		if !strings.Contains(opening, words) {
			t.Errorf("the reoriented window does not tell the model to %q:\n%s", words, opening)
		}
	}
	if strings.Contains(opening, "Do something different") {
		t.Errorf("the reoriented window still carries the stalled rounds' rethink, want a fresh window:\n%s", opening)
	}
}

// TestUnderKeepGoingAGuardStalledTaskIsReorientedAfterItsPickUpAndSetAsideAreSpent
// is the rule on the path the real runs hit. A task that stalls into the
// harness's guard is first picked up once and then set aside once, the two
// steps that already existed; only its third stall would have put it down for
// a person. Under yolo that third stall reorients it and keeps going instead.
func TestUnderKeepGoingAGuardStalledTaskIsReorientedAfterItsPickUpAndSetAsideAreSpent(t *testing.T) {
	steps, reads, calls := twoGuardStopsOnTheFirstTask()
	// The set-aside first task comes back and stalls a third time; under yolo
	// that reorients it rather than putting it down, and this time it finishes.
	steps = append(steps, closingScript("the terrain is drawn")...)
	steps = append(steps, answerStep("none"))
	reads = append(reads, "the notes")
	third, evenMore, _ := stallsThatStopTheGuard(calls)
	steps = append(steps, third...)
	// The third stall stops on the guard: its review and its architecture
	// question run, the way any guard stop's do.
	steps = append(steps, aReviewReply("Read a file once and move on."), answerStep("none"))
	// Then the job reorients task 1 on a fresh window rather than putting it
	// down. The window opens with a rethink over the stalled record, and there
	// the task finishes; then the job's last task, the sky, and the job's close.
	steps = append(steps, theUsualRethink(), answerStep("A fundamentally different route finished the cockpit."))
	steps = append(steps, answerStep("The sky is drawn."))
	steps = append(steps, aReviewReply("Keep going without waiting for anyone."))
	reads = append(reads, evenMore...)
	built := newHarness(t, steps, scriptedTool("read", reads...))
	built.keepGoing = true
	built.loop = mustBuild(t, built)
	jobID := aJobOfThreeTasks(t, built)

	runTheJobToTheEnd(t, built.loop, built.channel)

	sent := built.channel.Sent()
	if count := timesSent(sent, "is set aside after stopping twice"); count != 1 {
		t.Errorf("the task said it was set aside %d times, want once: the pick-up and the set-aside still run before the reorient", count)
	}
	if !sentSomethingLike(sent, "keeps working task t1 itself") {
		t.Errorf("the person was sent %v, want the line saying the job keeps working the stalled task itself after its set-aside is spent", sent)
	}
	if sentSomethingLike(sent, "Your next message picks this task up") {
		t.Errorf("the person was sent %v, and under yolo a thrice-stalled task is reoriented, never put down", sent)
	}
	if summary := theSummaryOf(t, built, jobID); summary.State != contract.JobDone || summary.FailuresInARow != 0 {
		t.Errorf("the job reads %+v, want it done with no failure counted: the reorient is neither a failure nor a put-down", summary)
	}
	if _, there, _ := built.jobs.PutDownTask(t.Context()); there {
		t.Error("the store holds a put-down task, and the job reoriented the stalled task instead")
	}
}

// TestWithoutKeepGoingAWaitingJobTaskStillWaitsForThePerson: with yolo off the
// old behaviour stands. A job's task that asks a question is put down and the
// job pauses on it for the person's answer, exactly as before, so the rule is
// the unattended agent's alone and changes nothing for an attended one.
func TestWithoutKeepGoingAWaitingJobTaskStillWaitsForThePerson(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		answerStep("Which folder should the output go in?"),
		answerStep("This reply is never played."),
	}, scriptedTool("read", "the notes"))
	// keepGoing is left off, the harness's default.
	jobID := aJobOfTwoTasks(t, built)

	ran, err := built.loop.RunNextJobTask(t.Context(), built.channel)
	if err != nil {
		t.Fatalf("the loop could not run the job's task: %v", err)
	}
	if !ran {
		t.Fatal("the loop found nothing to run, and the job has two tasks waiting")
	}

	if summary := theSummaryOf(t, built, jobID); summary.State != contract.JobPaused {
		t.Errorf("with yolo off the job is %q, want it paused on the task for the person", summary.State)
	}
	mark, there, err := built.jobs.PutDownTask(t.Context())
	if err != nil || !there || mark.Task.TaskID != "t1" || !mark.Waiting {
		t.Errorf("the store holds the put-down task as %+v (there %v, error %v), want task t1 waiting for the person", mark, there, err)
	}
	if sentSomethingLike(built.channel.Sent(), "keeps working task t1 itself") {
		t.Errorf("the person was sent %v, and with yolo off the job waits rather than keeping on itself", built.channel.Sent())
	}
	if built.model.StepsLeft() != 1 {
		t.Errorf("the model has %d steps left, want 1: nothing runs after the job is paused for the person", built.model.StepsLeft())
	}
}
