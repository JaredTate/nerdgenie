package loop

import (
	"regexp"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/markdown"
)

// A section the answer names is looked for on the page loosely before it is
// added: the same words in another case, or with a bracketed file after
// them, name the same part. Run 23's model had written "Rendering
// (js/render.js)" on the page itself, and the harness's answer "Rendering"
// would otherwise have become a second section. The page's own heading is
// the one kept, so the model's wording stands.

// aBracketedTail is the "(js/render.js)" a heading may end with.
var aBracketedTail = regexp.MustCompile(`\s*\([^)]*\)\s*$`)

// theHeadingOnThePage is the page's heading for the part the answer names:
// an exact match first, then the same words without their case or a
// bracketed tail, and the answer's own heading when the page has no such
// section yet.
func theHeadingOnThePage(page string, heading string) string {
	headings := markdown.Headings(page)
	for _, held := range headings {
		if held == heading {
			return held
		}
	}
	wanted := thePartNamed(heading)
	for _, held := range headings {
		if thePartNamed(held) == wanted {
			return held
		}
	}
	return heading
}

// thePartNamed is a heading with its case, its bracketed tail and its edges
// taken off, which is what two headings for one part have in common.
func thePartNamed(heading string) string {
	bare := aBracketedTail.ReplaceAllString(strings.TrimSpace(heading), "")
	return strings.ToLower(strings.Join(strings.Fields(strings.Trim(bare, " :.-")), " "))
}
