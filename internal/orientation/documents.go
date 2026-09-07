package orientation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/markdown"
)

// The two documents a project keeps that the block names, and the bounds on
// how much of each it names. The model never reads either whole: the
// architecture page is read one section at a time with the read tool, and the
// map is for the search tool, with only its legend of what each root folder is
// for shown here.
const (
	// ArchitectureFile is the page that says how the project is put together.
	ArchitectureFile = "ARCHITECTURE.md"
	// MapFile is the generated map of where everything is.
	MapFile = "REPO_MAP.md"
	// MapLegendHeading is the section of the map that says what each root is for.
	MapLegendHeading = "Roots"
	// MaxSectionsNamed is how many of the architecture page's sections are named.
	MaxSectionsNamed = 20
	// MaxLegendLines is how many lines of the map's legend are shown.
	MaxLegendLines = 15
	// MaxDocumentBytes is the most of either file that is read to make its
	// line; a page past this is one nobody meant to be read in a round.
	MaxDocumentBytes = 4 << 20
)

// TheMapLegendHeading is the line over the map's legend.
const TheMapLegendHeading = MapFile + ", what each root folder is for:"

// documentLines names the two documents when the folder holds them: the
// architecture page as one line of its section names and how to read one, and
// the map as its legend. A folder with neither adds nothing.
func documentLines(folder string) []string {
	lines := []string{}
	if page, ok := readDocument(filepath.Join(folder, ArchitectureFile)); ok {
		lines = append(lines, architectureLine(page))
	}
	if held, ok := readDocument(filepath.Join(folder, MapFile)); ok {
		lines = append(lines, legendLines(held)...)
	}
	return lines
}

// readDocument reads one of the two files, bounded, and says whether it is
// there to be read.
func readDocument(path string) (string, bool) {
	held, err := os.ReadFile(path)
	if err != nil || len(held) == 0 {
		return "", false
	}
	if len(held) > MaxDocumentBytes {
		held = held[:MaxDocumentBytes]
	}
	return string(held), true
}

// architectureLine names the page's level-two sections, at most
// MaxSectionsNamed of them with the rest counted, and the read call that
// brings one back.
func architectureLine(page string) string {
	names := []string{}
	for _, section := range markdown.Sections(page) {
		if section.Level == 2 {
			names = append(names, section.Heading)
		}
	}
	if len(names) == 0 {
		return fmt.Sprintf("%s: no sections yet. Read it with `read %s`.", ArchitectureFile, ArchitectureFile)
	}
	more := ""
	if len(names) > MaxSectionsNamed {
		more = fmt.Sprintf(", and %d more", len(names)-MaxSectionsNamed)
		names = names[:MaxSectionsNamed]
	}
	return fmt.Sprintf("%s: sections %s%s. Read one with `read %s <heading>`.",
		ArchitectureFile, strings.Join(names, ", "), more, ArchitectureFile)
}

// legendLines is the map's Roots section, its list lines only, at most
// MaxLegendLines of them with the rest counted, under a heading. A map with no
// legend adds nothing, because the tree is not for the model to read.
func legendLines(held string) []string {
	section, found := markdown.Find(held, MapLegendHeading)
	if !found {
		return nil
	}
	items := []string{}
	for _, line := range strings.Split(section.Body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- ") {
			items = append(items, strings.TrimSpace(line))
		}
	}
	if len(items) == 0 {
		return nil
	}
	lines := []string{TheMapLegendHeading}
	shown := items
	if len(shown) > MaxLegendLines {
		shown = shown[:MaxLegendLines]
	}
	lines = append(lines, shown...)
	if len(items) > MaxLegendLines {
		lines = append(lines, fmt.Sprintf("... and %d more roots; read them with `read %s %s`", len(items)-MaxLegendLines, MapFile, MapLegendHeading))
	}
	return lines
}
