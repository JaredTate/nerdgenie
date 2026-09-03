package read

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// The bounds on how a file is walked through, as against how much of it is
// shown, which the caps in read.go hold.
const (
	// ReadBufferBytes is how much of the file is held in memory at once while it
	// is walked through. It is bigger than the sample the text check looks at,
	// because that check reads its sample out of this buffer.
	ReadBufferBytes = 64 << 10
	// MaxLineBytes is how much of one line is kept while it is read. A file
	// written as one enormous line is walked past rather than held, and the line
	// says how many bytes were left unread.
	MaxLineBytes = 64 << 10
)

// readFile returns the text of one file with a line number in front of every
// line, from the offset the model asked for and no more lines than it asked for.
// The file is read a line at a time and only as far as the window asked for, so
// that a log of two hundred megabytes costs no more memory than a note of two
// hundred bytes.
func readFile(path string, asked input) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s, so check that it is a file the agent may read: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	reader := bufio.NewReaderSize(file, ReadBufferBytes)
	sample, err := reader.Peek(binarySampleBytes)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("cannot read %s, so check that it is a file the agent may read: %w", path, err)
	}
	if notText(sample) {
		return "", fmt.Errorf("%s is not a text file, so read it with a tool that understands its kind rather than this one", path)
	}

	from, count, wanted := window(asked)
	return numbered(reader, path, from, count, wanted)
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

// oneLine is a line of the file as it was read: the text that was kept, how many
// bytes past the line cap were walked past, and whether there was a line there
// at all.
type oneLine struct {
	text    string
	dropped int
	there   bool
}

// nextLine reads the next line of the file, without the newline at its end. A
// line longer than MaxLineBytes is kept up to there and the rest of it is walked
// past and counted, because a file written as one enormous line must not be held
// in memory to be read.
func nextLine(reader *bufio.Reader) (oneLine, error) {
	line := &strings.Builder{}
	dropped := 0
	for {
		piece, more, err := reader.ReadLine()
		if errors.Is(err, io.EOF) {
			return oneLine{text: line.String(), dropped: dropped, there: line.Len() > 0}, nil
		}
		if err != nil {
			return oneLine{}, err
		}
		if line.Len() < MaxLineBytes {
			line.Write(piece)
		} else {
			dropped += len(piece)
		}
		if !more {
			return oneLine{text: line.String(), dropped: dropped, there: true}, nil
		}
	}
}

// numbered writes the lines of the window out with their numbers in front,
// stopping at the count asked for or at the byte cap. It says how to read on
// when the read was cut short by a bound rather than by the number of lines the
// model asked for, and it refuses an offset past the end of the file, saying how
// many lines the file has.
func numbered(reader *bufio.Reader, path string, from int, count int, wanted bool) (string, error) {
	written := &strings.Builder{}
	shown := 0
	at := 0
	for {
		line, err := nextLine(reader)
		if err != nil {
			return "", fmt.Errorf("cannot read %s to its end, so check that it is a file the agent may read: %w", path, err)
		}
		if !line.there {
			break
		}
		at++
		if at < from {
			continue
		}
		if shown >= count {
			if !wanted {
				fmt.Fprintf(written, "... there are more lines; read on with an offset of %d\n", at)
			}
			return written.String(), nil
		}
		one := fmt.Sprintf("%d: %s\n", at, cutRunes(line.text, MaxLineRunes, line.dropped))
		if written.Len()+len(one) > MaxBytes {
			fmt.Fprintf(written, "... this read stopped at line %d because it reached %d bytes; read on with an offset of %d\n",
				at-1, MaxBytes, at)
			return written.String(), nil
		}
		written.WriteString(one)
		shown++
	}
	if at == 0 {
		return "this file is empty\n", nil
	}
	if at < from {
		return "", fmt.Errorf("%s has %d lines, and the read starts at line %d, so start at a line the file has", path, at, from)
	}
	return written.String(), nil
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
// one enormous line cannot fill the model's window on its own, and says how much
// was left off: the characters it cut, and the bytes past the line cap that were
// never read at all.
func cutRunes(line string, length int, dropped int) string {
	letters := []rune(line)
	kept := letters
	if len(kept) > length {
		kept = kept[:length]
	}
	more := len(letters) - len(kept)
	switch {
	case more == 0 && dropped == 0:
		return line
	case dropped == 0:
		return string(kept) + fmt.Sprintf(" ... (%d more characters on this line)", more)
	default:
		return string(kept) + fmt.Sprintf(" ... (%d more characters on this line, and %d more bytes of it were not read)", more, dropped)
	}
}
