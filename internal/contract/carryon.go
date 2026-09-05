package contract

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// CarryOnWords are the words that pick a put-down task up: a task the person
// stopped, whether their own or a job's. There is one list, read by the loop
// for a job's task and by cmd/nerdgenie for a person's own, so that the two
// cannot drift apart.
var CarryOnWords = []string{"continue", "go on", "carry on", "keep going"}

// CarryOn says whether a message picks a put-down task up, and hands back what
// the person said beyond the word. A message that is one of the words alone,
// in any case and with any punctuation after it, picks the task up with nothing
// more to say. One that begins with one of the words and goes on, such as
// "continue, but post at noon", picks the task up and hands back the rest,
// because that is carrying on and steering rather than a new ask. A word that
// merely begins with one of them, such as "continued", is not one.
func CarryOn(said string) (string, bool) {
	plain := strings.TrimSpace(said)
	for _, word := range CarryOnWords {
		if len(plain) < len(word) || !strings.EqualFold(plain[:len(word)], word) {
			continue
		}
		rest := plain[len(word):]
		if first, _ := utf8.DecodeRuneInString(rest); unicode.IsLetter(first) || unicode.IsDigit(first) {
			continue
		}
		rest = strings.TrimLeftFunc(rest, func(letter rune) bool {
			return unicode.IsSpace(letter) || unicode.IsPunct(letter)
		})
		return strings.TrimSpace(rest), true
	}
	return "", false
}
