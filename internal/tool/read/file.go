package read

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// readFile returns the text of one file with a line number in front of every
// line, from the offset the model asked for and no more lines than it asked for.
func readFile(path string, asked input) (string, error) {
	held, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s, so check that it is a file the agent may read: %w", path, err)
	}
	if notText(held) {
		return "", fmt.Errorf("%s is not a text file, so read it with a tool that understands its kind rather than this one", path)
	}

	lines := strings.Split(strings.TrimSuffix(string(held), "\n"), "\n")
	from, count, wanted := window(asked)
	if from > len(lines) {
		return "", fmt.Errorf("%s has %d lines, and the read starts at line %d, so start at a line the file has", path, len(lines), from)
	}
	return numbered(lines, from, count, wanted), nil
}

// window is the first line to read, how many lines to read, and whether the
// model set the count itself, with the bounds applied to whatever it asked for.
func window(asked input) (int, int, bool) {
	from := asked.Offset
	if from < 1 {
		from = 1
	}
	count := asked.Limit
	wanted := count >= 1 && count <= MaxLines
	if !wanted {
		count = MaxLines
	}
	return from, count, wanted
}

// numbered writes the lines out with their numbers in front, stopping at the
// count asked for or at the byte cap. It says how to read on when the read was
// cut short by a bound rather than by the number of lines the model asked for.
func numbered(lines []string, from int, count int, wanted bool) string {
	written := &strings.Builder{}
	shown := 0
	for at := from - 1; at < len(lines) && shown < count; at++ {
		line := fmt.Sprintf("%d: %s\n", at+1, cutRunes(lines[at], MaxLineRunes))
		if written.Len()+len(line) > MaxBytes {
			fmt.Fprintf(written, "... this read stopped at line %d because it reached %d bytes; read on with an offset of %d\n",
				at, MaxBytes, at+1)
			return written.String()
		}
		written.WriteString(line)
		shown++
	}
	if last := from + shown - 1; !wanted && last < len(lines) {
		fmt.Fprintf(written, "... %d more lines; read on with an offset of %d\n", len(lines)-last, last+1)
	}
	return written.String()
}

// listFolder returns the entries of one folder, one to a line, in order, with a
// slash after every folder among them.
func listFolder(path string) (string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("cannot list the folder %s, so check that the agent may read it: %w", path, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += string(filepath.Separator)
		}
		names = append(names, name)
	}
	sort.Strings(names)

	written := &strings.Builder{}
	for at, name := range names {
		if at >= MaxEntries {
			fmt.Fprintf(written, "... %d more entries; this listing stops at %d\n", len(names)-MaxEntries, MaxEntries)
			break
		}
		written.WriteString(name + "\n")
	}
	return written.String(), nil
}

// notText says whether a file holds something other than text, which is a byte
// of zero or a run of bytes that is not valid text in the first part of it.
func notText(held []byte) bool {
	sample := held
	if len(sample) > binarySampleBytes {
		sample = sample[:binarySampleBytes]
	}
	for _, one := range sample {
		if one == 0 {
			return true
		}
	}
	return len(sample) > 0 && !utf8.Valid(sample)
}

// cutRunes cuts one line to a number of characters, so that a file written as
// one enormous line cannot fill the model's window on its own.
func cutRunes(line string, length int) string {
	letters := []rune(line)
	if len(letters) <= length {
		return line
	}
	return string(letters[:length]) + fmt.Sprintf(" ... (%d more characters on this line)", len(letters)-length)
}
