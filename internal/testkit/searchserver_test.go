package testkit_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/testkit"
)

// get fetches one address and returns its status and body.
func get(t *testing.T, address string) (int, string) {
	t.Helper()
	answer, err := http.Get(address)
	if err != nil {
		t.Fatalf("fetching %s failed: %v", address, err)
	}
	defer answer.Body.Close()
	body, err := io.ReadAll(answer.Body)
	if err != nil {
		t.Fatalf("reading %s failed: %v", address, err)
	}
	return answer.StatusCode, string(body)
}

func TestTheFakeSearchServerAnswersInTheShapeSearxngUses(t *testing.T) {
	search := testkit.NewFakeSearchServer()
	defer search.Close()

	code, body := get(t, search.SearchAddress()+"?q=digibyte&format=json")

	if code != http.StatusOK {
		t.Fatalf("the search answered %d, want 200", code)
	}
	var answer struct {
		Query   string `json:"query"`
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(body), &answer); err != nil {
		t.Fatalf("the search answer is not JSON: %v\n%s", err, body)
	}
	if answer.Query != "digibyte" {
		t.Errorf("the answer says the query was %q, want digibyte", answer.Query)
	}
	if len(answer.Results) == 0 {
		t.Error("the search found nothing, and the fixture has results in it")
	}
}

func TestTheFakeSearchServerAlsoServesADuckDuckGoResultsPage(t *testing.T) {
	search := testkit.NewFakeSearchServer()
	defer search.Close()

	code, body := get(t, search.DuckDuckGoAddress()+"?q=digibyte")

	if code != http.StatusOK {
		t.Fatalf("the results page answered %d, want 200", code)
	}
	if !strings.Contains(body, "result__a") {
		t.Errorf("the results page does not look like the one the web tool reads:\n%s", body)
	}
	if !strings.Contains(body, "result__snippet") {
		t.Errorf("the results page carries no snippets, and the snippet is what the web tool reads:\n%s", body)
	}
	if strings.Count(body, "href=") < 2 {
		t.Errorf("the results page carries fewer than two links, and a results page is made of links:\n%s", body)
	}
	if !strings.Contains(body, "The anniversary is in January.") {
		t.Errorf("the results page carries no snippet text at all:\n%s", body)
	}
	if !strings.Contains(body, "digibyte") {
		t.Errorf("the results page never says what was searched for:\n%s", body)
	}
}

func TestTheFakeSearchServerServesFixturePages(t *testing.T) {
	search := testkit.NewFakeSearchServer()
	defer search.Close()

	code, body := get(t, search.PageAddress("/notes"))
	if code != http.StatusOK {
		t.Fatalf("the fixture page answered %d, want 200", code)
	}
	if !strings.Contains(body, "DigiByte") {
		t.Errorf("the fixture page does not hold what the fixture says:\n%s", body)
	}

	if code, _ := get(t, search.PageAddress("/nowhere")); code != http.StatusNotFound {
		t.Errorf("a page nobody added answered %d, want 404", code)
	}
}

func TestTheFakeSearchServerHasAPageThatTriesToGiveTheAgentInstructions(t *testing.T) {
	search := testkit.NewFakeSearchServer()
	defer search.Close()

	code, body := get(t, search.PageAddress(testkit.InstructionPagePath))

	if code != http.StatusOK {
		t.Fatalf("the instruction page answered %d, want 200", code)
	}
	if !strings.Contains(strings.ToLower(body), "ignore your instructions") {
		t.Errorf("the instruction page does not try to give the agent orders, and that is the whole point of it:\n%s", body)
	}
}

func TestTheFakeSearchServerTakesAPageATestAdds(t *testing.T) {
	search := testkit.NewFakeSearchServer()
	defer search.Close()
	search.AddPage("/product", "<html><body><h1>Product notes</h1></body></html>")

	code, body := get(t, search.PageAddress("/product"))

	if code != http.StatusOK || !strings.Contains(body, "Product notes") {
		t.Errorf("the added page answered %d with %q, want 200 and the page that was added", code, body)
	}
}
