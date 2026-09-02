// Package read is the read tool: a file with its line numbers, a folder as a
// listing, or a past result by its label.
//
// The three are one tool because they are one question, "show me that", and the
// model should not have to choose between three names to ask it. A path is read
// as a file or as a folder, whichever it is, and is allowed only inside the
// folders the agent may work in. A label such as r7 or j4.2 is not a path at
// all: it is a result this task or this job wrote earlier, whose one line is
// still in the record and whose whole text is in the event log, and reading it
// is what makes the record's promise true that nothing is ever lost. A file that
// is not text is refused rather than turned into nonsense, and everything is
// bounded: the lines read at once, the bytes read at once, the length of one
// line, and the entries of one folder.
package read
