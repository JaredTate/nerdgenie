package web

import (
	"fmt"
	"net/url"
	"strings"
)

// The two marks a results page puts on the parts of one row. They are the class
// names the DuckDuckGo results page has used for years, and the fake server in
// internal/testkit writes the same ones.
const (
	// resultTitleClass marks the link that carries the title and the address.
	resultTitleClass = "result__a"
	// resultSnippetClass marks the line of text under the title.
	resultSnippetClass = "result__snippet"
)

// redirectParameter is the name the results page hides the real address behind
// when it wraps a link in a redirect of its own.
const redirectParameter = "uddg"

// row is one result read off a results page.
type row struct {
	title   string
	address string
	snippet string
}

// rowsFromResultsPage reads the rows off a results page: the title of each
// result, the address it really points at with the page's own redirect
// unwrapped, and the line of text under it.
func rowsFromResultsPage(page string) string {
	found := readRows(page)
	if len(found) == 0 {
		return "nothing was found"
	}
	written := &strings.Builder{}
	for at, one := range found {
		if at >= MaxSearchRows {
			break
		}
		fmt.Fprintf(written, "%s\n%s\n%s\n\n", one.title, one.address, one.snippet)
	}
	return strings.TrimSpace(written.String())
}

// readRows walks the links on a results page and builds one row per result.
func readRows(page string) []row {
	rows := []row{}
	at := 0
	for len(rows) <= MaxSearchRows*2 {
		opened := strings.Index(page[at:], "<a ")
		if opened < 0 {
			return rows
		}
		opened += at
		closed := strings.IndexByte(page[opened:], '>')
		if closed < 0 {
			return rows
		}
		tag := page[opened+1 : opened+closed]
		text, after := linkText(page, opened+closed+1)
		rows = addRow(rows, tag, text)
		at = after
	}
	return rows
}

// addRow puts one link into the rows, starting a new row for a title and filling
// in the line of text for a snippet.
func addRow(rows []row, tag string, text string) []row {
	class := attributeOf("<"+tag+">", 0, "class")
	switch {
	case strings.Contains(class, resultTitleClass):
		return append(rows, row{title: text, address: realAddress(attributeOf("<"+tag+">", 0, "href"))})
	case strings.Contains(class, resultSnippetClass) && len(rows) > 0:
		rows[len(rows)-1].snippet = text
	}
	return rows
}

// linkText reads the words inside a link and says where the text after it starts.
func linkText(page string, from int) (string, int) {
	end := strings.Index(page[from:], "</a>")
	if end < 0 {
		return "", len(page)
	}
	return strings.TrimSpace(unescape(HTMLToText(page[from : from+end]))), from + end + 4
}

// realAddress unwraps the redirect a results page wraps its links in, and fills
// in the scheme when the page left it off.
func realAddress(written string) string {
	address := unescape(written)
	if parsed, err := url.Parse(address); err == nil {
		if hidden := parsed.Query().Get(redirectParameter); hidden != "" {
			address = hidden
		}
	}
	if strings.HasPrefix(address, "//") {
		return "https:" + address
	}
	return address
}
