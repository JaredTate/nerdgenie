// The tests for the lift: an ask under the six headings of the work order
// becomes a job before the model is called once, with the person's done lines,
// rules and tasks in the job's record, and a plain ask is left to the model as
// before.
package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/workorder"
)

const aWorkOrderAsk = `# Notes app

## Goal
A notes app for one person, in the browser. Because he wants his notes in one place.

## Where
A new, empty folder.

## Done when
1. Every test passes. [tests pass: npm test]
2. A note survives a reload.

## Rules
- Plain JavaScript, no framework.

## Tasks
1. Scaffold the app and read the notes. (Details: Storage)
2. The list. (Details: List)

## Details
### Storage
Notes live in local storage under one key.

### List
One line per note, newest first.
`

// TestAWorkOrderMakesTheJobBeforeTheFirstCall: the ask is a work order, so
// the harness makes the job itself, with the two tasks, the two done lines and
// the rules, and the model is not called at all until the job's first task
// runs, whose first request already carries all of it in the job summary.
func TestAWorkOrderMakesTheJobBeforeTheFirstCall(t *testing.T) {
	built := newHarness(t, closingScript("the notes are read"), scriptedTool("read", "the notes"))
	outcome := built.ask(t, aWorkOrderAsk)
	if calls := len(built.model.Requests()); calls != 0 {
		t.Fatalf("the model was called %d times while the work order was lifted, want none", calls)
	}
	if outcome.Status != contract.StatusDone || !strings.Contains(outcome.Report, "job 1") || !strings.Contains(outcome.Report, "2 tasks") {
		t.Errorf("the lift ended %q with %q, want done and a report naming job 1 and its 2 tasks", outcome.Status, outcome.Report)
	}
	tasks := built.jobs.Tasks("1")
	if len(tasks) != 2 || !strings.HasPrefix(tasks[0].Text, "Scaffold the app") || !strings.Contains(tasks[0].Text, "(Details: Storage)") {
		t.Fatalf("job 1 holds the tasks %+v, want the two of the work order with their details kept", tasks)
	}
	if summary := theSummaryOf(t, built, "1"); summary.State != contract.JobRunning || summary.Title != "Notes app" {
		t.Errorf("job 1 reads %+v, want it running and named by the ask's title", summary)
	}
	more, err := built.loop.RunNextJobTask(t.Context(), built.channel)
	if err != nil || !more {
		t.Fatalf("the job's first task did not run: more=%v err=%v", more, err)
	}
	first := wholeRequestText(built.model.Requests()[0])
	for _, wanted := range []string{
		"Done when:",
		"Every test passes. [tests pass: npm test]",
		"A note survives a reload.",
		"C1 \"" + workorder.TestsFirst + "\"",
		"C2 \"Plain JavaScript, no framework.\"",
		"t1 Scaffold the app",
		"t2 The list.",
	} {
		if !strings.Contains(first, wanted) {
			t.Errorf("the first request to the model lacks %q", wanted)
		}
	}
}

// TestAWorkOrdersRulesRideAsTheRecordsRulesWithTestsFirstFirst: the job's
// record holds the rules in the person's words, and tests first is the first
// of them whether or not the person wrote it.
func TestAWorkOrdersRulesRideAsTheRecordsRulesWithTestsFirstFirst(t *testing.T) {
	built := newHarness(t, nil)
	built.ask(t, aWorkOrderAsk)
	held, err := built.jobs.Load(t.Context(), "1")
	if err != nil {
		t.Fatalf("cannot load job 1: %v", err)
	}
	rules := held.Rules.Corrections
	if len(rules) != 2 || rules[0].Text != workorder.TestsFirst || rules[1].Text != "Plain JavaScript, no framework." {
		t.Errorf("the job's rules read %+v, want tests first and then the person's one rule", rules)
	}
	if len(held.Goal.DoneWhen) != 2 || held.Goal.DoneWhen[0].Text != "Every test passes. [tests pass: npm test]" || held.Goal.DoneWhen[0].Done {
		t.Errorf("the job's done list reads %+v, want the two lines as written, none of them done", held.Goal.DoneWhen)
	}
	if held.Goal.Name != "Notes app" || !strings.HasPrefix(held.Goal.Why, "A notes app for one person") {
		t.Errorf("the job is named %q with why %q, want the title and the goal", held.Goal.Name, held.Goal.Why)
	}
	if held.Goal.Ask != aWorkOrderAsk {
		t.Errorf("the job's ask is not the work order word for word")
	}
}

// TestAPlainAskIsUnchanged: an ask without the headings runs as it always did,
// through the model, and makes no job.
func TestAPlainAskIsUnchanged(t *testing.T) {
	built := newHarness(t, closingScript("the notes are read"), scriptedTool("read", "the notes"))
	outcome := built.ask(t, "read the notes and tell me what they say")
	if len(built.model.Requests()) == 0 {
		t.Fatalf("the model was not called for a plain ask")
	}
	if outcome.Status != contract.StatusDone {
		t.Errorf("the plain ask ended %q, want done", outcome.Status)
	}
	listed, err := built.jobs.List(t.Context())
	if err != nil || len(listed) != 0 {
		t.Errorf("a plain ask made %d jobs (err %v), want none", len(listed), err)
	}
}

// TestAJobTaskSeesItsOwnDetailsSectionsInFull: the job's first task names the
// Storage section, so its front carries that section whole and the List
// section as a heading with the way to read it, and not the whole work order.
func TestAJobTaskSeesItsOwnDetailsSectionsInFull(t *testing.T) {
	built := newHarness(t, closingScript("the notes are read"), scriptedTool("read", "the notes"))
	built.ask(t, aWorkOrderAsk)
	if _, err := built.loop.RunNextJobTask(t.Context(), built.channel); err != nil {
		t.Fatalf("the job's first task did not run: %v", err)
	}
	first := wholeRequestText(built.model.Requests()[0])
	for _, wanted := range []string{
		loop.TheDetailsHeading,
		"### Storage\nNotes live in local storage under one key.",
		"List",
		loop.TheOtherSectionsLine,
	} {
		if !strings.Contains(first, wanted) {
			t.Errorf("the first request lacks %q", wanted)
		}
	}
	if strings.Contains(first, "One line per note, newest first.") {
		t.Errorf("the first request carries the List section's body, which the task did not name")
	}
	if strings.Contains(first, "## Details") {
		t.Errorf("the first request carries the whole work order")
	}
}

// TestATaskNamingNoSectionsSeesEveryHeadingByLine: a task that names no
// section is shown every heading by line and no body.
func TestATaskNamingNoSectionsSeesEveryHeadingByLine(t *testing.T) {
	plain := strings.Replace(aWorkOrderAsk, "1. Scaffold the app and read the notes. (Details: Storage)", "1. Scaffold the app and read the notes.", 1)
	built := newHarness(t, closingScript("the notes are read"), scriptedTool("read", "the notes"))
	built.ask(t, plain)
	if _, err := built.loop.RunNextJobTask(t.Context(), built.channel); err != nil {
		t.Fatalf("the job's first task did not run: %v", err)
	}
	first := wholeRequestText(built.model.Requests()[0])
	if strings.Contains(first, loop.TheDetailsHeading) {
		t.Errorf("a task naming no section was shown a details block")
	}
	if !strings.Contains(first, loop.TheOtherSectionsLine+" Storage, List") {
		t.Errorf("the first request does not list the headings by line: %q", first)
	}
	for _, body := range []string{"under one key", "newest first"} {
		if strings.Contains(first, body) {
			t.Errorf("the first request carries a section body %q the task did not name", body)
		}
	}
}
