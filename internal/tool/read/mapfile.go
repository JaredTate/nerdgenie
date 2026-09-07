package read

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/codemap"
)

// TheMapFile is the generated map's name. On run eighteen the model read it
// whole at the start of every task, four thousand tokens to find one file,
// so a read of it with no section answers with its contents: the roots and
// one line per file, and how to read one file's entry.
const TheMapFile = "REPO_MAP.md"

// readTheMapsContents answers a read of a generated map with its contents,
// and says whether the path was such a map at all; a hand-written map, or a
// map too large to read by section, is read as any file is.
func readTheMapsContents(path string) (string, bool, error) {
	if filepath.Base(path) != TheMapFile {
		return "", false, nil
	}
	about, err := os.Stat(path)
	if err != nil || about.Size() > MaxSectionFileBytes {
		return "", false, nil
	}
	held, err := os.ReadFile(path)
	if err != nil {
		return "", true, fmt.Errorf("cannot read %s, so check that it is a file the agent may read: %w", path, err)
	}
	text := string(held)
	if !codemap.IsGenerated(text) {
		return "", false, nil
	}
	return withinTheCaps(codemap.Contents(text, MaxLines-mapListingHeadroom)), true, nil
}

// mapListingHeadroom is the lines of a map's contents kept for its title,
// its roots and its closing line, so the file lines fit under MaxLines.
const mapListingHeadroom = 40

// withinTheCaps cuts a listing to the line and byte caps of one read, and
// says so on its last line when it cut.
func withinTheCaps(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	var out strings.Builder
	for at, line := range lines {
		if at >= MaxLines || out.Len()+len(line)+1 > MaxBytes {
			out.WriteString("(cut at the read's cap; read one file's entry with `read " + TheMapFile + " <path>`)\n")
			break
		}
		out.WriteString(line + "\n")
	}
	return out.String()
}
