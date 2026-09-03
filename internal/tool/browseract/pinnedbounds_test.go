package browseract_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/tool/browseract"
)

// TestEveryBoundOfTheActToolIsTheNumberItSays writes each bound out as the
// literal it is, with the reason it is that number beside it.
func TestEveryBoundOfTheActToolIsTheNumberItSays(t *testing.T) {
	if browseract.MaxSteps != 10 {
		t.Errorf("MaxSteps is %d, want ten: a batch longer than ten steps is a plan, and a plan belongs in the record",
			browseract.MaxSteps)
	}
	if browseract.MaxScrollAmount != 20 {
		t.Errorf("MaxScrollAmount is %d, want twenty: a page is read a screen at a time, and a longer scroll is a loop",
			browseract.MaxScrollAmount)
	}
}

func TestABatchOfElevenStepsIsRefused(t *testing.T) {
	tool, _ := newTool(t)
	steps := []map[string]any{}
	for at := 0; at < 11; at++ {
		steps = append(steps, map[string]any{
			"method": "click", "element": testkit.FixtureChangeLinkRef, "expectation": "the page changes",
		})
	}

	_, err := run(t, tool, map[string]any{"intent": "click eleven times", "steps": steps})
	if err == nil {
		t.Fatalf("a batch of eleven steps was run, and the cap is ten")
	}
	if !strings.Contains(err.Error(), "pieces") {
		t.Errorf("the refusal reads %q and does not say what to do instead", err)
	}
}

func TestAScrollOfTwentyOneStepsIsRefused(t *testing.T) {
	tool, _ := newTool(t)

	_, err := run(t, tool, map[string]any{
		"intent": "read to the bottom",
		"steps": []map[string]any{{
			"method": "scroll", "direction": "down", "amount": 21, "expectation": "the end of the page is shown",
		}},
	})
	if err == nil {
		t.Fatalf("a scroll of twenty-one steps was run, and the cap is twenty")
	}
}
