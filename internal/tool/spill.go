// The idea of keeping the whole of a long tool result on disk and handing the
// model a preview that names the file is OpenCode's, at
// ~/Code/opencode/packages/opencode/src/tool/truncate.ts. The Go here is written
// fresh: Coeus names the file by the task it belongs to and caps the folder.

package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/JaredTate/coeus/internal/contract"
)

// MaxResultBytes is the most of one tool result that is kept at all. A tool that
// returns more than this had a run away with it, and the whole of it is no use
// to anybody, so the rest is dropped before anything is written to disk.
const MaxResultBytes = 8 << 20

// MaxSpillFolderBytes is how much the spill folder may hold. Once it is over,
// the oldest files go first, because the newest result is the one the model is
// still reading.
const MaxSpillFolderBytes = 64 << 20

// SpillFolderName is the folder under the home folder's run folder that the
// whole text of a long result is written into. It sits in the run folder because
// nothing in it has to survive a restart.
const SpillFolderName = "spill"

// spillFolder is where this registry writes the whole of a long result.
func (registry *Registry) spillFolder() string {
	return filepath.Join(registry.settings.Home.RunFolder(), SpillFolderName)
}

// applyOutputCap returns the result the model sees. A result inside the cap is
// handed back untouched. A longer one is written whole to a file and cut to the
// cap, with one line at the end naming the file, so that nothing is lost and the
// model can read the rest with the read tool.
func (registry *Registry) applyOutputCap(output contract.ToolOutput) (contract.ToolOutput, error) {
	outputCap := registry.settings.outputCap()
	if len(output.Text) <= outputCap {
		return output, nil
	}

	whole := output.Text
	if len(whole) > MaxResultBytes {
		whole = cutToLength(whole, MaxResultBytes)
	}
	path, err := registry.writeSpill(whole)
	if err != nil {
		return contract.ToolOutput{}, err
	}

	note := fmt.Sprintf("\n\n... %d bytes of this result are not shown. The whole of it is in %s, which the read tool opens.",
		len(whole), path)
	room := outputCap - len(note)
	if room < 0 {
		room = 0
	}
	return contract.ToolOutput{Text: cutToLength(whole, room) + note, SpillPath: path}, nil
}

// writeSpill writes the whole of one result into the spill folder under a name
// nothing else in this task has used, and makes room for it first.
func (registry *Registry) writeSpill(whole string) (string, error) {
	folder := registry.spillFolder()
	if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
		return "", fmt.Errorf("cannot make the folder %s to keep the whole of a long result in: %w", folder, err)
	}
	if err := makeRoomIn(folder, len(whole)); err != nil {
		return "", err
	}

	path := filepath.Join(folder, registry.nextSpillName(folder))
	if err := os.WriteFile(path, []byte(whole), contract.DataFileMode); err != nil {
		return "", fmt.Errorf("cannot write the whole of a long result to %s: %w", path, err)
	}
	return path, nil
}

// nextSpillName is the name the next spilled result of this task takes: the task
// and a number one above the highest the folder already holds for it, so that a
// registry built again for the same task never writes over what it wrote before.
func (registry *Registry) nextSpillName(folder string) string {
	task := registry.settings.TaskID
	if strings.TrimSpace(task) == "" {
		task = "task"
	}
	prefix := task + "-"

	highest := 0
	entries, err := os.ReadDir(folder)
	if err == nil {
		for _, entry := range entries {
			rest, named := strings.CutPrefix(entry.Name(), prefix)
			number, err := strconv.Atoi(strings.TrimSuffix(rest, ".txt"))
			if named && err == nil && number > highest {
				highest = number
			}
		}
	}
	return fmt.Sprintf("%s%d.txt", prefix, highest+1)
}

// makeRoomIn removes the oldest files in the folder until what it holds, plus
// what is about to be written, is inside the cap.
func makeRoomIn(folder string, wanted int) error {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return fmt.Errorf("cannot read the folder %s to make room in it: %w", folder, err)
	}

	held := int64(0)
	kept := []os.FileInfo{}
	for _, entry := range entries {
		about, err := entry.Info()
		if err != nil || entry.IsDir() {
			continue
		}
		held += about.Size()
		kept = append(kept, about)
	}
	sort.Slice(kept, func(left, right int) bool {
		return kept[left].ModTime().Before(kept[right].ModTime())
	})

	for _, about := range kept {
		if held+int64(wanted) <= MaxSpillFolderBytes {
			return nil
		}
		if err := os.Remove(filepath.Join(folder, about.Name())); err != nil {
			return fmt.Errorf("cannot remove the old file %s to make room for a new result: %w", about.Name(), err)
		}
		held -= about.Size()
	}
	return nil
}

// cutToLength cuts text to a number of bytes without splitting a character in
// half, because half a character is not text the model can read.
func cutToLength(text string, length int) string {
	if length <= 0 {
		return ""
	}
	if len(text) <= length {
		return text
	}
	for length > 0 && !utf8.RuneStart(text[length]) {
		length--
	}
	return text[:length]
}
