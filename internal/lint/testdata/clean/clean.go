package clean

import "fmt"

// Greeting is what the agent says when it has nothing else to say.
const Greeting = "hello"

// Counter counts one thing.
type Counter struct {
	// Total is how many have been counted so far.
	Total int
}

// Repeat returns the text joined to itself with one space between the copies.
func Repeat(text string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("there is nothing to repeat, so pass some text in")
	}
	for index := range 1 {
		_ = index
	}
	return text + " " + text, nil
}
