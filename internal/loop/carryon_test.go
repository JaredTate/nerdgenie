package loop

import "testing"

// TestTheWordsThatCarryOnAreReadAsWholeMessages pins the few whole messages
// that pick a put-down job task up, and that anything longer is a new ask even
// when it begins with one of them. The list is the one cmd/nerdgenie reads for
// a person's own stopped task, so a change here is a change there.
func TestTheWordsThatCarryOnAreReadAsWholeMessages(t *testing.T) {
	for _, said := range []string{"continue", "Continue!", "  go on  ", "carry on.", "Keep going?"} {
		if !saysCarryOn(said) {
			t.Errorf("%q was not read as the person asking for the stopped task back", said)
		}
	}
	for _, said := range []string{"continue with the tweet", "stop", "", "go on and post it", "continued"} {
		if saysCarryOn(said) {
			t.Errorf("%q was read as the person asking for the stopped task back, and it is not the whole word", said)
		}
	}
	if len(theWordsThatCarryOn) != 4 {
		t.Errorf("there are %d words that carry on, and the four this package and cmd/nerdgenie agree on are continue, go on, carry on, and keep going",
			len(theWordsThatCarryOn))
	}
}
