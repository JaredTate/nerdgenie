package context

import (
	"os"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// smallCaps are the two size limits the persona files are cut at, made small so
// that a test can go over them without writing a page of text.
var smallCaps = contract.MemoryCaps{WorldFactsBytes: 40, UserFactsBytes: 30}

// TestAHomeWithNoPersonaFilesBuildsAnEmptyPersona proves that a fresh install,
// which has no persona files at all, still builds a prompt. A missing file is
// not an error: the user has simply not written it yet.
func TestAHomeWithNoPersonaFilesBuildsAnEmptyPersona(t *testing.T) {
	home := testkit.NewTempHome(t)
	text, err := readPersona(home, smallCaps)
	if err != nil {
		t.Fatalf("a home with no persona files should build an empty persona: %v", err)
	}
	if text != "" {
		t.Errorf("the persona of an empty home reads %q, want nothing at all", text)
	}
}

// TestThePersonaHoldsTheThreeFilesInOrder proves the three files arrive in the
// order the design puts them in, each under a heading that names the file the
// user edits.
func TestThePersonaHoldsTheThreeFilesInOrder(t *testing.T) {
	home := testkit.NewTempHome(t)
	writePersonaFile(t, home.SoulFile(), "I am Coeus.")
	writePersonaFile(t, home.UserFactsFile(), "Jared runs DigiByte.")
	writePersonaFile(t, home.WorldFactsFile(), "DigiByte launched in 2014.")

	text, err := readPersona(home, contract.DefaultConfig().MemoryCaps)
	if err != nil {
		t.Fatalf("cannot read the persona: %v", err)
	}
	for _, wanted := range []string{"I am Coeus.", "Jared runs DigiByte.", "DigiByte launched in 2014."} {
		if !strings.Contains(text, wanted) {
			t.Errorf("the persona does not carry %q:\n%s", wanted, text)
		}
	}
	soul, user, world := strings.Index(text, "I am"), strings.Index(text, "Jared"), strings.Index(text, "DigiByte launched")
	if !(soul < user && user < world) {
		t.Errorf("the three persona files are out of order: SOUL.md at %d, USER.md at %d, MEMORY.md at %d", soul, user, world)
	}
}

// TestAPersonaFileOverItsLimitIsCutWithANote proves an oversized file is cut
// rather than crowding out the task, and that the note says which file to
// shorten. The rule is Hermes': a memory file with no limit grows until it
// crowds out the work.
func TestAPersonaFileOverItsLimitIsCutWithANote(t *testing.T) {
	home := testkit.NewTempHome(t)
	writePersonaFile(t, home.WorldFactsFile(), strings.Repeat("fact. ", 100))

	text, err := readPersona(home, smallCaps)
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

	text, err := readPersona(home, smallCaps)
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
	if _, err := readPersona(home, smallCaps); err == nil {
		t.Error("a persona file that cannot be read was passed over in silence")
	}
}

// writePersonaFile puts one persona file in place.
func writePersonaFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write %s: %v", path, err)
	}
}
