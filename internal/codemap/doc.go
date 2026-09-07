// Package codemap reads the names a source file defines, its functions,
// classes, methods, types and constants, each with the first sentence of the
// comment above it, and prints a map of a folder in the shape an AI agent
// can use to find code without grepping: one heading per file, one line per
// name saying what it does, tests listed by file with their count, and the
// folders a package manager or a build wrote left out. A map read whole
// answers with its contents, the roots and one line per file, and one
// file's entry can be written back into the map on its own.
//
// It is not a parser. A few regular expressions per language find the lines
// that define a name, at any depth, because a browser game is written inside
// a function that runs at once as often as it is a module; a class or an
// object literal opens a scope whose indented method-shaped lines are its
// methods until its brace, or its indentation, closes it. The comment is
// whatever sits directly above the line, so a file whose code is commented
// gets a map for free and a bare function shows its signature alone. The
// file's own first line is its first comment past the strict-mode pragma and
// the imports. Everything is bounded: the files, the names per file, the
// depth of the scopes, the length of a sentence.
package codemap
