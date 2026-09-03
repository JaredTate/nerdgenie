package browseropen_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/testkit"
)

func TestAnOpenThatCallsTheAddressSomethingElseStillGoesThere(t *testing.T) {
	names := []string{"address", "link", "page", "URL"}
	for _, name := range names {
		tool, _ := newTool(t)
		output, err := run(t, tool, map[string]any{"intent": "read the page", name: testkit.FixtureSimplePage})
		if err != nil {
			t.Errorf("an open with the address under %q was refused: %v", name, err)
			continue
		}
		if !strings.Contains(output.Text, "A simple page") {
			t.Errorf("an open with the address under %q returned %q", name, output.Text)
		}
	}
}

func TestAnOpenWithNoAddressNamesTheFieldToWrite(t *testing.T) {
	tool, _ := newTool(t)

	_, err := run(t, tool, map[string]any{"intent": "read the page", "site": "fixture.test"})
	if err == nil {
		t.Fatalf("an open that names no page was made")
	}
	if !strings.HasSuffix(err.Error(), `"url"`) {
		t.Errorf("the refusal reads %q and does not end with the name of the field to write", err)
	}
}
