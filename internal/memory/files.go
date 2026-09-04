package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// maxNoteBytes is the biggest one note in the memory folder may be. A dated
// note that reaches it is left alone and the next one for that day is started,
// so that no single file grows without end.
const maxNoteBytes = 64 * 1024

// maxFileBytes is the biggest memory file this package will read. Anything
// larger is not a memory file somebody meant to write, and reading it would
// pull an unbounded amount of text into the program.
const maxFileBytes = 1 << 20

// maxNotesPerDay is how many dated notes one day may hold before a save gives
// up and says so, which is far more than a day of overflow can ever fill.
const maxNotesPerDay = 100

// factFamily says which of the two memory files a fact belongs in.
type factFamily string

const (
	// worldFacts are the facts about the world, which live in MEMORY.md.
	worldFacts factFamily = "world"
	// userFacts are the facts about the user, which live in USER.md.
	userFacts factFamily = "user"
)

// userFactPrefix is what marks an id as belonging to a fact about the user,
// which is what puts the fact in USER.md rather than MEMORY.md. A fact that
// supersedes another goes wherever that one went, whatever its id says.
const userFactPrefix = "u"

// factFile is one of the two memory files: where it is, how big it may get, and
// the name the dated notes its overflow moves into are built from.
type factFile struct {
	path  string
	limit int
	stem  string
}

// fileOf returns the file one family of facts lives in.
func (memory *Memory) fileOf(family factFamily) factFile {
	if family == userFacts {
		return factFile{path: memory.home.UserFactsFile(), limit: memory.caps.UserFactsBytes, stem: "USER"}
	}
	return factFile{path: memory.home.WorldFactsFile(), limit: memory.caps.WorldFactsBytes, stem: "MEMORY"}
}

// writtenFile is one file a save wrote, with what it held before, so that a
// save that fails afterwards can put it back and so that the change can be
// written into the event log.
type writtenFile struct {
	path    string
	existed bool
	prior   []byte
	mode    fs.FileMode
}

// asEventBody turns the record of a written file into the body of a file-change
// event, which is what an undo reads to put the file back.
func (file writtenFile) asEventBody() (json.RawMessage, error) {
	body, err := json.Marshal(contract.FileChangeBody{
		Path:          file.path,
		Existed:       file.existed,
		PriorContents: file.prior,
		Mode:          uint32(file.mode),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot write down the change to %s as an event: %w", file.path, err)
	}
	return body, nil
}

// putFilesBack restores every file a failed save had already written, newest
// first, so that a save either happened or did not.
func putFilesBack(written []writtenFile) {
	for index := len(written) - 1; index >= 0; index-- {
		file := written[index]
		if !file.existed {
			_ = os.Remove(file.path)
			continue
		}
		_ = os.WriteFile(file.path, file.prior, file.mode)
	}
}

// readFactFile reads one memory file into the lines somebody wrote by hand,
// which are kept as they are, and the facts written on the fact lines. A file
// that is not there reads as an empty one.
func readFactFile(path string) (kept []string, facts []contract.Fact, err error) {
	held, err := readWholeFile(path)
	if err != nil {
		return nil, nil, err
	}
	for _, line := range strings.Split(string(held), "\n") {
		if fact, isFact := parseFactLine(line); isFact {
			facts = append(facts, fact)
			continue
		}
		if strings.TrimSpace(line) != "" {
			kept = append(kept, strings.TrimRight(line, "\r"))
		}
	}
	return kept, facts, nil
}

// readWholeFile reads a memory file, treating one that is not there as empty
// and refusing one far larger than any memory file should be.
func readWholeFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot look at the memory file %s: %w", path, err)
	}
	if info.Size() > maxFileBytes {
		return nil, fmt.Errorf("the memory file %s is %d bytes and this package reads at most %d, so shorten it or move it out of the memory folder", path, info.Size(), maxFileBytes)
	}
	held, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read the memory file %s: %w", path, err)
	}
	return held, nil
}

// renderFactFile writes the hand-written lines and then the fact lines, which
// is the shape every memory file has.
func renderFactFile(kept []string, facts []contract.Fact) []byte {
	builder := strings.Builder{}
	for _, line := range kept {
		builder.WriteString(line)
		builder.WriteString("\n")
	}
	for _, fact := range facts {
		builder.WriteString(formatFactLine(fact))
		builder.WriteString("\n")
	}
	return []byte(builder.String())
}

// renderedSize is how many bytes a file would hold, which is what the cap is
// measured against.
func renderedSize(kept []string, facts []contract.Fact) int {
	return len(renderFactFile(kept, facts))
}

// writeFileAtomically writes a file through a temporary name and a rename, so
// that a reader never sees half a file, and returns what the file held before.
func writeFileAtomically(path string, data []byte) (writtenFile, error) {
	file := writtenFile{path: path, mode: contract.DataFileMode}
	info, err := os.Stat(path)
	switch {
	case err == nil:
		file.existed = true
		file.mode = info.Mode().Perm()
		if file.prior, err = readWholeFile(path); err != nil {
			return file, err
		}
	case !errors.Is(err, fs.ErrNotExist):
		return file, fmt.Errorf("cannot look at the memory file %s before writing it: %w", path, err)
	}

	beingWritten := path + ".writing"
	if err := os.WriteFile(beingWritten, data, file.mode); err != nil {
		return file, fmt.Errorf("cannot write the memory file %s: %w", beingWritten, err)
	}
	if err := os.Rename(beingWritten, path); err != nil {
		_ = os.Remove(beingWritten)
		return file, fmt.Errorf("cannot put the memory file %s in place: %w", path, err)
	}
	return file, nil
}

// datedNote returns the note in the memory folder that facts leaving a file
// move into today, starting a new one when today's has reached its size limit.
func (memory *Memory) datedNote(family factFamily) (string, error) {
	day := memory.clock.Now().UTC().Format("2006-01-02")
	stem := memory.fileOf(family).stem
	for number := 1; number <= maxNotesPerDay; number++ {
		name := fmt.Sprintf("%s-%s.md", stem, day)
		if number > 1 {
			name = fmt.Sprintf("%s-%s-%d.md", stem, day, number)
		}
		path := filepath.Join(memory.home.MemoryFolder(), name)
		info, err := os.Stat(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return path, nil
		case err != nil:
			return "", fmt.Errorf("cannot look at the note %s the oldest facts would move into: %w", path, err)
		case info.Size() < maxNoteBytes:
			return path, nil
		}
	}
	return "", fmt.Errorf("the memory folder already holds %d notes dated %s, so move some of them somewhere else", maxNotesPerDay, day)
}

// movedFactsNote says whether a file in the memory folder is one the overflow
// wrote, which is indexed as the facts inside it rather than as one note.
func movedFactsNote(name string) bool {
	for _, stem := range []string{"MEMORY-", "USER-"} {
		if strings.HasPrefix(name, stem) && strings.HasSuffix(name, ".md") {
			return true
		}
	}
	return false
}

// appendFactsToNote adds the facts leaving a memory file to the end of a dated
// note, starting the note with a line saying what it is when it is new.
func appendFactsToNote(path string, stem string, facts []contract.Fact) (writtenFile, error) {
	held, err := readWholeFile(path)
	if err != nil {
		return writtenFile{}, err
	}
	builder := strings.Builder{}
	if len(held) == 0 {
		builder.WriteString(fmt.Sprintf("# facts moved out of %s.md when it reached its size limit\n\n", stem))
	} else {
		builder.Write(held)
		if !strings.HasSuffix(string(held), "\n") {
			builder.WriteString("\n")
		}
	}
	for _, fact := range facts {
		builder.WriteString(formatFactLine(fact))
		builder.WriteString("\n")
	}
	return writeFileAtomically(path, []byte(builder.String()))
}

// insideTheHome returns a path written relative to the home folder and with
// forward slashes, which is how the index refers to every file it holds.
func (memory *Memory) insideTheHome(path string) string {
	relative, err := filepath.Rel(memory.home.Root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}
