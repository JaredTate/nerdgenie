package contract_test

import (
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// TestTheCarryOnWordsAreTheFourTheLoopAndTheProgramShare pins the one list of
// words that pick a put-down task up. The loop reads it for a job's task and
// cmd/nerdgenie for a person's own, and there used to be two hand-kept copies.
func TestTheCarryOnWordsAreTheFourTheLoopAndTheProgramShare(t *testing.T) {
	want := []string{"continue", "go on", "carry on", "keep going"}
	if len(contract.CarryOnWords) != len(want) {
		t.Fatalf("there are %d carry-on words, want the four: %v", len(contract.CarryOnWords), want)
	}
	for at, word := range want {
		if contract.CarryOnWords[at] != word {
			t.Errorf("carry-on word %d is %q, want %q", at+1, contract.CarryOnWords[at], word)
		}
	}
}

// TestCarryOnReadsAMessageThatBeginsWithOneOfTheWords proves how a message is
// read: one of the words alone, in any case and with any punctuation after it,
// picks the task up with nothing more to say; one of the words followed by
// more picks it up and hands the rest back, because "continue, but post at
// noon" is carrying on and steering, not a new ask; and a word that only
// begins with one of them, such as "continued", is not one.
func TestCarryOnReadsAMessageThatBeginsWithOneOfTheWords(t *testing.T) {
	for said, rest := range map[string]string{
		"continue":                      "",
		"Continue!":                     "",
		"  go on  ":                     "",
		"carry on.":                     "",
		"Keep going?":                   "",
		"continue, but post at noon":    "but post at noon",
		"go on and post it at noon":     "and post it at noon",
		"Carry on: lead with the date.": "lead with the date.",
	} {
		got, ok := contract.CarryOn(said)
		if !ok {
			t.Errorf("%q was not read as picking the put-down task up", said)
		}
		if got != rest {
			t.Errorf("%q hands back %q as what the person said beyond the word, want %q", said, got, rest)
		}
	}
	for _, said := range []string{"stop", "", "continued", "continuously post", "the continue key is broken", "post it and carry on"} {
		if _, ok := contract.CarryOn(said); ok {
			t.Errorf("%q was read as picking the put-down task up, and it does not begin with one of the words", said)
		}
	}
}
