// Package markdown cuts a Markdown text into its sections by heading, so that
// the harness can hand the model one part of a document instead of the whole:
// one section of an architecture page, one part of a long ask, one folder of a
// map. A heading line is a run of one to six hash marks and a space at the
// start of a line, outside a code fence; a section runs from its heading to
// the next heading of any level. Nothing here is a parser of Markdown, and
// nothing needs to be: headings and fences are the only shapes it reads.
package markdown
