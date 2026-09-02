// Package lint checks that Coeus source obeys the plain-English rules in
// CLAUDE.md, and it is used only by make check.
//
// Six of the seven rules read one file at a time: a comment on a declaration is
// a complete sentence, every exported name carries a doc comment, no function
// body runs past sixty lines, no file runs past five hundred, an error message
// is at least four lowercase words with no full stop, and no identifier is a
// bare letter or a jargon abbreviation. The seventh rule reads the file's top
// comment: a file that names a project whose design it borrowed must also say
// where the reference file lives. The rule about a package having a doc.go is
// the one thing that needs the whole folder rather than one file.
//
// The checker reports rather than fixes. Every violation prints as
// "path:line: rule: what to do", because a message that does not say what to do
// is a message the reader has to guess at.
package lint
