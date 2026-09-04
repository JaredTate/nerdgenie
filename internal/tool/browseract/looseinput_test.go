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
