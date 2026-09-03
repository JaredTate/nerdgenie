package browser

import (
	"fmt"
	"slices"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// textBoxRole is what the worker calls anything a person types into, which is
// every kind of input box and every editable area.
const textBoxRole = "textbox"

// The words in a box's name that say which login box it is. They are matched
// against the name with its capitals folded, and only a box a person types into
// is ever considered, so the "Log in" button on a page is never mistaken for the
// box the login name goes in.
var (
	// usernameWords name the box the login name goes in.
	usernameWords = []string{"user", "email", "e-mail", "login", "account", "handle", "phone"}
	// passwordWords name the box the password goes in.
	passwordWords = []string{"password", "passphrase"}
	// codeWords name the box the second code goes in.
	codeWords = []string{"code", "one-time", "one time", "otp", "token", "authenticator", "2fa"}
)

// findBox is the reference of the first box a person types into whose name holds
// one of the words, leaving out the boxes already taken, and empty when the page
// has none.
func findBox(page contract.Snapshot, words []string, taken ...string) string {
	for _, element := range page.Elements {
		if element.Role != textBoxRole || slices.Contains(taken, element.Ref) {
			continue
		}
		folded := strings.ToLower(element.Name)
		for _, word := range words {
			if strings.Contains(folded, word) {
				return element.Ref
			}
		}
	}
	return ""
}

// chooseBox is the reference the model passed when it passed one and the page
// still holds it, and otherwise the box found by its role and its name. It says
// what is missing when neither works.
func chooseBox(page contract.Snapshot, passed string, words []string, called string, taken ...string) (string, error) {
	if passed != "" {
		if !holdsBox(page, passed) {
			return "", fmt.Errorf("there is no box called %q on %s, so read the page again and point at %s from the new snapshot",
				passed, page.URL, called)
		}
		return passed, nil
	}
	found := findBox(page, words, taken...)
	if found == "" {
		return "", fmt.Errorf("no box on %s looks like %s, so read the page and pass the reference of the right box",
			page.URL, called)
	}
	return found, nil
}

// holdsBox says whether the page has a box a person types into with that
// reference.
func holdsBox(page contract.Snapshot, ref string) bool {
	for _, element := range page.Elements {
		if element.Ref == ref && element.Role == textBoxRole {
			return true
		}
	}
	return false
}

// codeBoxOn is the box the second code goes in: the one the wall named if the
// wall named one, and otherwise the box whose name says it takes a code.
func codeBoxOn(page contract.Snapshot) string {
	if page.Wall != nil {
		detail := strings.ToLower(page.Wall.Detail)
		for _, element := range page.Elements {
			if element.Role != textBoxRole || element.Name == "" {
				continue
			}
			if strings.Contains(detail, strings.ToLower(element.Name)) {
				return element.Ref
			}
		}
	}
	return findBox(page, codeWords)
}
