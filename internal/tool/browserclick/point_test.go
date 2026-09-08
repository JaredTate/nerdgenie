package browserclick_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// The solar-system job on 7 September 2026 could not click a planet: the page
// is a canvas, the outline never lists one, and a click took only an element's
// reference. A click at a point, in whole pixels from the top left of the page,
// goes to the worker as that point.
func TestAClickAtAPointGoesToTheWorkerAsAPoint(t *testing.T) {
	tool, worker := newTool(t)

	output, err := run(t, tool, map[string]any{
		"intent": "click Jupiter on the canvas", "x": 519, "y": "335", "expectation": "the info panel names Jupiter",
	})
	if err != nil {
		t.Fatalf("clicking a point failed: %v", err)
	}
	if points := worker.PointsClicked(); len(points) != 1 || points[0] != (testkit.Point{X: 519, Y: 335}) {
		t.Errorf("the worker was handed the points %v, want the one point 519,335", points)
	}
	if !strings.Contains(output.Text, testkit.FixtureSimplePage) {
		t.Errorf("the click at a point answered %q, want what changed on the page", output.Text)
	}
}

func TestAClickNamingBothAnElementAndAPointIsRefused(t *testing.T) {
	tool, worker := newTool(t)

	_, err := run(t, tool, map[string]any{
		"intent": "click the link", "element": testkit.FixtureChangeLinkRef, "x": 10, "y": 20, "expectation": "the page changes",
	})
	if err == nil {
		t.Fatal("a click naming both an element and a point was made, and it must name one or the other")
	}
	if !strings.Contains(err.Error(), "both") || strings.Contains(err.Error(), ". ") {
		t.Errorf("the refusal reads %q, want one sentence saying the call names both", err)
	}
	if points := worker.PointsClicked(); len(points) != 0 {
		t.Errorf("the worker was handed the points %v, and the refused call must not reach it", points)
	}
}

func TestAClickNamingNeitherIsRefused(t *testing.T) {
	tool, _ := newTool(t)

	_, err := run(t, tool, map[string]any{"intent": "click something", "expectation": "something happens"})
	if err == nil {
		t.Fatal("a click naming neither an element nor a point was made, and there is nothing to click")
	}
	if !strings.Contains(err.Error(), "neither") || strings.Contains(err.Error(), ". ") {
		t.Errorf("the refusal reads %q, want one sentence saying the call names neither", err)
	}
	if !strings.Contains(err.Error(), "element") || !strings.Contains(err.Error(), "x and y") {
		t.Errorf("the refusal reads %q and does not say what to send instead", err)
	}

	if _, err := run(t, tool, map[string]any{"intent": "click something", "x": 10, "expectation": "something happens"}); err == nil {
		t.Error("a click naming half a point was made, and a point needs both x and y")
	}
}

func TestTheDescriptionSaysAPointIsForACanvas(t *testing.T) {
	tool, _ := newTool(t)
	spec := tool.Spec()

	if !strings.Contains(spec.Description, "canvas") {
		t.Errorf("the description reads %q and does not say that a point is for a canvas or anything the outline does not list", spec.Description)
	}
	for _, name := range []string{"x", "y"} {
		found := false
		for _, field := range spec.Fields {
			if field.Name == name {
				found = true
				if field.Required {
					t.Errorf("the field %s is marked required, and a click by element writes no point", name)
				}
			}
		}
		if !found {
			t.Errorf("the tool has no field %s, and a click at a point needs it", name)
		}
	}
}
