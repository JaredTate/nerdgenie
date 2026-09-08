package loop_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// Run 21: sixteen of the model's eighty-six calls did nothing but tell the
// record a plan step was done or a done line was proved, one round each, and
// the first-line form the instructions offer was used three times. A call to
// a file tool may now carry "done", a plan step, and "proves", a done line,
// and the harness marks them with that call's own result once it succeeds.

func aReadWithMarks(t *testing.T, arguments string, tools ...contract.Tool) *harness {
	t.Helper()
	if len(tools) == 0 {
		tools = []contract.Tool{scriptedTool(contract.ToolRead, "the notes", "the brand file")}
	}
	return newHarness(t, []testkit.Step{
		aPlanOfThree(),
		callStep("The notes are read. Next: the brand file.", callFor("c2", contract.ToolRead, arguments)),
		answerStep("What changed: nothing. What I checked: both files. What is left: the post."),
	}, tools...)
}

func TestAStepAndADoneLineRideOnTheCallThatFinishesThem(t *testing.T) {
	read := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolRead, Description: "A read the test scripted.", Classes: []contract.PermissionClass{contract.ClassRead}}, "the notes", "the brand file")
	built := aReadWithMarks(t, `{"path":"brand.md","done":1,"proves":1}`, read)

	outcome := built.ask(t, "read the notes and the brand file")

	held := built.held(t, outcome.TaskID)
	if len(held.Work.Plan) != 3 || !held.Work.Plan[0].Done || held.Work.Plan[0].ResultID != "r3" || held.Work.Plan[1].Done {
		t.Errorf("after a read carrying done:1 the plan reads %+v, want step 1 done by r3 and step 2 open", held.Work.Plan)
	}
	if len(held.Goal.DoneWhen) != 2 || !held.Goal.DoneWhen[0].Done || held.Goal.DoneWhen[0].ResultID != "r3" || held.Goal.DoneWhen[1].Done {
		t.Errorf("after a read carrying proves:1 the done list reads %+v, want line 1 proved by r3 and line 2 open", held.Goal.DoneWhen)
	}
	inputs := read.Inputs()
	if len(inputs) != 2 || strings.Contains(string(inputs[1]), "done") || strings.Contains(string(inputs[1]), "proves") {
		t.Errorf("the read tool was given %s, want the marks taken off before the tool saw the call", inputs)
	}
	requests := requestsJoined(built.model.Requests())
	if !strings.Contains(requests, "step 1 done by r3") || !strings.Contains(requests, "done line 1 proved by r3") {
		t.Errorf("the result does not say what was marked; the requests read:\n%s", requests)
	}
}

func TestAFailedCallMarksNothing(t *testing.T) {
	broken := &failingTool{name: contract.ToolRead, reason: errors.New("the file brand.md is not there, so check the path")}
	built := newHarness(t, []testkit.Step{
		callStep("I will plan and read.", taskCall("p1", `{"plan":["read the brand file","write the post"],"doneWhen":["the brand file is read"]}`)),
		callStep("Next: the brand file.", callFor("c2", contract.ToolRead, `{"path":"brand.md","done":1,"proves":1}`)),
		answerStep("What changed: nothing. What I checked: nothing. What is left: the post."),
	}, broken)

	outcome := built.ask(t, "read the brand file")

	held := built.held(t, outcome.TaskID)
	if len(held.Work.Plan) != 2 || held.Work.Plan[0].Done || held.Goal.DoneWhen[0].Done {
		t.Errorf("a failed read carrying marks marked something: plan %+v, done list %+v", held.Work.Plan, held.Goal.DoneWhen)
	}
	requests := requestsJoined(built.model.Requests())
	if !strings.Contains(requests, "step 1 not marked: the call failed") || !strings.Contains(requests, "done line 1 not proved: the call failed") {
		t.Errorf("the failed result does not say the marks were not made; the requests read:\n%s", requests)
	}
}

func TestAMarkOnANumberTheRecordDoesNotHaveIsSaidInTheResult(t *testing.T) {
	built := aReadWithMarks(t, `{"path":"brand.md","done":9,"proves":7}`)

	outcome := built.ask(t, "read the notes and the brand file")

	held := built.held(t, outcome.TaskID)
	for _, step := range held.Work.Plan {
		if step.Done {
			t.Errorf("a mark on step 9 of a three-step plan marked %+v", step)
		}
	}
	requests := requestsJoined(built.model.Requests())
	if !strings.Contains(requests, "step 9 not marked") || !strings.Contains(requests, "done line 7 not proved") {
		t.Errorf("the result does not say the numbers were wrong; the requests read:\n%s", requests)
	}
}

func TestTheFileToolsTellTheModelAboutTheMarks(t *testing.T) {
	built := aReadWithMarks(t, `{"path":"brand.md"}`)
	built.ask(t, "read the notes and the brand file")

	specs := built.model.Requests()[0].Tools
	carries := map[string]bool{}
	for _, spec := range specs {
		for _, field := range spec.Fields {
			if field.Name == "done" || field.Name == "proves" {
				carries[spec.Name] = true
			}
		}
	}
	for _, name := range []string{contract.ToolRead, contract.ToolWrite, contract.ToolEdit, contract.ToolShell, contract.ToolSearch} {
		if _, offered := specFor(specs, name); offered && !carries[name] {
			t.Errorf("the %s tool does not tell the model about done and proves", name)
		}
	}
	if carries[contract.ToolTask] {
		t.Errorf("the task tool carries the marks, and it has its own operations for them")
	}
	if len(loop.TheMarkFields) != 2 {
		t.Errorf("the mark fields read %+v, want done and proves", loop.TheMarkFields)
	}
}

func specFor(specs []contract.ToolSpec, name string) (contract.ToolSpec, bool) {
	for _, spec := range specs {
		if spec.Name == name {
			return spec, true
		}
	}
	return contract.ToolSpec{}, false
}
