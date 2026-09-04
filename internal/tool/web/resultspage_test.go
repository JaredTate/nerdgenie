package web_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/tool/web"
)

// The two pages saved from DuckDuckGo on the third of September 2026, each
// fetched once by hand with curl for the query "digibyte blockchain". The first
// is a real results page. The second is what the site answers a plain program
// with: a page asking whoever is searching to prove they are a person. No test
// in this package ever reaches the real internet, so both are read from the
// files rather than fetched again.
const (
	// realResultsPage is a results page with ten results on it.
	realResultsPage = "a_results_page_from_duckduckgo.html"
	// humanCheckPage is the page that asks for a human check instead.
	humanCheckPage = "a_human_check_page_from_duckduckgo.html"
	// theSavedQuery is the query both pages were fetched for.
	theSavedQuery = "digibyte blockchain"
)

// readingASavedPage builds the web tool over a server that answers every search
// with one saved page, which is how a real page from the web is read without
// reaching the web.
func readingASavedPage(t *testing.T, fixture string) *web.Tool {
	t.Helper()
	held, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatalf("cannot read the saved page %s: %v", fixture, err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write(held)
	}))
	t.Cleanup(server.Close)
	return web.New(web.Settings{ResultsPageAddress: server.URL + "/html/"})
}

func TestTodaysRealResultsPageIsReadIntoRows(t *testing.T) {
	tool := readingASavedPage(t, realResultsPage)

	output, err := run(t, tool, map[string]any{"action": "search", "query": theSavedQuery})
	if err != nil {
		t.Fatalf("searching the saved results page failed: %v", err)
	}
	for _, wanted := range []string{
		"DigiByte — Faster, Safer, Forward-thinking Blockchain",
		"https://www.digibyte.org/en-us/",
		"DigiByte is a highly secure, multi-algorithm UTXO blockchain",
	} {
		if !strings.Contains(output.Text, wanted) {
			t.Errorf("the rows read off today's results page do not hold %q:\n%s", wanted, output.Text)
		}
	}
	if strings.Contains(output.Text, "duckduckgo.com/l/") {
		t.Errorf("the results page's own redirect is still in the rows:\n%s", output.Text)
	}
}

func TestTodaysRealResultsPageGivesTheModelAsManyRowsAsTheCapAllows(t *testing.T) {
	tool := readingASavedPage(t, realResultsPage)

	output, err := run(t, tool, map[string]any{"action": "search", "query": theSavedQuery})
	if err != nil {
		t.Fatalf("searching the saved results page failed: %v", err)
	}
	if rows := strings.Count(output.Text, "https://"); rows < web.MaxSearchRows {
		t.Errorf("today's results page gave %d addresses, want the %d the cap allows:\n%s",
			rows, web.MaxSearchRows, output.Text)
	}
}

func TestThePageThatAsksForAHumanCheckSaysSoRatherThanFindingNothing(t *testing.T) {
	tool := readingASavedPage(t, humanCheckPage)

	output, err := run(t, tool, map[string]any{"action": "search", "query": theSavedQuery})
	if err != nil {
		t.Fatalf("searching a page that asks for a human check failed: %v", err)
	}
	if !strings.Contains(output.Text, "human") {
		t.Errorf("the page that asks for a human check read as %q, and the model cannot tell why it found nothing", output.Text)
	}
	if strings.Contains(output.Text, "try other words") {
		t.Errorf("the page that asks for a human check told the model to try other words, which will not help: %q", output.Text)
	}
	if !strings.Contains(output.Text, "search_server_address") {
		t.Errorf("the refusal reads %q and does not say what the user can set to search another way", output.Text)
	}
}
