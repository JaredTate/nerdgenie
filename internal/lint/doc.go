// Package lint checks that Nerd Genie source obeys the plain-English rules in
// CLAUDE.md, and it is used only by make check.
//
// Six of the seven rules read one file at a time: a comment on a declaration is
// a complete sentence, above it or on the end of its line; every exported name
// carries a doc comment; no function body runs past sixty lines; no file runs
// past five hundred; an error message is at least four lowercase words, not
// counting its format verbs, with no full stop; and no identifier is a bare
// letter or a jargon abbreviation, with the conventional t, b, f, r and w
// allowed in a test and in a request handler. The seventh rule reads the file's
// top comment: a file that names a project whose design it borrowed must also
// say where the reference file lives. The rule about a package having a doc.go
// is the one thing that needs the whole folder rather than one file.
//
// A file the parser cannot read at all is reported as a violation of its own,
// naming the file, because a checker that said nothing about it would let a
// broken file through the gate in silence.
//
// The checker reports rather than fixes. Every violation prints as
// "path:line: rule: what to do", because a message that does not say what to do
// is a message the reader has to guess at.
package lint
