package record

import "strings"

// A lesson is one line, and a list of lessons holds the newest few. The fifth
// game build's play-test task wrote seventeen failures and three decisions as
// paragraphs, 1,765 tokens of failures alone, and the record passed its size
// on a harness write of one read's result line, so the task failed on the
// harness's own bookkeeping. Every lesson is still whole in the log under its
// own event; the record holds the line that teaches it.

// MaxLessonRunes is the longest a lesson's text, cause or reason may be.
const MaxLessonRunes = 160

// MaxFailuresKept is how many failures the record holds; the oldest leave.
const MaxFailuresKept = 8

// MaxDecisionsKept is how many decisions the record holds; the oldest leave.
const MaxDecisionsKept = 8

// cutToALesson keeps a lesson's line to MaxLessonRunes, cut after a whole
// word, and says that it was cut.
func cutToALesson(text string) string {
	text = strings.TrimSpace(text)
	letters := []rune(text)
	if len(letters) <= MaxLessonRunes {
		return text
	}
	kept := string(letters[:MaxLessonRunes-3])
	if space := strings.LastIndex(kept, " "); space > MaxLessonRunes/2 {
		kept = kept[:space]
	}
	return strings.TrimRight(kept, " ,;:") + "..."
}

// theNewest keeps the last most of a list, dropping the oldest.
func theNewest[Item any](list []Item, most int) []Item {
	if len(list) <= most {
		return list
	}
	return list[len(list)-most:]
}

// numberAfter is the number that follows the last label in a list, read off
// the label itself rather than the list's length, because the oldest lessons
// leave the list and nothing a reader has seen is renumbered.
func numberAfter(lastID string) int {
	number, labelled := labelNumber(lastID)
	if !labelled {
		return 1
	}
	return number + 1
}

// labelNumber is the number in a label such as "F12", or false when the label
// carries none.
func labelNumber(id string) (int, bool) {
	digits := strings.TrimLeftFunc(id, func(character rune) bool { return character < '0' || character > '9' })
	return readCount(digits)
}
