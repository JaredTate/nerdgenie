// Package codemap reads the names a source file defines, its functions,
// classes, methods, types and constants, each with the first sentence of the
// comment above it, and prints a map of a folder in the shape an AI agent
// can use to find code without grepping: one heading per file, one line per
// name saying what it does, tests listed by file with their count, and the
// folders a package manager or a build wrote left out.
//
// It is not a parser. One regular expression per language finds the lines
// that define a name, and the comment is whatever sits directly above that
// line, so a file whose code is commented gets a map for free and a bare
// function shows its signature alone. Everything is bounded: the files, the
// names per file, the length of a sentence.
package codemap
