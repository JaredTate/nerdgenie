package codemap_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/codemap"
)

// aPrintedMap is a map of three files as Print writes it.
func aPrintedMap() string {
	return codemap.Print("Game", []codemap.File{
		codemap.Read("src/game.js", []byte(anIIFEGame)),
		codemap.Read("src/index.js", []byte("start();\n")),
		codemap.Read("test/game.test.js", []byte("// The game's tests.\ntest('a', () => {});\nit('b', () => {});\n")),
		codemap.Read("index.html", []byte("<html></html>")),
	})
}

func TestTheContentsOfAMapAreItsRootsAndOneLinePerFile(t *testing.T) {
	printed := aPrintedMap()
	if !codemap.IsGenerated(printed) {
		t.Error("a map Print wrote is not known as generated")
	}
	contents := codemap.Contents(printed, 100)
	for _, want := range []string{
		"## Roots\n\n- `./` - 1 files\n- `src/` - 2 files\n- `test/` - 1 files\n",
		"- src/game.js — Tic-tac-toe: the board, the rules and the rendering.\n",
		"- src/index.js — no names read\n",
		"- test/game.test.js — The game's tests. (2 tests)\n",
		"- index.html\n",
		"Read a file's functions with `read REPO_MAP.md <path>`.",
	} {
		if !strings.Contains(contents, want) {
			t.Errorf("the contents lack %q:\n%s", want, contents)
		}
	}
	for _, absent := range []string{"makeBoard", "written by Nerd Genie", "more files"} {
		if strings.Contains(contents, absent) {
			t.Errorf("the contents carry %q:\n%s", absent, contents)
		}
	}
}

func TestTheContentsAreBoundedAndSayHowManyFilesWereLeftOff(t *testing.T) {
	contents := codemap.Contents(aPrintedMap(), 2)
	if !strings.Contains(contents, "- src/game.js") || !strings.Contains(contents, "- src/index.js") || strings.Contains(contents, "index.html") {
		t.Errorf("the first two files are not the two kept:\n%s", contents)
	}
	if !strings.Contains(contents, "- and 2 more files\n") {
		t.Errorf("the contents do not say two files were left off:\n%s", contents)
	}
}

func TestAMapWithoutRootsOrTheMarkIsStillListedOrKnownAsAPersons(t *testing.T) {
	if codemap.IsGenerated("# My map\n\nWritten by hand.\n" + strings.Repeat("x\n", 600) + codemap.GeneratedMark) {
		t.Error("a mark far down a hand-written map made it generated")
	}
	contents := codemap.Contents("# Repository Map: Bare\n\n## Source files\n\n### a.js\n- `f()` → Does f.\n\n### b.js\nThe b file.\n- `g()`\n", 10)
	if strings.Contains(contents, "## Roots") {
		t.Errorf("a map with no roots lists some:\n%s", contents)
	}
	if !strings.Contains(contents, "- a.js\n- b.js — The b file.\n") {
		t.Errorf("a file with no first line is not listed bare, or the next is not listed with its first line:\n%s", contents)
	}
}

func TestAnEntrysBodyIsWhatSitsUnderItsHeading(t *testing.T) {
	source := codemap.EntryBody(codemap.Read("src/game.js", []byte(anIIFEGame)))
	if !strings.HasPrefix(source, "Tic-tac-toe: the board, the rules and the rendering.\n- `makeBoard()` → Makes an empty board.\n") || strings.Contains(source, "### ") {
		t.Errorf("a source file's body reads:\n%s", source)
	}
	test := codemap.EntryBody(codemap.Read("test/game.test.js", []byte("// The tests.\ntest('a', () => {});\n")))
	if test != "The tests.\n- 1 test\n" {
		t.Errorf("a test file's body reads %q", test)
	}
}

func TestTabsIndentAndDecoratorsSitBetweenACommentAndItsDef(t *testing.T) {
	tabbed := codemap.Read("board.js", []byte("class Board {\n\t// Places a mark.\n\tplace(at) {\n\t}\n}\nfunction free() {}\n"))
	want := []string{"class Board → ", "method place(at) → Places a mark.", "func free() → "}
	if got := lines(tabbed); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("a tab-indented class reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	decorated := codemap.Read("board.py", []byte("class Board:\n    # Makes a board.\n    @staticmethod\n    def make():\n        pass\n"))
	if got := lines(decorated); strings.Join(got, "\n") != "class Board → \nmethod make() → Makes a board." {
		t.Errorf("a decorated method reads %v", got)
	}
}
