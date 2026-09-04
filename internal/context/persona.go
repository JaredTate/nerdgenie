// The rule that the instruction files are read from disk again on every step,
// rather than remembered from the conversation, is OpenCode's. Its version is at
// ~/Code/opencode/packages/opencode/src/session/instruction.ts, where the
// project's own files are resolved and read on each request so that nothing the
// user wrote can be summarized away. The Go here is written fresh.
//
// The hard size limit on a memory file is Hermes'. A file with no limit grows
// until it crowds out the task it was meant to help with.

package context

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// SoulBytes is the size limit on SOUL.md, the file that says who the agent is.
//
// The other two persona files take their limits from contract.MemoryCaps, which
// has a field for MEMORY.md and one for USER.md and none for this one. The
// design says all three files have a limit, so the gap is in the contract rather
// than here, and it has been reported. Until the contract carries the third
// limit, this is it, set at the same size as USER.md.
const SoulBytes = 4000

// cutNoteMark opens the note left behind when a persona file was too long. It is
// the words a reader can search the prompt for, and the note that follows names
// the file to shorten.
const cutNoteMark = "[cut here:"

// The three headings that name each persona file in the prompt, so that the
// model knows which of the three it is reading and the user knows which file to
// edit.
const (
	soulHeading  = "**Who you are, from SOUL.md.**"
	userHeading  = "**What is true about the user, from USER.md.**"
	worldHeading = "**What is true about the world, from MEMORY.md.**"
)

// personaFile is one of the three files, with the heading it is printed under
// and the size it is cut at.
type personaFile struct {
	path    string
	heading string
	limit   int
}

// readPersona reads SOUL.md, the file that says who the agent is, and returns it
// as the persona block of the system prompt. Only this one of the three files is
// up there, because only the user writes it: the agent's own `memory` tool and
// its after-action review write into the other two, and a block above cache
// boundary A that the agent rewrites mid-task costs it the whole prompt
// underneath. Those two are read by readWhatIsKnown instead.
//
// It is read on every build rather than remembered, which is what makes a file
// the user edited by hand take effect on the very next turn.
func readPersona(home contract.Home) (string, error) {
	return readPersonaFiles([]personaFile{
		{path: home.SoulFile(), heading: soulHeading, limit: SoulBytes},
	})
}

// readWhatIsKnown reads USER.md and MEMORY.md, the two files the agent writes
// into as well as the user. They go below the cache line, near the memory hint
// they belong with, so that saving one fact in the middle of a task costs only
// the few lines after it rather than everything under cache boundary A.
func readWhatIsKnown(home contract.Home, caps contract.MemoryCaps) (string, error) {
	return readPersonaFiles([]personaFile{
		{path: home.UserFactsFile(), heading: userHeading, limit: caps.UserFactsBytes},
		{path: home.WorldFactsFile(), heading: worldHeading, limit: caps.WorldFactsBytes},
	})
}

// readPersonaFiles reads a run of persona files from disk and joins them under
// their headings.
//
// A file that is not there is passed over, because a fresh install has none of
// them. A file that is longer than its limit is cut on a character boundary and
// a note says which file to shorten. Any other failure to read is returned,
// because a persona the user wrote and the model never sees is worse than no
// persona at all.
func readPersonaFiles(files []personaFile) (string, error) {
	parts := []string{}
	for _, file := range files {
		content, err := os.ReadFile(file.path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("cannot read the persona file %s, so check that it is a readable file: %w", file.path, err)
		}
		text := strings.TrimRight(string(content), "\n")
		if text == "" {
			continue
		}
		parts = append(parts, file.heading+"\n"+cutToLimit(text, file.path, file.limit))
	}
	return strings.Join(parts, "\n\n"), nil
}

// cutToLimit keeps a persona file inside its size limit and says plainly when it
// had to cut. The cut lands on a character boundary, so a file written in any
// language comes back as readable text rather than half a letter.
func cutToLimit(text string, path string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	kept := text[:limit]
	for len(kept) > 0 && !utf8.ValidString(kept) {
		kept = kept[:len(kept)-1]
	}
	return fmt.Sprintf("%s\n%s %s is over its limit of %d bytes, so the rest was left out. Shorten the file to see all of it.]",
		kept, cutNoteMark, path, limit)
}
