package codemap

import (
	"fmt"
	"strings"
)

// A map of a real project runs to thousands of lines, and a model that reads
// it whole to find one file spends a round on it. So a generated map read
// without a section answers with its contents, the roots and one line per
// file, which is enough to pick the file whose entry to read next.

// IsGenerated says whether a map's text is one the harness wrote, by the mark
// near its top; a map without the mark is a person's own.
func IsGenerated(text string) bool {
	head := text
	if len(head) > 1024 {
		head = head[:1024]
	}
	return strings.Contains(head, GeneratedMark)
}

// Contents is the listing of a printed map: the roots as they are, then one
// line per file with its first line, at most mostFiles of them, and how to
// read one file's entry.
func Contents(text string, mostFiles int) string {
	var out strings.Builder
	out.WriteString("# Repository Map: contents\n\n")
	out.WriteString(rootsOf(text))
	out.WriteString("## Files\n\n")
	files := fileLinesOf(text)
	left := 0
	if mostFiles >= 0 && len(files) > mostFiles {
		left = len(files) - mostFiles
		files = files[:mostFiles]
	}
	for _, line := range files {
		out.WriteString(line + "\n")
	}
	if left > 0 {
		fmt.Fprintf(&out, "- and %d more files\n", left)
	}
	out.WriteString("\nRead a file's functions with `read REPO_MAP.md <path>`.\n")
	return out.String()
}

// rootsOf is the Roots section of the printed map, heading and lines, or
// nothing when the map has none.
func rootsOf(text string) string {
	var out strings.Builder
	inRoots := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "## ") {
			inRoots = strings.TrimSpace(line) == "## Roots"
			if inRoots {
				out.WriteString("## Roots\n\n")
			}
			continue
		}
		if inRoots && strings.HasPrefix(line, "- ") {
			out.WriteString(line + "\n")
		}
	}
	if out.Len() > 0 {
		out.WriteString("\n")
	}
	return out.String()
}

// fileLinesOf is one line per file of the printed map: a heading's path with
// the line under it, which is the file's first line, its test count, or the
// note that nothing was read; and each of the other files as its path.
func fileLinesOf(text string) []string {
	var files []string
	heading := ""
	said := false
	inOtherFiles, inFence := false, false
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimRight(raw, "\r")
		switch {
		case strings.HasPrefix(line, "### "):
			heading = strings.TrimSpace(line[4:])
			said = false
			files = append(files, "- "+heading)
		case strings.HasPrefix(line, "## "):
			heading = ""
			inOtherFiles = strings.TrimSpace(line) == "## Other files"
		case inOtherFiles && strings.HasPrefix(line, "```"):
			inFence = !inFence
		case inOtherFiles && inFence && strings.TrimSpace(line) != "":
			files = append(files, "- "+strings.TrimSpace(line))
		case heading != "" && isATestCount(line):
			// A test file's count rides after its first line, in brackets.
			files[len(files)-1] += " (" + strings.TrimPrefix(strings.TrimSpace(line), "- ") + ")"
			said = true
		case heading != "" && !said && strings.TrimSpace(line) != "":
			// The line under a heading is the file's first line or the note
			// that nothing was read; a name line is not what the file is
			// about, and is left to the entry.
			said = true
			if under := strings.TrimSpace(line); !strings.HasPrefix(under, "- `") && !strings.HasPrefix(under, "- and ") {
				files[len(files)-1] += " — " + strings.TrimPrefix(under, "- ")
			}
		}
	}
	return files
}

// isATestCount says whether the line is a test file's count, "- 12 tests".
func isATestCount(line string) bool {
	words := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "- "))
	if len(words) != 2 || (words[1] != "tests" && words[1] != "test") {
		return false
	}
	return strings.Trim(words[0], "0123456789") == ""
}
