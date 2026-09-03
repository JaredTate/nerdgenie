// The rule for judging an expectation is the one written out in
// worker/browser/PROTOCOL.md under "How the worker judges an expectation", and
// it is applied here to a page rather than to a change. A worker cannot judge
// English, and a rule the model can read is a rule the model can write an
// expectation against.

package browser

import (
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// shortestWord is how long a word must be before it counts towards an expected
// state. Anything shorter carries no meaning on its own.
const shortestWord = 4

// The words of four letters or more that say nothing about what a page shows.
// They are common enough that a state built out of them would match almost any
// page, which would make the check worthless.
var stopWords = []string{
	"about", "after", "again", "against", "because", "been", "before", "being",
	"between", "both", "could", "does", "doing", "down", "during", "each",
	"from", "further", "have", "having", "here", "into", "itself", "just",
	"more", "most", "once", "only", "other", "over", "same", "should", "some",
	"such", "than", "that", "their", "them", "then", "there", "these", "they",
	"this", "those", "through", "under", "until", "very", "were", "what",
	"when", "where", "which", "while", "will", "with", "would", "your",
}

// StateWords are the words of an expected state that are looked for on the
// page: four letters or more, folded to lowercase, and not a stop word.
func StateWords(state string) []string {
	words := []string{}
	for _, word := range strings.FieldsFunc(strings.ToLower(state), notALetterOrDigit) {
		if len([]rune(word)) < shortestWord || contains(stopWords, word) {
			continue
		}
		words = append(words, word)
	}
	return words
}

// notALetterOrDigit says where one word of an expected state ends.
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

// StateMet says whether the page bears out the expected state, by the same word
// rule the browser worker uses on an expectation. A state with no words of its
// own is met by any page that could be read at all, because there is nothing in
// it to look for.
func StateMet(state string, page contract.Snapshot) bool {
	words := StateWords(state)
	if len(words) == 0 {
		return true
	}
	haystack := strings.ToLower(strings.Join(placesToLook(page), "\n"))
	for _, word := range words {
		if strings.Contains(haystack, word) {
			return true
		}
	}
	return false
}

// placesToLook is everywhere on a page an expected state's words may appear: the
// address, the title, the name and role of every element, and the message of an
// open dialog box.
func placesToLook(page contract.Snapshot) []string {
	places := []string{page.URL, page.Title}
	for _, element := range page.Elements {
		places = append(places, element.Name, element.Role)
	}
	if page.Dialog != nil {
		places = append(places, page.Dialog.Message)
	}
	return places
}

// WhatIsShown says in plain words what the page is showing, which is what a
// report line carries when an expected state is not met. It is cut off at the
// cap, because a page with a thousand elements on it must not fill a report.
func WhatIsShown(page contract.Snapshot) string {
	said := fmt.Sprintf("the page %q at %s", page.Title, page.URL)
	if page.Wall != nil {
		said += fmt.Sprintf(", which is showing a %s wall: %s", page.Wall.Kind, page.Wall.Detail)
	}
	if page.Dialog != nil {
		said += ", with a dialog box saying " + page.Dialog.Message
	}
	if shown := elementsShown(page); shown != "" {
		said += ", showing " + shown
	}
	return shorten(said, MaxSeenRunes)
}

// elementsShown lists the elements of a page the way a person would read them
// out.
func elementsShown(page contract.Snapshot) string {
	named := []string{}
	for _, element := range page.Elements {
		named = append(named, fmt.Sprintf("%s %q", element.Role, element.Name))
	}
	return strings.Join(named, ", ")
}

// shorten cuts a line of text down to the cap and says that it was cut, so that
// nobody reads half a sentence and believes it was the whole of it.
func shorten(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + " (cut short before the end)"
}
