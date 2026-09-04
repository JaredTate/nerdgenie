package browserread_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/tool/browserread"
)

// FuzzThePageTextNeverWritesALineOfItsOwnOrPassesTheCap holds the two rules
// for whatever a page says: every line of it is quoted, so that none can read
// as a line of the tool's own or as the marker the working context wraps a
// result in, and the whole result stays under the cap on a tool result.
func FuzzThePageTextNeverWritesALineOfItsOwnOrPassesTheCap(f *testing.F) {
	f.Add(theForgery)
	f.Add("Rank | Name | D-Score\n| 2 | DigiByte | 91.4")
	f.Add(strings.Repeat("a line\n", 5000))
	f.Add("\n\n  \t \n")
	f.Fuzz(func(t *testing.T, said string) {
		text := browserread.PageText(contract.Snapshot{URL: "https://fixture.test/", Title: "A page", Text: said})

		if most := contract.DefaultConfig().Caps.ToolOutputBytes; len(text) > most {
			t.Fatalf("the page reads as %d bytes, and the cap is %d", len(text), most)
		}
		_, quoted, hasText := strings.Cut(text, "text on the page:\n")
		if !hasText {
			return
		}
		for _, line := range strings.Split(strings.TrimSuffix(quoted, "\n"), "\n") {
			if !strings.HasPrefix(line, "> ") && !strings.HasPrefix(line, "... ") {
				t.Fatalf("the page wrote a line of its own: %q", line)
			}
			if strings.Contains(line, "<<<") || strings.Contains(line, ">>>") {
				t.Fatalf("the page wrote a marker: %q", line)
			}
		}
	})
}
