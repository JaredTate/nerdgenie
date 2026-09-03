package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// MaxSearchRows is how many results one search returns. A model that cannot
// choose from ten will not do better with a hundred.
const MaxSearchRows = 10

// searchAnswer is the shape a SearXNG instance answers in, of which three fields
// are worth reading.
type searchAnswer struct {
	// Results are the rows it found.
	Results []struct {
		// Title is the name of the page.
		Title string `json:"title"`
		// URL is its address.
		URL string `json:"url"`
		// Content is the line or two of it the server shows.
		Content string `json:"content"`
	} `json:"results"`
}

// search asks the configured search server when there is one, and reads the
// results page when there is not, and returns the rows and where they came from.
func (tool *Tool) search(ctx context.Context, query string) (string, string, error) {
	if tool.settings.SearchServerAddress != "" {
		return tool.throughTheServer(ctx, query)
	}
	return tool.throughTheResultsPage(ctx, query)
}

// throughTheServer asks a SearXNG instance for its answer as JSON.
func (tool *Tool) throughTheServer(ctx context.Context, query string) (string, string, error) {
	address := withQuery(tool.settings.SearchServerAddress, query, "format", "json")
	page, err := tool.fetchPage(ctx, address)
	if err != nil {
		return "", "", err
	}
	answer := searchAnswer{}
	if err := json.Unmarshal([]byte(page.body), &answer); err != nil {
		return "", "", fmt.Errorf("cannot read what the search server at %s answered as JSON: %w",
			tool.settings.SearchServerAddress, err)
	}

	rows := &strings.Builder{}
	for at, found := range answer.Results {
		if at >= MaxSearchRows {
			break
		}
		fmt.Fprintf(rows, "%s\n%s\n%s\n\n", found.Title, found.URL, oneLine(found.Content))
	}
	return strings.TrimSpace(rows.String()), address, nil
}

// throughTheResultsPage reads the results page as text, which is how search
// works with no key and no server of one's own.
func (tool *Tool) throughTheResultsPage(ctx context.Context, query string) (string, string, error) {
	address := withQuery(tool.resultsPage(), query)
	page, err := tool.fetchPage(ctx, address)
	if err != nil {
		return "", "", err
	}
	return rowsFromResultsPage(page.body), address, nil
}

// resultsPage is the results page to read, with the shipped one when the
// settings name none.
func (tool *Tool) resultsPage() string {
	if tool.settings.ResultsPageAddress != "" {
		return tool.settings.ResultsPageAddress
	}
	return DefaultResultsPage
}

// withQuery puts the search words, and any other pairs, on the end of an
// address.
func withQuery(address string, query string, pairs ...string) string {
	values := url.Values{}
	values.Set("q", query)
	for at := 0; at+1 < len(pairs); at += 2 {
		values.Set(pairs[at], pairs[at+1])
	}
	if strings.Contains(address, "?") {
		return address + "&" + values.Encode()
	}
	return address + "?" + values.Encode()
}

// hostAndPortOf is the host and port of an address, with the port its scheme is
// served on filled in when the address leaves the port out, and nothing at all
// when the address is not one. The allowed hosts hold a host and a port
// together, never a bare name, because a bare name on that list would be
// reachable on every port of whatever machine answers for it.
func hostAndPortOf(address string) string {
	parsed, err := url.Parse(strings.TrimSpace(address))
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	port := parsed.Port()
	if port == "" {
		port = defaultPortFor(parsed.Scheme)
	}
	if port == "" {
		return ""
	}
	return net.JoinHostPort(parsed.Hostname(), port)
}

// oneLine puts text on a single line, because a row of results is one line.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
