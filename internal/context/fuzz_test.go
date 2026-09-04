package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// FuzzThePersonaReaderTakesAnyBytes throws anything at the three persona files,
// which are plain text a person edits by hand and so are outside text in the
// sense the work plan means. The reader must never panic, and it must never hand
// back more than the caps allow.
func FuzzThePersonaReaderTakesAnyBytes(f *testing.F) {
	f.Add("I am Nerd Genie.", "The user is Jared.", "DigiByte launched in 2014.", 40)
	f.Add("", "", "", 1)
	f.Add(strings.Repeat("a", 5000), "\x00\xff", "日本語", 7)
	f.Add("a\nb\rc", "-----", "\xed\xa0\x80", 0)

	f.Fuzz(func(t *testing.T, soul string, user string, world string, limit int) {
		home := fuzzHome(t, soul, user, world)
		caps := contract.MemoryCaps{UserFactsBytes: boundedLimit(limit), WorldFactsBytes: boundedLimit(limit)}

		persona, err := readPersona(home)
		if err != nil {
			t.Fatalf("an ordinary SOUL.md could not be read: %v", err)
		}
		known, err := readWhatIsKnown(home, caps)
		if err != nil {
			t.Fatalf("two ordinary memory files could not be read: %v", err)
		}
		text := persona + known
		room := SoulBytes + caps.UserFactsBytes + caps.WorldFactsBytes + 3*len(soulHeading) + 3*300
		if len(text) > room {
			t.Errorf("the persona and what is known came back as %d bytes together, and the caps allow at most %d", len(text), room)
		}
	})
}

// FuzzTheCutKeepsWholeCharacters throws any text and any limit at the cut, which
// is the one place this package takes a piece out of the middle of something a
// person wrote.
func FuzzTheCutKeepsWholeCharacters(f *testing.F) {
	f.Add("a plain line of English", 5)
	f.Add("日本語のテキスト", 4)
	f.Add("", 0)
	f.Add("\xff\xfe\xfd", 2)

	f.Fuzz(func(t *testing.T, text string, limit int) {
		cut := cutToLimit(text, "USER.md", limit)
		if limit <= 0 || len(text) <= limit {
			if cut != text {
				t.Errorf("nothing needed cutting and %q came back as %q", text, cut)
			}
			return
		}
		if !strings.HasPrefix(cut, kept(text, limit)) {
			t.Errorf("the cut of %q is not a prefix of it: %q", text, cut)
		}
		if utf8.ValidString(text) && !utf8.ValidString(cut) {
			t.Errorf("the cut broke a whole character out of %q: %q", text, cut)
		}
		if !strings.Contains(cut, cutNoteMark) {
			t.Errorf("%q was cut with no note saying so: %q", text, cut)
		}
	})
}

// kept is what the cut may have held on to: the whole of the text up to the
// limit, less whatever it dropped to land on a character boundary.
func kept(text string, limit int) string {
	held := text[:limit]
	for len(held) > 0 && !utf8.ValidString(held) {
		held = held[:len(held)-1]
	}
	return held
}

// boundedLimit keeps a fuzzed cap inside the range a configuration could hold,
// because a cap of minus one billion says nothing about the reader.
func boundedLimit(limit int) int {
	if limit < 1 {
		return 1
	}
	if limit > 100000 {
		return 100000
	}
	return limit
}

// fuzzHome writes the three persona files under a folder of their own, without
// the environment variable the ordinary temporary home sets, because a fuzz
// worker runs many cases in one process.
func fuzzHome(t *testing.T, soul string, user string, world string) contract.Home {
	t.Helper()
	home := contract.NewHome(filepath.Join(t.TempDir(), contract.HomeFolderName))
	if err := os.MkdirAll(home.PersonaFolder(), contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the persona folder: %v", err)
	}
	for path, content := range map[string]string{
		home.SoulFile():       soul,
		home.UserFactsFile():  user,
		home.WorldFactsFile(): world,
	} {
		if err := os.WriteFile(path, []byte(content), contract.DataFileMode); err != nil {
			t.Fatalf("cannot write %s: %v", path, err)
		}
	}
	return home
}
