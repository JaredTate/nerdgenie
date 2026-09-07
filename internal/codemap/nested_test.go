package codemap_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/codemap"
)

// The files a model writes for a browser game are rarely modules: run
// eighteen's game.js opened with 'use strict' and wrapped everything in an
// immediately invoked function, and its server.js opened with three requires
// before its first comment. The map read nothing from either, so the model
// read the files whole. These fixtures are shaped exactly like those files.

const anIIFEGame = `'use strict';

// Tic-tac-toe: the board, the rules and the rendering.
(function () {
  const SIZE = 3;

  // Makes an empty board.
  function makeBoard() {
    return Array(SIZE * SIZE).fill(null);
  }

  // Says whether the move is legal.
  const isLegal = (board, at) => board[at] === null;

  function winnerOf(board) {
    for (let line = 0; line < 8; line++) {
      if (board[line]) {
        return board[line];
      }
    }
    while (false) {
      break;
    }
    return null;
  }

  window.Game = { makeBoard, isLegal, winnerOf };
})();
`

func TestAFileWrappedInAnImmediatelyInvokedFunctionListsTheFunctionsInsideIt(t *testing.T) {
	file := codemap.Read("game.js", []byte(anIIFEGame))
	if file.FirstLine != "Tic-tac-toe: the board, the rules and the rendering." {
		t.Errorf("the first line reads %q, want the comment past 'use strict'", file.FirstLine)
	}
	want := []string{
		"func makeBoard() → Makes an empty board.",
		"func isLegal(board, at) → Says whether the move is legal.",
		"func winnerOf(board) → ",
	}
	if got := lines(file); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the names read\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

const aNodeServer = `'use strict';
const http = require('http');
const fs = require('fs');
const path = require('path');

// A static file server for the game on port 8091.
const PORT = 8091;

// Serves one request from the folder.
function serve(request, response) {
  response.end('ok');
}

http.createServer(serve).listen(PORT);
`

func TestAFileThatOpensWithRequiresStillHasItsFirstCommentAsItsFirstLine(t *testing.T) {
	file := codemap.Read("server.js", []byte(aNodeServer))
	if file.FirstLine != "A static file server for the game on port 8091." {
		t.Errorf("the first line reads %q, want the comment past the requires", file.FirstLine)
	}
	if got := lines(file); strings.Join(got, "\n") != "func serve(request, response) → Serves one request from the folder." {
		t.Errorf("the names read %v", got)
	}
	module := codemap.Read("app.mjs", []byte("import { a } from './a.js';\nimport b from 'b';\n\n// The app.\nexport function start() {}\n"))
	if module.FirstLine != "The app." {
		t.Errorf("a module's first line reads %q, want the comment past the imports", module.FirstLine)
	}
}

const anObjectLiteral = `// The renderer: draws the board into the page.
const view = {
  // Draws the board.
  draw(board) {
    return board;
  },
  clear: function () {
    return null;
  },
  mark: (at) => at,
  handlers: {
    onClick(event) {
      return event;
    },
  },
};

function helper() {}
`

func TestAnObjectLiteralsMethodsAreListedUnderIt(t *testing.T) {
	file := codemap.Read("view.js", []byte(anObjectLiteral))
	want := []string{
		"const view → The renderer: draws the board into the page.",
		"method draw(board) → Draws the board.",
		"method clear() → ",
		"method mark(at) → ",
		"method onClick(event) → ",
		"func helper() → ",
	}
	if got := lines(file); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the names read\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

const aClassThenFunctions = `// The board.
export class Board {
  constructor(size) {
    this.size = size;
  }

  // Places a mark.
  place(at, mark) {
    return true;
  }
}

// Reads the board off the page.
function readBoard(page) {
  return page;
}

function helper() {
  function inner(x) {
    return x;
  }
  if (true) {
    return inner(1);
  }
}
`

func TestAClassClosesAtItsBraceSoWhatFollowsIsNotItsMethods(t *testing.T) {
	file := codemap.Read("board.js", []byte(aClassThenFunctions))
	want := []string{
		"class Board → The board.",
		"method constructor(size) → ",
		"method place(at, mark) → Places a mark.",
		"func readBoard(page) → Reads the board off the page.",
		"func helper() → ",
		"func inner(x) → ",
	}
	if got := lines(file); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the names read\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestAPythonClassClosesWhereTheIndentationEnds(t *testing.T) {
	file := codemap.Read("game.py", []byte("class Board:\n    def place(self, at):\n        return at\n\ndef play():\n    def inner():\n        pass\n    return inner\n"))
	want := []string{"class Board → ", "method place(self, at) → ", "func play() → "}
	if got := lines(file); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the names read\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestACodeFileWithNoNamesSaysSoInTheMap(t *testing.T) {
	printed := codemap.Print("Game", []codemap.File{codemap.Read("index.js", []byte("window.onload = () => {\n  start();\n};\n"))})
	if !strings.Contains(printed, "### index.js\n- no names read\n") {
		t.Errorf("the entry of a file the map read nothing from is silent:\n%s", printed)
	}
}

func TestTheScopesAreBounded(t *testing.T) {
	var source strings.Builder
	source.WriteString("class A {\n")
	for at := 0; at < 100; at++ {
		source.WriteString(strings.Repeat(" ", at+2) + "const o" + strings.Repeat("x", at%5) + " = {\n")
	}
	source.WriteString("  place(at) {\n  }\n")
	file := codemap.Read("deep.js", []byte(source.String()))
	if len(file.Names) > codemap.MaxNamesPerFile {
		t.Errorf("%d names, over the cap", len(file.Names))
	}
}
