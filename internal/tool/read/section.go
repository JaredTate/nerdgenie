package read

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/markdown"
)

// MaxSectionFileBytes is the most of a Markdown file that is read to find a
// section in it. An architecture page of a large repository is under half a
// megabyte; a file past this is not a page anyone reads by section.
const MaxSectionFileBytes = 8 << 20

// readSection returns one section of a Markdown file, the lines from its
// heading to the next heading of any level, with the file's own line numbers in
// front, bounded exactly as a file read is. A heading the file does not have is
// refused with the headings it does have, so the next call is right, and a
// section of a file that is not Markdown is refused with the way out.
func readSection(path string, asked input) (string, error) {
	if !isMarkdown(path) {
		return "", fmt.Errorf("%s is not a Markdown file, so a section cannot be picked out of it; read the file without a section", path)
	}
	held, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s, so check that it is a file the agent may read: %w", path, err)
	}
	if len(held) > MaxSectionFileBytes {
		return "", fmt.Errorf("%s is over %d bytes, too large to read by section; read it with an offset and a limit instead", path, MaxSectionFileBytes)
	}
	text := string(held)
	section, found := markdown.Find(text, asked.Section)
	if !found {
		headings := markdown.Headings(text)
		if len(headings) == 0 {
			return "", fmt.Errorf("%s has no headings, so it has no section %q; read the file without a section", path, asked.Section)
		}
		return "", fmt.Errorf("%s has no section %q; its headings are: %s", path, asked.Section, strings.Join(headings, ", "))
	}
	from, count := sectionWindow(text, section)
	if asked.Limit > 0 && asked.Limit < count {
		count = asked.Limit
	}
	wanted := asked.Limit > 0
	if count > MaxLines {
		count = MaxLines
		wanted = false
	}
	return numbered(bufio.NewReader(strings.NewReader(text)), path, from, count, wanted)
}

// sectionWindow is the line the section's heading is on, counting from one,
// and how many lines the section runs to, heading included.
func sectionWindow(text string, section markdown.Section) (int, int) {
	headingLine := strings.Repeat("#", section.Level) + " " + section.Heading
	from := 1
	for at, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == headingLine {
			from = at + 1
			break
		}
	}
	count := 1 + strings.Count(strings.TrimRight(section.Body, "\n"), "\n")
	if strings.TrimRight(section.Body, "\n") != "" {
		count++
	}
	return from, count
}

// isMarkdown says whether a file is one a section can be picked out of.
func isMarkdown(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".mdown":
		return true
	}
	return false
}
