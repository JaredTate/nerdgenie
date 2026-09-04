package web_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/tool/web"
)

func TestTheMarksAPageWritesAsNamesOrNumbersComeBackAsCharacters(t *testing.T) {
	for _, written := range []struct {
		page   string
		wanted string
	}{
		{"&#65;", "A"},
		{"&#x42;", "B"},
		{"&nbsp;a", "a"},
		{"&hellip;", "..."},
		{"&notaname;", "&notaname;"},
		{"&#0;", "&#0;"},
		{"&#99999999;", "&#99999999;"},
		{"a & b", "a & b"},
		{"&averyverylongnamehere;", "&averyverylongnamehere;"},
	} {
		if text := web.HTMLToText(written.page); text != written.wanted {
			t.Errorf("the page %q came back as %q, want %q", written.page, text, written.wanted)
		}
	}
}

func TestALinkWithNoAddressAndOneWithAnUnquotedAddressBothRead(t *testing.T) {
	if text := web.HTMLToText(`<a>a link with no address</a>`); !strings.Contains(text, "a link with no address") {
		t.Errorf("a link with no address came back as %q", text)
	}
	if text := web.HTMLToText(`<a href=https://example.com/x>a link</a>`); !strings.Contains(text, "https://example.com/x") {
		t.Errorf("a link with an unquoted address came back as %q", text)
	}
	if text := web.HTMLToText(`<a href="https://example.com/x>a link</a>`); !strings.Contains(text, "example.com") {
		t.Errorf("a link whose quote is never closed came back as %q", text)
	}
}

func TestAPageWithATagThatNeverEndsStillComesBackAsText(t *testing.T) {
	if text := web.HTMLToText("<p>the words<"); !strings.Contains(text, "the words") {
		t.Errorf("a page whose last tag never ends came back as %q", text)
	}
	if text := web.HTMLToText("<!-- a comment that never ends"); text != "" {
		t.Errorf("a comment that never ends came back as %q", text)
	}
	if text := web.HTMLToText("<h9>not a heading</h9>"); !strings.Contains(text, "not a heading") {
		t.Errorf("a heading deeper than there are headings came back as %q", text)
	}
}

func TestASearchServerThatAnswersNonsenseSaysSo(t *testing.T) {
	nonsense := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, "this is not JSON at all")
	}))
	t.Cleanup(nonsense.Close)

	tool := web.New(web.Settings{SearchServerAddress: nonsense.URL + "/search?safe=1"})
	_, err := run(t, tool, map[string]any{"action": "search", "query": "anything"})
	if err == nil {
		t.Fatalf("a search server that answered nonsense was treated as a search")
	}
	if !strings.Contains(err.Error(), "JSON") {
		t.Errorf("the failure reads %q and does not say what was wrong with the answer", err)
	}
}

func TestAResultsPageWithNoResultOnItSaysNoResultCouldBeRead(t *testing.T) {
	bare := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(writer, "<html><body><p>no results today</p></body></html>")
	}))
	t.Cleanup(bare.Close)

	tool := web.New(web.Settings{ResultsPageAddress: bare.URL + "/html/"})
	output, err := run(t, tool, map[string]any{"action": "search", "query": "anything"})
	if err != nil {
		t.Fatalf("searching a page with no rows on it failed: %v", err)
	}
	if !strings.Contains(output.Text, "no result could be read from it, so try other words") {
		t.Errorf("a search that found nothing said %q, and the model cannot tell the page apart from an empty web", output.Text)
	}
}

func TestWithNoResultsPageConfiguredTheShippedOneIsUsed(t *testing.T) {
	// No test in this package reaches the real internet, so what is checked here
	// is the address itself rather than a search through it.
	if !strings.Contains(web.DefaultResultsPage, "duckduckgo") {
		t.Errorf("the shipped results page is %q, and search must work with no key and no server", web.DefaultResultsPage)
	}
	if !strings.HasPrefix(web.DefaultResultsPage, "https://") {
		t.Errorf("the shipped results page is %q, and it must be fetched over a secure connection", web.DefaultResultsPage)
	}
}

func TestAPageThatIsPlainTextComesBackAsItself(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(writer, "just a line of plain text")
	}))
	t.Cleanup(plain.Close)

	tool := web.New(web.Settings{AllowedHosts: []string{hostOf(t, plain.URL)}})
	output, err := run(t, tool, map[string]any{"action": "fetch", "url": plain.URL + "/notes.txt"})
	if err != nil {
		t.Fatalf("fetching a page of plain text failed: %v", err)
	}
	if !strings.Contains(output.Text, "just a line of plain text") {
		t.Errorf("a page of plain text came back as %q", output.Text)
	}
}

func TestARedirectWithNowhereToGoIsReadAsThePageItself(t *testing.T) {
	pointless := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusFound)
		fmt.Fprint(writer, "<html><body><p>nowhere to go</p></body></html>")
	}))
	t.Cleanup(pointless.Close)

	tool := web.New(web.Settings{AllowedHosts: []string{hostOf(t, pointless.URL)}})
	_, err := run(t, tool, map[string]any{"action": "fetch", "url": pointless.URL + "/x"})
	if err == nil {
		t.Fatalf("a redirect with nowhere to go was read as a page")
	}
	if !strings.Contains(err.Error(), "302") {
		t.Errorf("the failure reads %q and does not say what the server answered", err)
	}
}

func TestAnAddressThatCannotBeReachedSaysSo(t *testing.T) {
	shut := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	address := shut.URL
	shut.Close()

	tool := web.New(web.Settings{AllowedHosts: []string{hostOf(t, address)}})
	_, err := run(t, tool, map[string]any{"action": "fetch", "url": address + "/x"})
	if err == nil {
		t.Fatalf("an address nothing is listening on was fetched")
	}
	if !strings.Contains(err.Error(), "cannot reach") {
		t.Errorf("the failure reads %q and does not say the address could not be reached", err)
	}
}

func TestTheResultsPageRedirectIsUnwrappedSoTheRealAddressComesBack(t *testing.T) {
	tool, server := newTool(t, "")

	output, err := run(t, tool, map[string]any{"action": "search", "query": "digibyte"})
	if err != nil {
		t.Fatalf("searching the results page failed: %v", err)
	}
	if strings.Contains(output.Text, "duckduckgo.com/l/") {
		t.Errorf("the results page's own redirect is still in the rows: %q", output.Text)
	}
	if !strings.Contains(output.Text, server.PageAddress("/notes")) {
		t.Errorf("the rows do not carry the address the result really points at: %q", output.Text)
	}
}

func TestAResultsPageWithLinksThatNeverCloseStillReads(t *testing.T) {
	broken := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(writer, `<html><body><div class="results"><a class="result__a" href="//x/?uddg=%2F%2Fnowhere`)
	}))
	t.Cleanup(broken.Close)

	tool := web.New(web.Settings{ResultsPageAddress: broken.URL + "/html/"})
	output, err := run(t, tool, map[string]any{"action": "search", "query": "anything"})
	if err != nil {
		t.Fatalf("searching a broken results page failed: %v", err)
	}
	if !strings.Contains(output.Text, "no result could be read from it, so try other words") {
		t.Errorf("a results page whose links never close read as %q", output.Text)
	}
}

// The fuzzer found this one: a result link with no address, no words and no end
// made a row of nothing, and the whole answer trimmed to an empty string, so the
// model was handed nothing at all rather than a sentence.
func TestAResultLinkWithNoAddressIsNoResultRatherThanAnEmptyAnswer(t *testing.T) {
	addressless := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(writer, "<a ClAss=result__a>")
	}))
	t.Cleanup(addressless.Close)

	tool := web.New(web.Settings{ResultsPageAddress: addressless.URL + "/html/"})
	output, err := run(t, tool, map[string]any{"action": "search", "query": "anything"})
	if err != nil {
		t.Fatalf("searching a page whose one result link has no address failed: %v", err)
	}
	if !strings.Contains(output.Text, "no result could be read from it, so try other words") {
		t.Errorf("a result link with no address read as %q, and the model cannot follow a result that leads nowhere", output.Text)
	}
}

// The fuzzer's second finding: an address of one space was enough to keep a row,
// so a page whose one result link led nowhere the fetch could open again read
// as an empty answer. Only a web address is one the model can follow.
func TestAResultLinkWhoseAddressIsNotAWebAddressIsNoResult(t *testing.T) {
	for _, page := range []string{
		`<a ClAss=result__ahref=" ">`,
		`<a class="result__a" href="   ">a title over a blank address</a>`,
		`<a class="result__a" href="/a/page/of/the/site/itself">a title over a path with no host</a>`,
	} {
		leadingNowhere := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(writer, page)
		}))
		t.Cleanup(leadingNowhere.Close)

		tool := web.New(web.Settings{ResultsPageAddress: leadingNowhere.URL + "/html/"})
		output, err := run(t, tool, map[string]any{"action": "search", "query": "anything"})
		if err != nil {
			t.Fatalf("searching the page %q failed: %v", page, err)
		}
		if !strings.Contains(output.Text, "no result could be read from it, so try other words") {
			t.Errorf("the page %q read as %q, and the model cannot follow a result whose address is not a web address", page, output.Text)
		}
	}
}
