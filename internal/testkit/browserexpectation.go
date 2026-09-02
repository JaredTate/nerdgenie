package testkit

import (
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// The rule for judging an expectation is written out in
// worker/browser/PROTOCOL.md, under "How the worker judges an expectation". It
// is fixed and dull on purpose: a worker cannot judge English, and a rule the
// model can read is one it can write an expectation against.

// shortestExpectationWord is how long a word must be before it counts. Anything
// shorter carries no meaning on its own.
const shortestExpectationWord = 4

// The words of four letters or more that say nothing about what a page did. They
// are common enough that an expectation built out of them would match almost any
// page, which would make the check worthless.
var expectationStopWords = []string{
	"about", "after", "again", "against", "because", "been", "before", "being",
	"between", "both", "could", "does", "doing", "down", "during", "each",
	"from", "further", "have", "having", "here", "into", "itself", "just",
	"more", "most", "once", "only", "other", "over", "same", "should", "some",
	"such", "than", "that", "their", "them", "then", "there", "these", "they",
	"this", "those", "through", "under", "until", "very", "were", "what",
	"when", "where", "which", "while", "will", "with", "would", "your",
}

// expectationWords are the words of an expectation the worker looks for: four
// letters or more, folded to lowercase, and not a stop word.
func expectationWords(expectation string) []string {
	words := []string{}
	for _, word := range strings.FieldsFunc(strings.ToLower(expectation), notALetterOrDigit) {
		if len([]rune(word)) < shortestExpectationWord {
			continue
		}
		if contains(expectationStopWords, word) {
			continue
		}
		words = append(words, word)
	}
	return words
}

// notALetterOrDigit says where one word of an expectation ends.
func notALetterOrDigit(letter rune) bool {
	isLetter := letter >= 'a' && letter <= 'z'
	isDigit := letter >= '0' && letter <= '9'
	return !isLetter && !isDigit
}

// contains says whether the list holds the word.
func contains(list []string, wanted string) bool {
	for _, held := range list {
		if held == wanted {
			return true
		}
	}
	return false
}

// meetsExpectation judges one action against what the model said it expected, by
// the rule the protocol writes out. An empty expectation is met when anything
// changed at all.
func meetsExpectation(expectation string, diff contract.Diff, aimedAt contract.Element) bool {
	words := expectationWords(expectation)
	if len(words) == 0 {
		return somethingChanged(diff)
	}
	haystack := strings.ToLower(strings.Join(placesToLook(diff, aimedAt), "\n"))
	for _, word := range words {
		if strings.Contains(haystack, word) {
			return true
		}
	}
	return false
}

// placesToLook is where the protocol says an expectation's words may appear: a
// new element's name or role, the new address, the new title, a dialog's
// message, and the name and role of the element the action was aimed at. That
// last one is what lets "the text box holds the post" hold after typing into a
// box named "Post text", since typing changes no element.
func placesToLook(diff contract.Diff, aimedAt contract.Element) []string {
	places := []string{diff.URL, diff.Snapshot.Title, aimedAt.Name, aimedAt.Role}
	for _, element := range diff.NewElements {
		places = append(places, element.Name, element.Role)
	}
	if diff.Dialog != nil {
		places = append(places, diff.Dialog.Message)
	}
	return places
}

// somethingChanged says whether the action did anything to the page at all.
func somethingChanged(diff contract.Diff) bool {
	return diff.URLChanged || len(diff.NewElements) > 0 || diff.Dialog != nil ||
		diff.Download != nil || diff.NewTab != ""
}

// whatChanged says in plain words what the action did, which is what the model
// reads when what it expected did not happen.
func whatChanged(expectation string, diff contract.Diff) string {
	if diff.Wall != nil {
		return fmt.Sprintf("expected %q, and the page is showing a %s wall: %s",
			expectation, diff.Wall.Kind, diff.Wall.Detail)
	}
	if !diff.Settled {
		return fmt.Sprintf("expected %q, and the page kept changing past the limit, so this is the page as it stood at %s",
			expectation, diff.Snapshot.Title)
	}
	return fmt.Sprintf("expected %q, and instead %s, on the page called %s",
		expectation, strings.Join(changesIn(diff), ", "), diff.Snapshot.Title)
}

// changesIn lists what the action actually did, in plain words.
func changesIn(diff contract.Diff) []string {
	changes := []string{}
	if diff.URLChanged {
		changes = append(changes, "the page moved to "+diff.URL)
	}
	if len(diff.NewElements) > 0 {
		changes = append(changes, fmt.Sprintf("%d elements appeared", len(diff.NewElements)))
	}
	if diff.Dialog != nil {
		changes = append(changes, "a dialog box opened saying "+diff.Dialog.Message)
	}
	if diff.NewTab != "" {
		changes = append(changes, "a tab opened")
	}
	if diff.Download != nil {
		changes = append(changes, "a download started")
	}
	if len(changes) == 0 {
		changes = append(changes, "nothing on the page changed")
	}
	return changes
}
