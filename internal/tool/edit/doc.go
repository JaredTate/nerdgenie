// Package edit is the edit tool: it replaces one span of text in one file, and
// finds the span even when the model's copy of it is not quite the file's.
//
// A model reading a file back and quoting part of it gets the words right and
// the whitespace wrong more often than not, so an editor that only accepts an
// exact match fails on work it could have done. This one tries the exact text
// first and then four fallbacks in order: lines that match once trailing spaces
// are off, a line that matches once every run of spaces is one space, a block
// that matches once the indentation both sides share is taken off, and the text
// with its ends trimmed found as a substring. Every one of them must land on
// exactly one place in the file, because an editor that guesses between two
// candidates is worse than one that refuses; a span found twice is refused with
// the count, and a span found nowhere is refused with the text that was looked
// for. What the file held goes into the event log before the file is touched, in
// the same shape the write tool uses, so the undo command can put it back.
package edit
