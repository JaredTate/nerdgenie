package loop

import (
	"strings"
	"testing"
)

// FuzzTheScriptSourcesIn holds the page scan to never panicking and never
// listing more scripts than the cap, on any page.
func FuzzTheScriptSourcesIn(f *testing.F) {
	f.Add(theGamePageSeed)
	f.Add("<script src=main.js>")
	f.Add("<SCRIPT\nSRC = 'a b'>")
	f.Fuzz(func(t *testing.T, page string) {
		if sources := theScriptSourcesIn(page); len(sources) > MaxScriptsRead {
			t.Fatalf("%d scripts were listed, more than the cap", len(sources))
		}
	})
}

// FuzzLoopLinesIn holds the loop scan to never panicking and to naming the
// script on every line, on any script.
func FuzzLoopLinesIn(f *testing.F) {
	f.Add("while (x) {}\nfor (;;) {}\n")
	f.Add("forward(x)\nwhile(y)")
	f.Fuzz(func(t *testing.T, script string) {
		whiles, fors := loopLinesIn(namedText{name: "s.js", text: script})
		for _, line := range append(whiles, fors...) {
			if !strings.HasPrefix(line, "s.js:") {
				t.Fatalf("the loop line %q does not name the script", line)
			}
		}
	})
}

// theGamePageSeed is a page with a script of its own and an inline loop.
const theGamePageSeed = "<!doctype html>\n<script src=\"js/main.js\"></script>\n<script>for (const t of tots) {}</script>\n"
