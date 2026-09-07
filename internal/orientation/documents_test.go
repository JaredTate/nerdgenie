package orientation_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/orientation"
)

const theArchitecturePage = `# Tater Tots Tetris Architecture

## Engine

The board and the pieces.

## Hazards

The states and the config.

### Dragon

A level-three heading is not a section the line names.

## Effects

## Shell

## Tests
`

const theMap = `# Repository Map

## Roots

- ` + "`./`" + ` - The living documents and the game's package file.
- ` + "`src/`" + ` - The engine, the hazards and the effects.
- ` + "`test/`" + ` - One test file per part.

## Tree

` + "```text" + `
.
src/engine.js
` + "```" + `
`

func writeFile(t *testing.T, folder string, name string, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(folder, name), []byte(text), 0o644); err != nil {
		t.Fatalf("cannot write %s: %v", name, err)
	}
}

// TestTheBlockNamesTheArchitecturePagesSectionsAndHowToReadOne: a folder with
// ARCHITECTURE.md gets one line naming its level-two sections and the read
// call that brings one back, so the model reads two hundred words of design
// instead of the files.
func TestTheBlockNamesTheArchitecturePagesSectionsAndHowToReadOne(t *testing.T) {
	folder := aFolder(t, "src/")
	writeFile(t, folder, "ARCHITECTURE.md", theArchitecturePage)

	block := orientation.Block(orientation.Facts{Folder: folder, ProcFiles: []string{aTable(t)}})

	want := "ARCHITECTURE.md: sections Engine, Hazards, Effects, Shell, Tests. Read one with `read ARCHITECTURE.md <heading>`."
	if !strings.Contains(block, want) {
		t.Errorf("the block does not carry %q:\n%s", want, block)
	}
	if strings.Contains(block, "Dragon") {
		t.Errorf("a level-three heading was named as a section:\n%s", block)
	}
}

// TestTheBlockCarriesTheMapsRootsLegend: a folder with REPO_MAP.md and a
// Roots section gets the legend's lines, so the lay of the land costs no
// rounds of listing.
func TestTheBlockCarriesTheMapsRootsLegend(t *testing.T) {
	folder := aFolder(t, "src/")
	writeFile(t, folder, "REPO_MAP.md", theMap)

	block := orientation.Block(orientation.Facts{Folder: folder, ProcFiles: []string{aTable(t)}})

	for _, want := range []string{
		"REPO_MAP.md, what each root folder is for:",
		"- `src/` - The engine, the hazards and the effects.",
		"- `test/` - One test file per part.",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("the block does not carry %q:\n%s", want, block)
		}
	}
	if strings.Contains(block, "src/engine.js") {
		t.Errorf("the map's tree was put in the block, and only the legend belongs there:\n%s", block)
	}
}

// TestTheDocumentLinesAreCapped: many sections and a long legend are cut, with
// the count of what was left out, so the two files can never crowd the block.
func TestTheDocumentLinesAreCapped(t *testing.T) {
	folder := aFolder(t, "src/")
	var page, legend strings.Builder
	page.WriteString("# Big\n")
	legend.WriteString("# Map\n\n## Roots\n\n")
	for at := 1; at <= 30; at++ {
		fmt.Fprintf(&page, "## Section %d\n\ntext\n\n", at)
		fmt.Fprintf(&legend, "- `folder%d/` - what it is for\n", at)
	}
	writeFile(t, folder, "ARCHITECTURE.md", page.String())
	writeFile(t, folder, "REPO_MAP.md", legend.String())

	block := orientation.Block(orientation.Facts{Folder: folder, ProcFiles: []string{aTable(t)}})

	if !strings.Contains(block, "Section 20, and 10 more.") || strings.Contains(block, "Section 21,") {
		t.Errorf("the sections are not cut at twenty with the rest counted:\n%s", block)
	}
	if !strings.Contains(block, "- `folder15/` - what it is for") || strings.Contains(block, "folder16/") {
		t.Errorf("the legend is not cut at fifteen lines:\n%s", block)
	}
	if !strings.Contains(block, "... and 15 more roots") {
		t.Errorf("the cut legend does not say how many roots were left out:\n%s", block)
	}
}

// TestAFolderWithoutTheDocumentsHasNoDocumentLines: nothing is added for a
// folder that carries neither file, so the block is what it was.
func TestAFolderWithoutTheDocumentsHasNoDocumentLines(t *testing.T) {
	folder := aFolder(t, "src/", "README.md")
	block := orientation.Block(orientation.Facts{Folder: folder, ProcFiles: []string{aTable(t)}})
	for _, absent := range []string{"ARCHITECTURE.md", "REPO_MAP.md"} {
		if strings.Contains(block, absent) {
			t.Errorf("the block names %s, which the folder does not have:\n%s", absent, block)
		}
	}
}
