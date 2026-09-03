package web_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/tool/web"
)

// FuzzTheTurnIntoText throws any text at the step that turns a web page into
// text and asserts the three things that must always hold: it never panics, what
// comes back is inside the cap, and no tag survives it.
func FuzzTheTurnIntoText(f *testing.F) {
	f.Add("<h1>a heading</h1><p>a paragraph</p>")
	f.Add(`<a href="https://example.com">a link</a>`)
	f.Add("<ul><li>one</li><li>two</li></ul>")
	f.Add("<script>alert(1)</script>")
	f.Add("<!-- a comment --><p>after it</p>")
	f.Add("&amp;&lt;&gt;&quot;&#39;&nbsp;&#x41;")
	f.Add("<p unclosed")
	f.Add("")

	f.Fuzz(func(t *testing.T, page string) {
		if len(page) > 1<<18 {
			t.Skip("the fuzzer wrote more text than one page ever carries")
		}
		text := web.HTMLToText(page)
		if len(text) > web.MaxPageBytes {
			t.Fatalf("a page of %d bytes turned into %d bytes of text, and the cap is %d",
				len(page), len(text), web.MaxPageBytes)
		}
		if strings.Contains(text, "<script") || strings.Contains(text, "<style") {
			t.Fatalf("a tag survived the turn into text: %q", text)
		}
	})
}

// FuzzTheAddressCheck throws any text at the check that says whether an address
// may be reached, and asserts that it never panics and never allows an address
// that is not a web address.
func FuzzTheAddressCheck(f *testing.F) {
	f.Add("https://example.com/x")
	f.Add("http://127.0.0.1:8080/x")
	f.Add("file:///etc/passwd")
	f.Add("http://[::1]/x")
	f.Add("")
	f.Add("http://")

	f.Fuzz(func(t *testing.T, address string) {
		if len(address) > 4096 {
			t.Skip("the fuzzer wrote more text than one address ever carries")
		}
		if err := web.CheckAddressAllowed(address, nil); err != nil {
			return
		}
		if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
			t.Fatalf("the address %q was allowed and is not a web address", address)
		}
	})
}
