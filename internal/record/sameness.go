package record

import (
	"strings"
	"unicode"
)

// A lesson written twice teaches nothing twice. The model rewords a failure a
// little each time it writes it again, so two failures say the same thing when
// they share nearly all their words, not only when they match letter for
// letter.

// TheShareOfWordsThatMakesTheSame is the share of one failure's distinct words
// that must appear in another's for the two to say the same thing.
const TheShareOfWordsThatMakesTheSame = 0.8

// TheFewestWordsCompared is the size below which only the same words in the
// same order count as the same, because a few words shared prove little.
const TheFewestWordsCompared = 4

// saysTheSame says whether a new failure says what a held one already says.
func saysTheSame(fresh string, held string) bool {
	freshWords, heldWords := wordsOf(fresh), wordsOf(held)
	if strings.Join(freshWords, " ") == strings.Join(heldWords, " ") {
		return true
	}
	if len(freshWords) < TheFewestWordsCompared || len(heldWords) < TheFewestWordsCompared {
		return false
	}
	inHeld := map[string]bool{}
	for _, word := range heldWords {
		inHeld[word] = true
	}
	distinct := map[string]bool{}
	shared := 0
	for _, word := range freshWords {
		if distinct[word] {
			continue
		}
		distinct[word] = true
		if inHeld[word] {
			shared++
		}
	}
	return float64(shared) >= TheShareOfWordsThatMakesTheSame*float64(len(distinct))
}

// wordsOf is the text as lowercase words of letters and digits, in order.
func wordsOf(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character)
	})
}
