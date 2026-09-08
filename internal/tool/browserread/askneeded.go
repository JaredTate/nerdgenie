package browserread

import (
	"errors"
	"strings"
	"unicode"
)

// errAskNeeded refuses a read whose intent says it evaluates something on the
// page and which carries no ask, with one sentence and one example: five
// rounds of the flight simulator's sky task were the model saying "I need to
// pass an ask parameter" and not passing it.
var errAskNeeded = errors.New(`this call's intent says it evaluates something on the page, but it carries no ask; ` +
	`write the expression in the ask field, such as ask: "JSON.stringify(window.sim.state)"`)

// evaluatingWords are the words an intent uses when it means to run something
// on the page rather than read what is on it. Beside them, the two words
// "read back" mean the same.
var evaluatingWords = map[string]bool{
	"evaluate": true, "diagnose": true, "run": true, "execute": true,
	"script": true, "js": true, "javascript": true, "state": true,
}

// needAsk refuses an intent that names one of the evaluating words, or says
// "read back", when the call carries no ask. A word is matched whole, in any
// case, so "statement" is not "state" and "running" is not "run".
func needAsk(intent string, ask string) error {
	if ask != "" {
		return nil
	}
	words := strings.FieldsFunc(strings.ToLower(intent), func(letter rune) bool {
		return !unicode.IsLetter(letter) && !unicode.IsDigit(letter)
	})
	for at, word := range words {
		if evaluatingWords[word] || (word == "read" && at+1 < len(words) && words[at+1] == "back") {
			return errAskNeeded
		}
	}
	return nil
}
