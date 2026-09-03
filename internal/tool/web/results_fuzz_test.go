package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FuzzTheResultsPageReader throws any text at the step that reads a results page
// into rows and asserts what must always hold of the answer: the reader never
// panics, it always says something, a page with no result link on it comes back
// as one of the two sentences that say which kind of nothing it was, and the
// rows are bounded, each row being three lines and there being at most
// MaxSearchRows of them. The two pages saved from the real site are the first
// seeds, so that the fuzzer starts from the markup the reader is for.
func FuzzTheResultsPageReader(f *testing.F) {
	for _, fixture := range []string{"a_results_page_from_duckduckgo.html", "a_human_check_page_from_duckduckgo.html"} {
		held, err := os.ReadFile(filepath.Join("testdata", fixture))
		if err != nil {
			f.Fatalf("cannot read the saved page %s: %v", fixture, err)
		}
		f.Add(string(held))
	}
	for _, seed := range []string{
		"",
		"<a ",
		"<a class=result__a",
		`<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com&amp;rut=1">a title</a>`,
		`<a class="result__a" href="/x">never closed`,
		`<a class="result__snippet">a snippet with no title before it</a>`,
		`<a class="RESULT__A" href="/x">a class written in capitals</a>`,
		strings.Repeat(`<a class="result__a" href="/x">t</a>`, MaxSearchRows*3),
		`<a class="result__a" href="/x">` + strings.Repeat("a line<br>", MaxSearchRows*4+1) + "</a>",
		"<p>anomaly-modal</p>",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, page string) {
		if len(page) > 1<<18 {
			t.Skip("the fuzzer wrote more text than one page ever carries")
		}
		read := rowsFromResultsPage(page)
		if read == "" {
			t.Fatalf("the reader had nothing at all to say about a page of %d bytes", len(page))
		}
		if !strings.Contains(page, resultTitleClass) && read != noResultRead && read != humanCheckAsked {
			t.Fatalf("a page with no result link on it read as %q rather than as one of the two sentences that say so", read)
		}
		if lines := strings.Count(read, "\n"); lines > 4*MaxSearchRows {
			t.Fatalf("the rows run to %d lines, and %d rows of three lines each with a blank line between them is the most a reader may give",
				lines, MaxSearchRows)
		}
	})
}
