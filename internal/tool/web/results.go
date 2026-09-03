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

// The two lines a results page with no result on it comes back as. A page that
// arrived and held nothing the reader could use is not the same thing as a web
// with nothing on it, and the model has to be able to tell them apart: the first
// is worth trying other words for, and the second never will be.
const (
	// noResultRead is what a page the reader found no result on comes back as.
	noResultRead = "the results page came back but no result could be read from it, so try other words"
	// humanCheckAsked is what the page a search site answers a plain program
	// with comes back as: a page asking whoever searched to prove they are a
	// person, which no wording of the query will get past.
	humanCheckAsked = "the search site answered with a page asking for a human check rather than with results, so search " +
		"another way: set search_server_address in config.toml to a search server of your own, or open the site with the browser tools"
)

// marksOfAHumanCheck are the words the page asking for a human check carries,
// in small letters. The first is the name that page marks its own parts with and
// the second is the sentence it shows the person, so that a change to either one
// still leaves the page recognised.
var marksOfAHumanCheck = []string{"anomaly-modal", "confirm this search was made by a human"}

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
	found := withAnAddress(readRows(page))
	if len(found) == 0 {
		return whyNoResultWasRead(page)
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

// withAnAddress keeps the rows the model could follow: those whose address is
// a web address, the only kind the fetch can open. A result link with no
// address, or with a space or a bare path where one should be, is nothing the
// model can open, and a page made only of those has no result on it that the
// reader could find, which is what the model has to be told. The fuzzer found
// both halves: such a page otherwise read as an empty answer, because a row of
// nothing is nothing once the ends are trimmed, and an address of one space was
// then enough to keep the row.
func withAnAddress(rows []row) []row {
	kept := rows[:0]
	for _, one := range rows {
		if isWebAddress(one.address) {
			kept = append(kept, one)
		}
	}
	return kept
}

// isWebAddress says whether an address is one the fetch could open, which is
// one that names its scheme as http or https, however the letters are cased.
func isWebAddress(address string) bool {
	lowered := asciiLower(address)
	return strings.HasPrefix(lowered, "http://") || strings.HasPrefix(lowered, "https://")
}

// whyNoResultWasRead says why a page came back with no result on it, telling the
// page that asks for a human check apart from a page that simply holds nothing.
func whyNoResultWasRead(page string) string {
	lowered := asciiLower(page)
	for _, mark := range marksOfAHumanCheck {
		if strings.Contains(lowered, mark) {
			return humanCheckAsked
		}
	}
	return noResultRead
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

// linkText reads the words inside a link, on one line because a row is three
// lines and a title that broke across several would break the shape the model
// reads rows by, and says where the text after the link starts.
func linkText(page string, from int) (string, int) {
	end := strings.Index(page[from:], "</a>")
	if end < 0 {
		return "", len(page)
	}
	return oneLine(unescape(HTMLToText(page[from : from+end]))), from + end + 4
}

// realAddress unwraps the redirect a results page wraps its links in, and fills
// in the scheme when the page left it off.
func realAddress(written string) string {
	address := strings.TrimSpace(unescape(written))
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
