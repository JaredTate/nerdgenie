package browseract_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestAMethodInCapitalsOrWithADashIsTheMethodItPlainlyMeans(t *testing.T) {
	spellings := []string{"CLICK", "Click", "click "}
	for _, spelling := range spellings {
		tool, _ := newTool(t)
		_, err := run(t, tool, map[string]any{
			"intent": "follow the link",
			"steps": []map[string]any{{
				"method": spelling, "element": testkit.FixtureChangeLinkRef, "expectation": "the page changes",
			}},
		})
		if err != nil {
			t.Errorf("a step whose method is written %q was refused: %v", spelling, err)
		}
	}
}

func TestAStepThatCallsTheElementARefStillFindsIt(t *testing.T) {
	tool, _ := newTool(t)

	_, err := run(t, tool, map[string]any{
		"intent": "follow the link",
		"steps": []map[string]any{{
			"method": "click", "ref": testkit.FixtureChangeLinkRef, "expectation": "the page changes",
		}},
	})
	if err != nil {
		t.Errorf("a step with the element under ref was refused: %v", err)
	}
}

func TestAScrollAmountWrittenInQuotesIsStillANumber(t *testing.T) {
	tool, _ := newTool(t)

	_, err := run(t, tool, map[string]any{
		"intent": "read below the fold",
		"steps": []map[string]any{{
			"method": "scroll", "direction": "DOWN", "amount": "3", "expectation": "more of the page is shown",
		}},
	})
	if err != nil {
		t.Errorf("a scroll whose amount is written in quotes was refused: %v", err)
	}
}

func TestAStepWithNoExpectationNamesTheFieldToWrite(t *testing.T) {
	tool, _ := newTool(t)

	_, err := run(t, tool, map[string]any{
		"intent": "follow the link",
		"steps":  []map[string]any{{"method": "click", "element": testkit.FixtureChangeLinkRef}},
	})
	if err == nil {
		t.Fatalf("a step that says nothing about what should happen was run")
	}
	if !strings.Contains(err.Error(), "expectation") {
		t.Errorf("the refusal reads %q and does not name the field to write", err)
	}
}

// TestAStepWithNoMethodIsReadByWhatItCarries: run 23's model wrote a five-step
// batch whose last step named an element and an expectation and no method,
// and the whole batch was refused twice. A step that names no method is
// read by what it carries: an element or a point is a click, text is a
// type, a key is a press; one with nothing to go on is still refused.
func TestAStepWithNoMethodIsReadByWhatItCarries(t *testing.T) {
	for _, shape := range []struct {
		name string
		step map[string]any
		want string
	}{
		{"an element alone is a click", map[string]any{"element": testkit.FixtureChangeLinkRef, "expectation": "the page changes"}, ""},
		{"text is a type", map[string]any{"element": testkit.FixtureUsernameRef, "text": "hello", "expectation": "the box holds hello"}, ""},
		{"a key is a press", map[string]any{"key": "Enter", "expectation": "the form is sent"}, ""},
		{"nothing to go on is refused", map[string]any{"expectation": "something happens"}, "not a method this tool knows"},
	} {
		tool, _ := newTool(t)
		_, err := run(t, tool, map[string]any{"intent": "act", "steps": []map[string]any{shape.step}})
		if shape.want == "" && err != nil {
			t.Errorf("%s: the step was refused: %v", shape.name, err)
		}
		if shape.want != "" && (err == nil || !strings.Contains(err.Error(), shape.want)) {
			t.Errorf("%s: the refusal reads %v, want it to say %q", shape.name, err, shape.want)
		}
	}
}
