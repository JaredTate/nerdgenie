package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aPlanOfThree is the model's first round: a plan of three steps and the read
// that makes r1.
func aPlanOfThree() testkit.Step {
	return callStep("Nothing is read yet. I will plan and read the notes.",
		taskCall("p1", `{"plan":["read the notes","read the brand file","write the post"],"doneWhen":["the notes are read","the post is written"]}`),
		callFor("c1", contract.ToolRead, `{"path":"notes.md"}`))
}

// TestAStepMarkedDoneOnTheFirstLineIsMarkedWithoutACall is idea one's second
// half, held back until the nightly set could measure it: in the fifth game
// build's last three tasks a fifth of all rounds did nothing but mark a step
// or pin a line through the task tool, one call per round at twenty seconds
// each. The first line of a reply already says where the work stands; a
// "step 2 done: r1" or "line 1 done: r1" on it marks the step or the line
// with that result, and the round is spent on work instead.
func TestAStepMarkedDoneOnTheFirstLineIsMarkedWithoutACall(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		aPlanOfThree(),
		callStep("Step 1 done: r1. Line 1 done: r1. Next: the brand file.", callFor("c2", contract.ToolRead, `{"path":"brand.md"}`)),
		answerStep("Step 2 done: r2. What changed: nothing. What I checked: both files. What is left: the post."),
	}, scriptedTool(contract.ToolRead, "the notes", "the brand file"))

	outcome := built.ask(t, "read the notes and the brand file")

	held := built.held(t, outcome.TaskID)
	if len(held.Work.Plan) != 3 || !held.Work.Plan[0].Done || held.Work.Plan[0].ResultID != "r1" {
		t.Errorf("after 'Step 1 done: r1' on the first line the plan reads %+v, want step 1 done by r1", held.Work.Plan)
	}
	if !held.Work.Plan[1].Done || held.Work.Plan[1].ResultID != "r2" {
		t.Errorf("after 'Step 2 done: r2' on the answer's first line the plan reads %+v, want step 2 done by r2", held.Work.Plan)
	}
	if held.Work.Plan[2].Done {
		t.Errorf("step 3 was marked done, and nothing said so: %+v", held.Work.Plan[2])
	}
	if len(held.Goal.DoneWhen) != 2 || !held.Goal.DoneWhen[0].Done || held.Goal.DoneWhen[0].ResultID != "r1" || held.Goal.DoneWhen[1].Done {
		t.Errorf("after 'Line 1 done: r1' the done list reads %+v, want line 1 done by r1 and line 2 open", held.Goal.DoneWhen)
	}
}

// TestAMarkOnTheFirstLineNamingNoResultOrAWrongOneMarksNothingAndSaysSo
// keeps the mark honest the way the tool is: a step is done by a result the
// record wrote, and a mark that names none, or one the record never wrote,
// marks nothing and the next request says why.
func TestAMarkOnTheFirstLineNamingNoResultOrAWrongOneMarksNothingAndSaysSo(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		aPlanOfThree(),
		callStep("Step 1 done. Next: the brand file.", callFor("c2", contract.ToolRead, `{"path":"brand.md"}`)),
		callStep("Step 2 done: r99. Next: the post.", callFor("c3", contract.ToolRead, `{"path":"post.md"}`)),
		answerStep("The files are read. What changed: nothing. What I checked: the files. What is left: the post."),
	}, scriptedTool(contract.ToolRead, "the notes", "the brand file", "the post"))

	outcome := built.ask(t, "read the notes and the brand file")

	held := built.held(t, outcome.TaskID)
	if held.Work.Plan[0].Done || held.Work.Plan[1].Done {
		t.Errorf("a mark with no result, or with one the record never wrote, marked a step: %+v", held.Work.Plan)
	}
	requests := built.model.Requests()
	if shown := wholeRequestText(requests[2]); !strings.Contains(shown, loop.TheMarkNeedsAResult) {
		t.Errorf("after 'Step 1 done.' with no result the next request does not say to name one:\n%s", shown)
	}
	if shown := wholeRequestText(requests[3]); !strings.Contains(shown, "r99") || !strings.Contains(shown, "never wrote") {
		t.Errorf("after 'Step 2 done: r99' the next request does not say the record never wrote r99:\n%s", shown)
	}
}

// TestARoundSpentOnlyMarkingTheRecordEarnsTheHintOnce is the teaching moment
// at the point of the waste: in the nightly set's second run the model, told
// in the rules to mark on its first line, still spent whole rounds on a lone
// step_done or pin_result call. The round after such a call reads one line
// saying to put the mark on the first line next time, once per task, and a
// mark made in the same reply as real work earns nothing.
func TestARoundSpentOnlyMarkingTheRecordEarnsTheHintOnce(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		aPlanOfThree(),
		callStep("The notes are read.", taskCall("m1", `{"operation":"step_done","step":1,"result":"r1"}`)),
		callStep("Step 1 is marked.", taskCall("m2", `{"operation":"pin_result","line":1,"result":"r1"}`)),
		callStep("I will read the brand file and mark it.", callFor("c2", contract.ToolRead, `{"path":"brand.md"}`), taskCall("m3", `{"operation":"step_done","step":2,"result":"r1"}`)),
		answerStep("The files are read. What changed: nothing. What I checked: the files. What is left: the post."),
	}, scriptedTool(contract.ToolRead, "the notes", "the brand file"))

	built.ask(t, "read the notes and the brand file")

	first, _ := requestsCarrying(built, loop.TheMarkHint)
	if first != 2 {
		t.Errorf("the hint first rode on model call %d, want call 2, the one after the lone step mark", first)
	}
	requests := built.model.Requests()
	if said := strings.Count(wholeRequestText(requests[len(requests)-1]), loop.TheMarkHint); said != 1 {
		t.Errorf("the last request carries the hint %d times, want once for the task, whatever the lone marks after the first", said)
	}
}
