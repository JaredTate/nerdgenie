package browseract_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// A click step takes a point, x and y in whole pixels from the top left of the
// page, in place of an element, the way browser_click does, and the batch
// passes it through to the worker.
func TestAClickStepAtAPointGoesToTheWorkerAsAPoint(t *testing.T) {
	tool, worker := newTool(t)

	output, err := run(t, tool, map[string]any{
		"intent": "follow the link, then click Jupiter on the canvas",
		"steps": []any{
			map[string]any{"method": "click", "element": testkit.FixtureChangeLinkRef, "expectation": "the page says it changed"},
			map[string]any{"method": "click", "x": 519, "y": "335", "expectation": "the info panel names Jupiter"},
		},
	})
	if err != nil {
		t.Fatalf("a batch with a click at a point failed: %v", err)
	}
	if strings.Count(output.Text, "step ") != 2 {
		t.Errorf("the batch answered %q, want both steps to have run", output.Text)
	}
	if points := worker.PointsClicked(); len(points) != 1 || points[0] != (testkit.Point{X: 519, Y: 335}) {
		t.Errorf("the worker was handed the points %v, want the one point 519,335", points)
	}
}

func TestAClickStepNamingBothAnElementAndAPointIsRefused(t *testing.T) {
	tool, worker := newTool(t)

	_, err := run(t, tool, map[string]any{
		"intent": "click the link",
		"steps": []any{map[string]any{
			"method": "click", "element": testkit.FixtureChangeLinkRef, "x": 10, "y": 20, "expectation": "the page changes",
		}},
	})
	if err == nil {
		t.Fatal("a click step naming both an element and a point was run, and it must name one or the other")
	}
	if !strings.Contains(err.Error(), "both") || !strings.Contains(err.Error(), "step 1") {
		t.Errorf("the refusal reads %q, want it to say step 1 names both", err)
	}
	if points := worker.PointsClicked(); len(points) != 0 {
		t.Errorf("the worker was handed the points %v, and the refused batch must not reach it", points)
	}
}

func TestAClickStepNamingNeitherIsRefused(t *testing.T) {
	tool, _ := newTool(t)

	_, err := run(t, tool, map[string]any{
		"intent": "click something",
		"steps":  []any{map[string]any{"method": "click", "expectation": "something happens"}},
	})
	if err == nil {
		t.Fatal("a click step naming neither an element nor a point was run, and there is nothing to click")
	}
	if !strings.Contains(err.Error(), "neither") || !strings.Contains(err.Error(), "step 1") {
		t.Errorf("the refusal reads %q, want it to say step 1 names neither", err)
	}

	_, err = run(t, tool, map[string]any{
		"intent": "type something",
		"steps":  []any{map[string]any{"method": "type", "x": 10, "y": 20, "text": "hello", "expectation": "the box holds it"}},
	})
	if err == nil {
		t.Error("a type step at a point was run, and typing needs an element")
	}
}
