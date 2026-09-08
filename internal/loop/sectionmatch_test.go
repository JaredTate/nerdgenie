package loop

import "testing"

// Run 23, task 3: the model had written "## Rendering (js/render.js)" on the
// page itself, the harness's answer was headed "Rendering", the exact match
// found no such section, and a second Rendering section was added. A
// heading that names the same part with or without a bracketed file, or in
// another case, is the same section, and the page's own heading is kept.
func TestAHeadingThatNamesAnExistingSectionLooselyWritesThatSection(t *testing.T) {
	page := "# Architecture\n\n## Scaffold\n\nold\n\n## Rendering (js/render.js)\n\nold\n\n## Game logic\n\nold\n"
	for _, shape := range []struct{ asked, want string }{
		{"Rendering", "Rendering (js/render.js)"},
		{"rendering (js/render.js)", "Rendering (js/render.js)"},
		{"Game Logic (game.js)", "Game logic"},
		{"Scaffold", "Scaffold"},
		{"Theme", "Theme"},
		{"Board", "Board"},
	} {
		if got := theHeadingOnThePage(page, shape.asked); got != shape.want {
			t.Errorf("the answer's heading %q writes the section %q, want %q", shape.asked, got, shape.want)
		}
	}
}
