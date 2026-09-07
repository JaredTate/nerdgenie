package codemap_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/codemap"
)

const aJavaScriptFile = `// The engine: the board and the pieces, with no rendering in it.
import { rng } from './rng.js';

/**
 * Spawns the next piece at the top of the board. Returns false when it cannot.
 * @param {Board} board the board
 */
export function spawn(board, piece) {
  return true;
}

// Locks the piece into the board and clears full rows.
export const lock = (board, piece) => {
  return board;
};

export class Board {
  // Makes an empty board of the given size.
  constructor(width, height) {
    this.width = width;
  }

  /** Says whether the cell is free. */
  isFree(x, y) {
    return true;
  }

  static fromRows(rows) {
    return new Board(rows[0].length, rows.length);
  }
}

function helper() {
  if (true) {
    return 1;
  }
}

export const GRAVITY_MS = 800;
`

func TestAJavaScriptFilesFunctionsClassesAndConstantsAreListedWithTheirCommentsFirstSentence(t *testing.T) {
	file := codemap.Read("src/engine.js", []byte(aJavaScriptFile))
	if file.FirstLine != "The engine: the board and the pieces, with no rendering in it." {
		t.Errorf("the file's first line reads %q, want the comment at its top", file.FirstLine)
	}
	want := []string{
		"func spawn(board, piece) → Spawns the next piece at the top of the board.",
		"func lock(board, piece) → Locks the piece into the board and clears full rows.",
		"class Board → ",
		"method constructor(width, height) → Makes an empty board of the given size.",
		"method isFree(x, y) → Says whether the cell is free.",
		"method fromRows(rows) → ",
		"func helper() → ",
		"const GRAVITY_MS → ",
	}
	got := lines(file)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the names read\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

const aGoFile = `// Package engine keeps the board.
package engine

// Board is the grid the pieces fall into.
type Board struct{ cells [][]int }

// Spawn puts the next piece at the top. It says whether it fit.
func Spawn(board *Board, piece Piece) bool { return true }

// Lock locks the piece in and clears the full rows.
func (board *Board) Lock(piece Piece) int {
	return 0
}

func helper() {}
`

func TestAGoFilesTypesFunctionsAndMethodsAreListed(t *testing.T) {
	file := codemap.Read("internal/engine/board.go", []byte(aGoFile))
	want := []string{
		"type Board → Board is the grid the pieces fall into.",
		"func Spawn(board *Board, piece Piece) → Spawn puts the next piece at the top.",
		"method Lock(piece Piece) → Lock locks the piece in and clears the full rows.",
		"func helper() → ",
	}
	if got := lines(file); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the names read\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

const aPythonFile = `"""The physics: lift, drag and gravity, with no rendering."""

class Aircraft:
    """One aircraft and its state."""

    def step(self, dt):
        """Advances the state by dt seconds. Returns the new state."""
        return self

def lift(speed, angle):
    """Lift from speed and angle of attack."""
    return 0.0

def test_lift_grows_with_speed():
    assert lift(2, 1) > lift(1, 1)
`

func TestAPythonFilesClassesFunctionsAndDocstringsAreListedAndItsTestsCounted(t *testing.T) {
	file := codemap.Read("sim/physics.py", []byte(aPythonFile))
	if file.FirstLine != "The physics: lift, drag and gravity, with no rendering." {
		t.Errorf("the first line reads %q, want the module docstring", file.FirstLine)
	}
	want := []string{
		"class Aircraft → One aircraft and its state.",
		"method step(self, dt) → Advances the state by dt seconds.",
		"func lift(speed, angle) → Lift from speed and angle of attack.",
		"func test_lift_grows_with_speed() → ",
	}
	if got := lines(file); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the names read\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if file.Tests != 1 {
		t.Errorf("the file counts %d tests, want one", file.Tests)
	}
}

const aCppFile = `// Validates blocks against the chain.
#include "validation.h"

/** Validates block structure, merkle root and size limits. */
bool CheckBlock(const CBlock& block, BlockValidationState& state)
{
    if (block.vtx.empty()) {
        return false;
    }
    return true;
}

class CChainState {
public:
    // Connects a validated block to the chain and updates the UTXO set.
    bool ConnectBlock(const CBlock& block) const {
        return true;
    }
};

static int helper(int x) {
    return x;
}
`

func TestACppFilesFunctionsAndClassesAreListedWithoutTheirControlFlow(t *testing.T) {
	file := codemap.Read("src/validation.cpp", []byte(aCppFile))
	want := []string{
		"func CheckBlock(const CBlock& block, BlockValidationState& state) → Validates block structure, merkle root and size limits.",
		"class CChainState → ",
		"method ConnectBlock(const CBlock& block) → Connects a validated block to the chain and updates the UTXO set.",
		"func helper(int x) → ",
	}
	if got := lines(file); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the names read\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestATestFileIsKnownAndItsTestsAreCounted(t *testing.T) {
	js := codemap.Read("test/engine.test.js", []byte("import test from 'node:test';\ntest('spawns at the top', () => {});\nit('locks', () => {});\ndescribe('board', () => { test('is empty', () => {}); });\n"))
	if !js.IsTest || js.Tests != 3 {
		t.Errorf("the node test file reads test=%v tests=%d, want a test file with three tests", js.IsTest, js.Tests)
	}
	goTest := codemap.Read("internal/engine/board_test.go", []byte("package engine\nfunc TestSpawn(t *testing.T) {}\nfunc TestLock(t *testing.T) {}\nfunc helper() {}\n"))
	if !goTest.IsTest || goTest.Tests != 2 {
		t.Errorf("the Go test file reads test=%v tests=%d, want a test file with two tests", goTest.IsTest, goTest.Tests)
	}
}

func TestAFileThatIsNotCodeHasNoNames(t *testing.T) {
	for _, path := range []string{"package.json", "index.html", "styles.css", "README.md", "assets/logo.png"} {
		file := codemap.Read(path, []byte("# not code\n{\"a\": 1}\n"))
		if len(file.Names) != 0 || file.IsCode {
			t.Errorf("%s read as code with %d names", path, len(file.Names))
		}
	}
}

func TestTheNamesOfOneFileAreBounded(t *testing.T) {
	var source strings.Builder
	for at := 0; at < codemap.MaxNamesPerFile+20; at++ {
		source.WriteString("function f" + strings.Repeat("x", at%7) + "_" + string(rune('a'+at%26)) + "() {}\n")
	}
	file := codemap.Read("big.js", []byte(source.String()))
	if len(file.Names) != codemap.MaxNamesPerFile || file.More != 20 {
		t.Errorf("a big file lists %d names with %d more, want %d and 20", len(file.Names), file.More, codemap.MaxNamesPerFile)
	}
}

func TestTheMapIsPrintedInTheGuidesShape(t *testing.T) {
	files := []codemap.File{
		codemap.Read("src/engine.js", []byte(aJavaScriptFile)),
		codemap.Read("test/engine.test.js", []byte("test('spawns', () => {});\n")),
		codemap.Read("index.html", []byte("<html></html>")),
	}
	printed := codemap.Print("Tetris", files)
	for _, want := range []string{
		"# Repository Map: Tetris",
		codemap.GeneratedMark,
		"## Roots",
		"- `src/` - 1 files",
		"## Source files",
		"### src/engine.js",
		"The engine: the board and the pieces, with no rendering in it.",
		"- `spawn(board, piece)` → Spawns the next piece at the top of the board.",
		"- `Board` (class)",
		"  - `isFree(x, y)` → Says whether the cell is free.",
		"- `GRAVITY_MS` (const)",
		"## Tests",
		"### test/engine.test.js",
		"- 1 test",
		"## Other files",
		"index.html",
	} {
		if !strings.Contains(printed, want) {
			t.Errorf("the map lacks %q; it reads:\n%s", want, printed)
		}
	}
	if strings.Index(printed, "## Source files") > strings.Index(printed, "## Tests") {
		t.Errorf("the tests come before the source files")
	}
}

func FuzzRead(f *testing.F) {
	f.Add("a.js", aJavaScriptFile)
	f.Add("a.go", aGoFile)
	f.Add("a.py", aPythonFile)
	f.Add("a.cpp", aCppFile)
	f.Add("a.rs", "/// Lifts.\npub fn lift(x: f64) -> f64 { x }\nstruct Wing;\n#[test]\nfn lifts() {}\n")
	f.Add("a.rb", "# Lifts.\ndef lift(x)\nend\nclass Wing\nend\n")
	f.Fuzz(func(t *testing.T, path string, source string) {
		file := codemap.Read(path, []byte(source))
		if len(file.Names) > codemap.MaxNamesPerFile {
			t.Errorf("%d names, over the cap", len(file.Names))
		}
		for _, name := range file.Names {
			if name.Name == "" {
				t.Errorf("a name with no name: %+v", name)
			}
		}
		_ = codemap.Print("fuzz", []codemap.File{file})
	})
}

// lines flattens a file's names the way the tests compare them.
func lines(file codemap.File) []string {
	var out []string
	for _, name := range file.Names {
		out = append(out, name.Kind+" "+name.Signature+" → "+name.Says)
	}
	return out
}
