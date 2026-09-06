package loop

import "testing"

// TestWhereToNextIsAnOfferToCarryOn is the eleventh nightly run's node-tests
// task, which ended "Great Scott, a tidy little suite, no fixes needed. Where
// to next?" and waited. A person's own task waits on it as it should, and a
// job's task must not: "where to next" and "what next" are offers.
func TestWhereToNextIsAnOfferToCarryOn(t *testing.T) {
	for _, line := range []string{"Where to next?", "What next?", "What's next?", "Want me to keep rolling?"} {
		if !offersToCarryOn(line) {
			t.Errorf("%q is not read as an offer to carry on", line)
		}
	}
	for _, line := range []string{"Which folder should I use?", "Is the test file right?"} {
		if offersToCarryOn(line) {
			t.Errorf("%q is read as an offer to carry on, and it asks about the work", line)
		}
	}
}
