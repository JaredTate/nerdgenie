package browserclick_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/testkit"
)

func TestAClickThatCallsTheElementARefStillFindsIt(t *testing.T) {
	names := []string{"ref", "element_ref", "elementRef", "target"}
	for _, name := range names {
		tool, _ := newTool(t)
		_, err := run(t, tool, map[string]any{
			"intent": "follow the link", name: testkit.FixtureChangeLinkRef, "expectation": "the page changes",
		})
		if err != nil {
			t.Errorf("a click with the element under %q was refused: %v", name, err)
		}
	}
}

func TestAClickWithNoElementNamesTheFieldToWrite(t *testing.T) {
	tool, _ := newTool(t)

	_, err := run(t, tool, map[string]any{"intent": "follow the link", "expectation": "the page changes"})
	if err == nil {
		t.Fatalf("a click that names no element was made")
	}
	if !strings.HasSuffix(err.Error(), `"element"`) {
		t.Errorf("the refusal reads %q and does not end with the name of the field to write", err)
	}
}
