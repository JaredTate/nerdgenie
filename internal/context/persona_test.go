package context

import (
	"os"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// smallCaps are the two size limits the persona files are cut at, made small so
// that a test can go over them without writing a page of text.
var smallCaps = contract.MemoryCaps{WorldFactsBytes: 40, UserFactsBytes: 30}

// TestAHomeWithNoPersonaFilesBuildsAnEmptyPersona proves that a fresh install,
// which has no persona files at all, still builds a prompt. A missing file is
// not an error: the user has simply not written it yet.
func TestAHomeWithNoPersonaFilesBuildsAnEmptyPersona(t *testing.T) {
	home := testkit.NewTempHome(t)
	text, err := readPersona(home)
	if err != nil {
		t.Fatalf("a home with no persona files should build an empty persona: %v", err)
	}
	if text != "" {
		t.Errorf("the persona of an empty home reads %q, want nothing at all", text)
	}
	known, err := readWhatIsKnown(home, smallCaps)
	if err != nil {
		t.Fatalf("a home with no memory files should build nothing known: %v", err)
	}
	if known != "" {
		t.Errorf("what an empty home knows reads %q, want nothing at all", known)
	}
}

// TestTheThreeFilesArriveInOrderAndInTheRightHalves proves the three files
// arrive in the order the design puts them in, each under a heading that names
// the file the user edits, and that the split of finding 18 holds: SOUL.md, which
// only the user writes, is the persona; USER.md and MEMORY.md, which the agent
// writes into with the `memory` tool, are what it knows and go below the cache
// line.
func TestTheThreeFilesArriveInOrderAndInTheRightHalves(t *testing.T) {
	home := testkit.NewTempHome(t)
	writePersonaFile(t, home.SoulFile(), "I am Nerd Genie.")
	writePersonaFile(t, home.UserFactsFile(), "Jared runs DigiByte.")
	writePersonaFile(t, home.WorldFactsFile(), "DigiByte launched in 2014.")

	persona, err := readPersona(home)
	if err != nil {
		t.Fatalf("cannot read the persona: %v", err)
	}
	known, err := readWhatIsKnown(home, contract.DefaultConfig().MemoryCaps)
	if err != nil {
		t.Fatalf("cannot read what the agent knows: %v", err)
	}

	if !strings.Contains(persona, "I am Nerd Genie.") {
		t.Errorf("the persona does not carry SOUL.md:\n%s", persona)
	}
	for _, unwanted := range []string{"Jared runs DigiByte.", "DigiByte launched in 2014."} {
		if strings.Contains(persona, unwanted) {
			t.Errorf("the persona carries %q, which the agent rewrites mid-task, so a memory save would move the top of the prompt:\n%s",
				unwanted, persona)
		}
	}
	for _, wanted := range []string{"Jared runs DigiByte.", "DigiByte launched in 2014."} {
		if !strings.Contains(known, wanted) {
			t.Errorf("what the agent knows does not carry %q:\n%s", wanted, known)
		}
	}
	if strings.Contains(known, "I am Nerd Genie.") {
		t.Errorf("what the agent knows carries SOUL.md, which belongs in the persona:\n%s", known)
	}
	if user, world := strings.Index(known, "Jared"), strings.Index(known, "DigiByte launched"); user > world {
		t.Errorf("USER.md at %d comes after MEMORY.md at %d, and the design puts it first", user, world)
	}
}

// TestAPersonaFileOverItsLimitIsCutWithANote proves an oversized file is cut
// rather than crowding out the task, and that the note says which file to
// shorten. The rule is Hermes': a memory file with no limit grows until it
// crowds out the work.
func TestAPersonaFileOverItsLimitIsCutWithANote(t *testing.T) {
	home := testkit.NewTempHome(t)
	writePersonaFile(t, home.WorldFactsFile(), strings.Repeat("fact. ", 100))

	text, err := readWhatIsKnown(home, smallCaps)
	if err != nil {
		t.Fatalf("cannot read the persona: %v", err)
	}
	if !strings.Contains(text, "MEMORY.md") {
		t.Errorf("the note on the cut file does not name it:\n%s", text)
	}
	if !strings.Contains(text, cutNoteMark) {
		t.Errorf("an oversized persona file was cut with no note saying so:\n%s", text)
	}
	if len(text) > 400 {
		t.Errorf("the persona is %d bytes and the cap on this file was %d, so the cut did not happen",
			len(text), smallCaps.WorldFactsBytes)
	}
}

// TestAPersonaFileIsCutOnACharacterBoundary proves the cut never leaves half a
// letter behind, because a file may be written in any language.
func TestAPersonaFileIsCutOnACharacterBoundary(t *testing.T) {
	home := testkit.NewTempHome(t)
	writePersonaFile(t, home.UserFactsFile(), strings.Repeat("日", 40))

	text, err := readWhatIsKnown(home, smallCaps)
	if err != nil {
		t.Fatalf("cannot read the persona: %v", err)
	}
	if !strings.Contains(text, "USER.md") {
		t.Errorf("the note on the cut file does not name it:\n%s", text)
	}
	if strings.ContainsRune(text, '\uFFFD') {
		t.Errorf("the cut broke a character in half:\n%q", text)
	}
}

// TestAPersonaFolderThatCannotBeReadIsAnError proves a file that is there but
// unreadable is reported rather than quietly left out, because a persona the
// user wrote and the model never sees is worse than no persona at all.
func TestAPersonaFolderThatCannotBeReadIsAnError(t *testing.T) {
	home := testkit.NewTempHome(t)
	if err := os.Mkdir(home.SoulFile(), contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot put a folder where SOUL.md belongs: %v", err)
	}
	if _, err := readPersona(home); err == nil {
		t.Error("a persona file that cannot be read was passed over in silence")
	}
}

// TestAMemoryFileThatCannotBeReadIsAnError proves the same holds for the two
// files below the cache line, which the reader reached by another road.
func TestAMemoryFileThatCannotBeReadIsAnError(t *testing.T) {
	home := testkit.NewTempHome(t)
	if err := os.Mkdir(home.WorldFactsFile(), contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot put a folder where MEMORY.md belongs: %v", err)
	}
	if _, err := readWhatIsKnown(home, smallCaps); err == nil {
		t.Error("a memory file that cannot be read was passed over in silence")
	}
}

// writePersonaFile puts one persona file in place.
func writePersonaFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write %s: %v", path, err)
	}
}
