package loop

import "testing"

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

// FuzzLoopLinesIn holds the loop scan to its cap on any script.
func FuzzLoopLinesIn(f *testing.F) {
	f.Add("while (x) {}\nfor (;;) {}\n", 1)
	f.Add("forward(x)\nwhile(y)", 5)
	f.Fuzz(func(t *testing.T, script string, capLeft int) {
		if lines := loopLinesIn(namedText{name: "s.js", text: script}, capLeft); capLeft >= 0 && len(lines) > capLeft {
			t.Fatalf("%d loop lines were listed under a cap of %d", len(lines), capLeft)
		}
	})
}

// theGamePageSeed is a page with a script of its own and an inline loop.
const theGamePageSeed = "<!doctype html>\n<script src=\"js/main.js\"></script>\n<script>for (const t of tots) {}</script>\n"
