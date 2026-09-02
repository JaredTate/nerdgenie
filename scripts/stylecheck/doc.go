// Package main runs the plain-English style checker over the folders named on
// its command line, and is the way make check calls internal/lint.
//
// It takes package patterns the way the Go tool does. A pattern ending in "/..."
// means the folder and everything under it, and "./..." means the whole
// repository from where the command was run. Every violation prints on its own
// line as "path:line: rule: what to do", and the command exits with 1 when there
// was at least one, so that a build stops on a style violation the same way it
// stops on a failing test.
package main
