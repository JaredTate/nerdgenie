package loop_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/codemap"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// A map written at a task's end is stale by the task's second write. On run
// eighteen the model read REPO_MAP.md at a task's start and found the entries
// of the files it had just written empty. So a write or an edit under the
// project folder refreshes that file's entry at once, when the map there is
// the harness's own.

// aStaleMap is a generated map whose one entry names a function the file no
// longer has.
func aStaleMap(paths ...string) string {
	var files []codemap.File
	for _, path := range paths {
		files = append(files, codemap.File{Path: path, IsCode: true, Names: []codemap.Name{{Kind: "func", Name: "old", Signature: "old()"}}})
	}
	return codemap.Print("game", files)
}

// aWriteOf is a harness whose model writes the one file and answers.
func aWriteOf(t *testing.T, path string) *harness {
	t.Helper()
	return newHarness(t, []testkit.Step{
		callStep("I will write the game.", callFor("c1", contract.ToolWrite, `{"path":"`+path+`","content":"x"}`)),
		answerStep("The game is written. What changed: game.js. What I checked: nothing. What is left: nothing."),
	}, scriptedTool(contract.ToolWrite, "created the file"))
}

func aFileIn(t *testing.T, folder string, path string, text string) string {
	t.Helper()
	whole := filepath.Join(folder, path)
	if err := os.MkdirAll(filepath.Dir(whole), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(whole, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return whole
}

func TestAWriteUnderTheProjectFolderRefreshesThatFilesEntryInTheMap(t *testing.T) {
	folder := t.TempDir()
	game := aFileIn(t, folder, "src/game.js", "// The game.\n\n// Spawns the first piece.\nfunction spawn(board) {}\n")
	aFileIn(t, folder, loop.MapFile, aStaleMap("src/game.js", "src/view.js"))
	built := aWriteOf(t, game)
	built.workFolder = folder
	built.loop = mustBuild(t, built)

	built.ask(t, "write the game")

	written, err := os.ReadFile(filepath.Join(folder, loop.MapFile))
	if err != nil {
		t.Fatal(err)
	}
	text := string(written)
	gameAt, viewAt := strings.Index(text, "### src/game.js\n"), strings.Index(text, "### src/view.js\n")
	if gameAt < 0 || viewAt < gameAt {
		t.Fatalf("the map lost an entry or their order after the write; it reads:\n%s", text)
	}
	entry := text[gameAt:viewAt]
	if !strings.Contains(entry, "The game.\n- `spawn(board)` → Spawns the first piece.\n") || strings.Contains(entry, "old()") {
		t.Errorf("the written file's entry is not fresh after the write; it reads:\n%s", entry)
	}
	if !strings.Contains(text[viewAt:], "- `old()`") {
		t.Errorf("the other file's entry was changed by the write:\n%s", text[viewAt:])
	}
}

func TestAWriteOfAFileTheMapDoesNotHaveWritesTheWholeMapAgain(t *testing.T) {
	folder := t.TempDir()
	game := aFileIn(t, folder, "src/game.js", "// The game.\nfunction spawn(board) {}\n")
	aFileIn(t, folder, "src/view.js", "// The view.\nfunction draw() {}\n")
	aFileIn(t, folder, loop.MapFile, aStaleMap("src/view.js"))
	built := aWriteOf(t, game)
	built.workFolder = folder
	built.loop = mustBuild(t, built)

	built.ask(t, "write the game")

	written, err := os.ReadFile(filepath.Join(folder, loop.MapFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"### src/game.js\nThe game.\n- `spawn(board)`", "### src/view.js\nThe view.\n- `draw()`"} {
		if !strings.Contains(string(written), want) {
			t.Errorf("the map lacks %q after the write; it reads:\n%s", want, string(written))
		}
	}
}

func TestAHandWrittenMapAndAFolderWithoutOneAreLeftAloneByAWrite(t *testing.T) {
	folder := t.TempDir()
	game := aFileIn(t, folder, "src/game.js", "function spawn(board) {}\n")
	mine := aFileIn(t, folder, loop.MapFile, "# My map\n\nWritten by hand.\n")
	built := aWriteOf(t, game)
	built.workFolder = folder
	built.loop = mustBuild(t, built)
	built.ask(t, "write the game")
	if kept, _ := os.ReadFile(mine); string(kept) != "# My map\n\nWritten by hand.\n" {
		t.Errorf("a map the person wrote was changed by a write:\n%s", string(kept))
	}

	bare := t.TempDir()
	other := aFileIn(t, bare, "src/game.js", "function spawn(board) {}\n")
	again := aWriteOf(t, other)
	again.workFolder = bare
	again.loop = mustBuild(t, again)
	again.ask(t, "write the game")
	if _, err := os.Stat(filepath.Join(bare, loop.MapFile)); err == nil {
		t.Errorf("a write in a folder with no map wrote one, which is the task's end's job")
	}
}

func TestAWriteOutsideTheProjectFolderTouchesNoMap(t *testing.T) {
	folder := t.TempDir()
	elsewhere := aFileIn(t, t.TempDir(), "src/game.js", "function spawn(board) {}\n")
	stale := aStaleMap("src/game.js")
	aFileIn(t, folder, loop.MapFile, stale)
	built := aWriteOf(t, elsewhere)
	built.workFolder = folder
	built.loop = mustBuild(t, built)

	built.ask(t, "write the game")

	if kept, _ := os.ReadFile(filepath.Join(folder, loop.MapFile)); string(kept) != stale {
		t.Errorf("a write outside the folder changed its map:\n%s", string(kept))
	}
}

// mustBuild builds the loop again after the harness's folder was changed.
func mustBuild(t *testing.T, built *harness) *loop.Loop {
	t.Helper()
	made, err := loop.New(built.options())
	if err != nil {
		t.Fatalf("cannot build the loop: %v", err)
	}
	return made
}
