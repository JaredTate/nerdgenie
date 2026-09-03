// The idea of cutting a command line down to the part a person recognises comes
// from OpenCode's prefix table at
// ~/Code/opencode/packages/opencode/src/permission/arity.ts. This file holds the
// step before that one: turning a line of shell into the separate commands it
// runs and the words each of them was given.

package permission

import "strings"

// The caps that keep one command line from turning into work without end. A
// model can write anything, so the reader stops at each of these, and says so
// when it does, because text dropped quietly is exactly where a command the user
// would want to see about hides.
const (
	// maxCommandBytes is how much of one command line is read at all.
	maxCommandBytes = 8192
	// maxSegments is how many separate commands one line may hold.
	maxSegments = 64
	// maxWordsPerSegment is how many words are read from one command.
	maxWordsPerSegment = 256
	// maxWordBytes is how long one word may be before the rest of it is
	// dropped, because a word longer than this is data and not a command.
	maxWordBytes = 512
)

// The two notes a readable form ends with when it is not the whole story. A
// bound that hides the end of a command, or a command that has not been written
// yet when the reducer reads it, both turn the ask-me-first list off silently, so
// the form says out loud what it leaves out and the permission function puts such
// a call to the user.
const (
	// cutShortNote says the words stopped before the end of the command line.
	cutShortNote = "(cut short before the end)"
	// buildsItselfNote says the command works out part of what it will run
	// while it runs, as a command substitution or a pair of backticks does.
	buildsItselfNote = "(builds part of itself at run time)"
	// unclosedQuoteNote says a quote was opened and never closed, so where one
	// word ends and the next begins is a guess. Everything after such a quote,
	// the characters that end one command and start another among them, is read
	// as more of the same word, and the program's own name can come out of it
	// carrying text that was never part of it.
	unclosedQuoteNote = "(a quote that is never closed)"
)

// commandSeparators are the characters that end one command and start another:
// the three shell operators, a line break, and the openers and closers of a
// command substitution, so that a command hidden inside another one is still
// read as a command.
const commandSeparators = "|&;\n\r()`"

// substitutionOpeners are the characters that turn the bracket after them into
// a command the shell works out while it runs: "$(" is a command substitution
// and "<(" and ">(" are process substitutions. A backtick is the older spelling
// of the same thing and needs no opener.
const substitutionOpeners = "$<>"

// commandWords cuts a command line into the separate commands a shell would run,
// each already split into the words it was given. Quotes hold a word together
// and are dropped, and a backslash escapes the character after it. The second
// value is the note saying these words are not the whole command line, and it is
// empty when they are.
func commandWords(command string) ([][]string, string) {
	splitter := &lineSplitter{}
	for position, letter := range command {
		if position >= maxCommandBytes || len(splitter.segments) >= maxSegments {
			splitter.markNotWholeStory(cutShortNote)
			break
		}
		splitter.read(letter)
	}
	segments := splitter.done()
	return segments, splitter.notWholeStory
}

// lineSplitter reads a command line one character at a time and collects the
// words of each separate command in it.
type lineSplitter struct {
	segments [][]string
	words    []string
	current  strings.Builder
	inWord   bool
	quote    rune
	escaped  bool

	// previous is the character read just before this one, which is how a "$("
	// is told from a bracket that opens a plain subshell.
	previous rune
	// notWholeStory is the note saying what these words leave out, and stays
	// empty while they still say everything the command line will do.
	notWholeStory string
}

// markNotWholeStory records the first reason these words stopped saying
// everything the command line will do.
func (splitter *lineSplitter) markNotWholeStory(note string) {
	if splitter.notWholeStory == "" {
		splitter.notWholeStory = note
	}
}

// read takes in one character of the command line.
func (splitter *lineSplitter) read(letter rune) {
	switch {
	case splitter.escaped:
		splitter.write(letter)
		splitter.escaped = false
	case splitter.quote == '\'':
		splitter.readInsideSingleQuotes(letter)
	case letter == '\\':
		splitter.escaped = true
		splitter.inWord = true
	case splitter.quote == '"':
		splitter.readInsideDoubleQuotes(letter)
	case letter == '\'' || letter == '"':
		splitter.quote = letter
		splitter.inWord = true
	case strings.ContainsRune(commandSeparators, letter):
		splitter.markWhenTheCommandBuildsItself(letter)
		splitter.endSegment()
	case letter == ' ' || letter == '\t':
		splitter.endWord()
	default:
		splitter.write(letter)
	}
	splitter.previous = letter
}

// markWhenTheCommandBuildsItself says the words are no longer the whole story
// when this character opens a command the shell works out while it runs: a
// backtick, or the bracket of a "$(", "<(" or ">(" substitution. Such a command
// is not written down anywhere the reducer can read, so nothing read beforehand
// can say what it will do.
func (splitter *lineSplitter) markWhenTheCommandBuildsItself(letter rune) {
	if letter == '`' || (letter == '(' && strings.ContainsRune(substitutionOpeners, splitter.previous)) {
		splitter.markNotWholeStory(buildsItselfNote)
	}
}

// readInsideSingleQuotes takes in one character between single quotes, where a
// backslash means nothing and only the closing quote ends the run.
func (splitter *lineSplitter) readInsideSingleQuotes(letter rune) {
	if letter == '\'' {
		splitter.quote = 0
		return
	}
	splitter.write(letter)
}

// readInsideDoubleQuotes takes in one character between double quotes, where the
// closing quote ends the run and a substitution still builds a command, because
// double quotes hold the words of one argument together and nothing more.
func (splitter *lineSplitter) readInsideDoubleQuotes(letter rune) {
	if letter == '"' {
		splitter.quote = 0
		return
	}
	splitter.markWhenTheCommandBuildsItself(letter)
	splitter.write(letter)
}

// write adds one character to the word being built, and says the words are no
// longer the whole story once one of them runs past the cap.
func (splitter *lineSplitter) write(letter rune) {
	splitter.inWord = true
	if splitter.current.Len() >= maxWordBytes {
		splitter.markNotWholeStory(cutShortNote)
		return
	}
	splitter.current.WriteRune(letter)
}

// endWord finishes the word being built, if there is one.
func (splitter *lineSplitter) endWord() {
	if !splitter.inWord {
		return
	}
	if len(splitter.words) < maxWordsPerSegment {
		splitter.words = append(splitter.words, splitter.current.String())
	} else {
		splitter.markNotWholeStory(cutShortNote)
	}
	splitter.current.Reset()
	splitter.inWord = false
}

// endSegment finishes the command being built, if it has any words.
func (splitter *lineSplitter) endSegment() {
	splitter.endWord()
	if len(splitter.words) == 0 {
		return
	}
	splitter.segments = append(splitter.segments, splitter.words)
	splitter.words = nil
}

// done finishes whatever is still being built and returns every command found.
// A quote still open at the end of the line means the words are not the whole
// story, because a shell reading the same line would not end it where this
// reader did.
func (splitter *lineSplitter) done() [][]string {
	if splitter.quote != 0 {
		splitter.markNotWholeStory(unclosedQuoteNote)
	}
	splitter.endSegment()
	return splitter.segments
}
