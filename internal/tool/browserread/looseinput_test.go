package browserread_test

import (
	"strings"
	"testing"
)

func TestReadingOnlyWhatIsAboveTheFoldTakesTheWordTrue(t *testing.T) {
	for _, written := range []any{true, "true", "yes"} {
		tool, _ := newTool(t)
		output, err := run(t, tool, map[string]any{"intent": "see the page", "visible_only": written})
		if err != nil {
			t.Errorf("a call with visible_only written as %v was refused: %v", written, err)
			continue
		}
		if strings.Contains(output.Text, "below the fold") {
			t.Errorf("a call with visible_only written as %v read the whole page: %q", written, output.Text)
		}
	}
}

func TestAReadWithNoIntentNamesTheFieldToWrite(t *testing.T) {
	tool, _ := newTool(t)

	_, err := run(t, tool, map[string]any{"purpose": "see the page"})
	if err == nil {
		t.Fatalf("a call that does not say what the step is for was made")
	}
	if !strings.Contains(err.Error(), "intent") {
		t.Errorf("the refusal reads %q and does not name the field to write", err)
	}
}
