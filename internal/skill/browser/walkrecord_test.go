package browser_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/skill/browser"
	"github.com/JaredTate/coeus/internal/testkit"
)

// stepsOfTheWalk reads back what a recording wrote down, failing the test when
// it cannot be read as a walk at all.
func stepsOfTheWalk(t *testing.T, built *bench, name string) []browser.Step {
	t.Helper()
	steps, err := browser.StepsOf(built.load(t, name))
	if err != nil {
		t.Fatalf("what was recorded does not read back as a walk: %v", err)
	}
	return steps
}

func TestWalkRecordWritesDownWhatThePersonDoesUntilWalkStop(t *testing.T) {
	built := newBench(t)
	if _, err := built.worker.Open(context.Background(), testkit.FixtureSimplePage); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	walk := theWalkCommand(built, nil)

	started := runTheWalkCommand(t, walk, "record shop-walk")
	if !strings.Contains(started, "/walk stop") {
		t.Errorf("recording said %q, and it should say that /walk stop finishes it", started)
	}
	built.worker.PersonDoes(
		contract.BrowserEvent{Kind: contract.BrowserEventClick, Ref: "e1", Text: "Change the page"},
		contract.BrowserEvent{Kind: contract.BrowserEventType, Ref: "e3", Length: 10},
		contract.BrowserEvent{Kind: contract.BrowserEventNavigate, Address: testkit.FixtureChangedPage},
	)
	finished := runTheWalkCommand(t, walk, "stop")
	if !strings.Contains(finished, "shop-walk") || !strings.Contains(finished, "4 steps") {
		t.Errorf("stopping said %q, and it should name the walk and how many steps it holds", finished)
	}

	steps := stepsOfTheWalk(t, built, "shop-walk")
	if len(steps) != 4 {
		t.Fatalf("the recording holds %d steps, want the page it started on and the three things the person did: %+v", len(steps), steps)
	}
	if steps[0].Tool != contract.ToolBrowserOpen || steps[0].Address != testkit.FixtureSimplePage {
		t.Errorf("step 1 is %+v, want the page the browser was on", steps[0])
	}
	if steps[1].Tool != contract.ToolBrowserClick || steps[1].Element.Ref != "e1" {
		t.Errorf("step 2 is %+v, want a click on the element the person clicked", steps[1])
	}
	if steps[1].Element.Shown != "Change the page" || steps[1].Expectation != "the page answers the click" {
		t.Errorf("step 2 is %+v, want the text of what was clicked and the expectation that the page answers it", steps[1])
	}
	if steps[2].Tool != contract.ToolBrowserType || steps[2].Element.Ref != "e3" {
		t.Errorf("step 3 is %+v, want a typing step on the box the person typed into", steps[2])
	}
	if steps[2].Typed != "" || !strings.Contains(steps[2].Intent, "10") {
		t.Errorf("step 3 is %+v, want no text at all and the length noted for the person to fill in", steps[2])
	}
	if steps[3].Tool != contract.ToolBrowserOpen || steps[3].Address != testkit.FixtureChangedPage {
		t.Errorf("step 4 is %+v, want an opening step on the page the person went to", steps[3])
	}
}

func TestWalkStopWithNothingBeingRecordedSaysSo(t *testing.T) {
	built := newBench(t)
	walk := theWalkCommand(built, nil)

	said, err := walk.Run(context.Background(), "stop", contract.CommandContext{})
	if err == nil {
		t.Fatalf("/walk stop with nothing being recorded answered %q, and it should say there is nothing to stop", said)
	}
	if !strings.Contains(err.Error(), "record") {
		t.Errorf("/walk stop said %q, and it should say how to start a recording", err)
	}
}

func TestWalkRecordRefusesASecondRecordingWhileOneIsRunning(t *testing.T) {
	built := newBench(t)
	if _, err := built.worker.Open(context.Background(), testkit.FixtureSimplePage); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	walk := theWalkCommand(built, nil)
	runTheWalkCommand(t, walk, "record first-walk")

	_, err := walk.Run(context.Background(), "record second-walk", contract.CommandContext{})
	if err == nil {
		t.Fatal("a second recording started while the first was still running, and there is one browser")
	}
	if !strings.Contains(err.Error(), "first-walk") || !strings.Contains(err.Error(), "/walk stop") {
		t.Errorf("the refusal says %q, and it should name the walk being recorded and how to finish it", err)
	}
	runTheWalkCommand(t, walk, "stop")
}

func TestWalkStopAfterTheBrowserWentSavesWhatThereIs(t *testing.T) {
	built := newBench(t)
	if _, err := built.worker.Open(context.Background(), testkit.FixtureSimplePage); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	walk := theWalkCommand(built, nil)
	runTheWalkCommand(t, walk, "record short-walk")

	built.worker.PersonDoes(contract.BrowserEvent{Kind: contract.BrowserEventClick, Ref: "e1", Text: "Change the page"})
	if err := built.worker.Close(); err != nil {
		t.Fatalf("closing the fixture browser failed: %v", err)
	}
	finished := runTheWalkCommand(t, walk, "stop")

	if !strings.Contains(finished, "short-walk") {
		t.Errorf("stopping said %q, and it should name the walk it saved", finished)
	}
	steps := stepsOfTheWalk(t, built, "short-walk")
	if len(steps) != 2 {
		t.Errorf("the recording holds %d steps, want the page it started on and the one click that arrived: %+v", len(steps), steps)
	}
}
