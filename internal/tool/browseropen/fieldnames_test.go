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

func TestAnAddressThatIsNotAWebAddressIsRefusedWithWhatToWriteInstead(t *testing.T) {
	written := map[string]string{
		"file:///etc/passwd":       "web address",
		"https://":                 "host",
		"https://%zz-not-a-parse/": "web address",
		"   ":                      "web address",
	}
	for address, says := range written {
		tool, _ := newTool(t)
		_, err := run(t, tool, map[string]any{"intent": "read the page", "url": address})
		if err == nil {
			t.Errorf("the browser was sent to %q", address)
			continue
		}
		if !strings.Contains(err.Error(), says) {
			t.Errorf("the refusal for %q reads %q and does not say %q", address, err, says)
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
