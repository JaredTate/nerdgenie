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
		lines = append(lines, mapLines(held)...)
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

// mapLines are what the orientation says of the map: one line counting its
// source files and saying how a file's functions are read, or, for a map
// somebody wrote without a legend, its sections; then the Roots legend's
// list lines when it has one, at most MaxLegendLines of them.
func mapLines(held string) []string {
	lines := []string{mapLine(held)}
	section, found := markdown.Find(held, MapLegendHeading)
	if !found {
		return lines
	}
	items := []string{}
	for _, line := range strings.Split(section.Body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- ") {
			items = append(items, strings.TrimSpace(line))
		}
	}
	if len(items) == 0 {
		return lines
	}
	lines = append(lines, TheMapLegendHeading)
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

// mapLine is the one line that tells the model what the map is for: how
// many files it holds an entry for, and that a file's functions are read by
// the file's path. A map with no file entries is named by its sections.
func mapLine(held string) string {
	entries := 0
	var sections []string
	for _, section := range markdown.Sections(held) {
		switch section.Level {
		case 2:
			if section.Heading != MapLegendHeading {
				sections = append(sections, section.Heading)
			}
		case 3:
			entries++
		}
	}
	_, hasLegend := markdown.Find(held, MapLegendHeading)
	if len(sections) > MaxSectionsNamed {
		sections = sections[:MaxSectionsNamed]
	}
	switch {
	case hasLegend && entries > 0:
		return fmt.Sprintf("%s: %d source files; read a file's functions with `read %s <path>`.", MapFile, entries, MapFile)
	case len(sections) > 0 && entries > 0:
		return fmt.Sprintf("%s: sections %s; %d file entries; read a file's functions with `read %s <path>`.", MapFile, strings.Join(sections, ", "), entries, MapFile)
	case len(sections) > 0:
		return fmt.Sprintf("%s: sections %s; read one with `read %s <heading>`.", MapFile, strings.Join(sections, ", "), MapFile)
	}
	return fmt.Sprintf("%s: where everything is; read it with `read %s`.", MapFile, MapFile)
}
