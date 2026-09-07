package markdown_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/markdown"
)

const thePage = `# Tater Tots Tetris

The game in one line.

## Engine

The board and the pieces live in src/engine.js.

### Pieces

Seven of them.

## Hazards

The states are NORMAL, DRAGON_WARNING and so on.

` + "```" + `
# not a heading, a comment in a code fence
## nor this
` + "```" + `

## Effects
`

func TestSectionsAreCutAtEveryHeadingWithTheirLevel(t *testing.T) {
	sections := markdown.Sections(thePage)
	var got []string
	for _, section := range sections {
		got = append(got, strings.Repeat("#", section.Level)+" "+section.Heading)
	}
	want := []string{"# Tater Tots Tetris", "## Engine", "### Pieces", "## Hazards", "## Effects"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the sections are %q, want %q", got, want)
	}
}

func TestASectionsBodyRunsToTheNextHeadingOfAnyLevel(t *testing.T) {
	sections := markdown.Sections(thePage)
	engine := sections[1]
	if !strings.Contains(engine.Body, "src/engine.js") || strings.Contains(engine.Body, "Seven of them") {
		t.Errorf("the engine section's body is %q, want the board line and not the pieces", engine.Body)
	}
	hazards := sections[3]
	if !strings.Contains(hazards.Body, "not a heading, a comment in a code fence") {
		t.Errorf("the hazards body %q lost the fenced lines that look like headings", hazards.Body)
	}
}

func TestALastSectionMayBeEmpty(t *testing.T) {
	sections := markdown.Sections(thePage)
	last := sections[len(sections)-1]
	if last.Heading != "Effects" || strings.TrimSpace(last.Body) != "" {
		t.Errorf("the last section is %+v, want Effects with nothing under it", last)
	}
}

func TestSectionFindsAHeadingWithoutRegardToCaseOrSpaces(t *testing.T) {
	for _, heading := range []string{"Hazards", "hazards", "  HAZARDS ", "## Hazards"} {
		section, found := markdown.Find(thePage, heading)
		if !found || section.Heading != "Hazards" {
			t.Errorf("Find(%q) found=%v heading=%q, want the Hazards section", heading, found, section.Heading)
		}
	}
}

func TestSectionSaysWhenThereIsNoSuchHeading(t *testing.T) {
	if _, found := markdown.Find(thePage, "Rendering"); found {
		t.Errorf("a heading that is not in the page was found")
	}
}

func TestAnEmptyTextHasNoSections(t *testing.T) {
	if sections := markdown.Sections(""); len(sections) != 0 {
		t.Errorf("an empty text has %d sections, want none", len(sections))
	}
	if sections := markdown.Sections("just a line\nand another\n"); len(sections) != 0 {
		t.Errorf("a text with no heading has %d sections, want none", len(sections))
	}
}

func TestHeadingsListsTheHeadingsInOrder(t *testing.T) {
	got := markdown.Headings(thePage)
	want := "Tater Tots Tetris, Engine, Pieces, Hazards, Effects"
	if strings.Join(got, ", ") != want {
		t.Errorf("the headings are %q, want %q", got, want)
	}
}

func TestReplaceSectionSwapsOneBodyAndKeepsTheRest(t *testing.T) {
	replaced, found := markdown.ReplaceSection(thePage, "Engine", "A new engine paragraph.\n")
	if !found {
		t.Fatalf("the engine section was not found to replace")
	}
	if !strings.Contains(replaced, "## Engine\n\nA new engine paragraph.\n") {
		t.Errorf("the engine section was not replaced: %q", replaced)
	}
	if strings.Contains(replaced, "src/engine.js") {
		t.Errorf("the old engine body is still there")
	}
	for _, kept := range []string{"### Pieces", "Seven of them", "## Hazards", "## Effects", "The game in one line."} {
		if !strings.Contains(replaced, kept) {
			t.Errorf("replacing one section lost %q", kept)
		}
	}
}

func TestReplaceSectionAppendsAHeadingThatIsNotThere(t *testing.T) {
	replaced, found := markdown.ReplaceSection(thePage, "Rendering", "Canvas.\n")
	if found {
		t.Errorf("a heading that was not there was reported found")
	}
	if !strings.HasSuffix(replaced, "## Rendering\n\nCanvas.\n") {
		t.Errorf("the new section was not appended at the end: %q", replaced[len(replaced)-60:])
	}
}

func FuzzSections(f *testing.F) {
	f.Add(thePage)
	f.Add("# a\n## b\n### c\n")
	f.Add("```\n# fenced\n```\n# real\n")
	f.Add("no headings at all")
	f.Fuzz(func(t *testing.T, text string) {
		sections := markdown.Sections(text)
		for _, section := range sections {
			if section.Level < 1 || section.Level > 6 {
				t.Errorf("a section has level %d", section.Level)
			}
			if !strings.Contains(text, section.Heading) {
				t.Errorf("the heading %q is not in the text", section.Heading)
			}
			// Two headings may differ only in case or in leading hash marks,
			// so Find may answer the first of them; the words must still match
			// the way Find compares them.
			if got, found := markdown.Find(text, section.Heading); !found || sameHeading(got.Heading, section.Heading) == false {
				t.Errorf("Find cannot find %q that Sections reported", section.Heading)
			}
		}
	})
}

// sameHeading compares two headings the way Find does: hash marks and spaces
// off both ends, and case set aside.
func sameHeading(left string, right string) bool {
	clean := func(heading string) string {
		return strings.ToLower(strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(heading), "#")))
	}
	return clean(left) == clean(right)
}
