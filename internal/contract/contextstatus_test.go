package contract

import "testing"

// TestTheStatusFieldsForTheContextAndTheCallInProgressAreNamed pins the four
// status fields the screen needs to show how full the model's context is and
// that a call in progress is alive: the tokens the last call held, the window
// the model can hold, the tokens the running call has written so far, and when
// it began.
func TestTheStatusFieldsForTheContextAndTheCallInProgressAreNamed(t *testing.T) {
	named := map[string]string{
		StatusFieldContextTokens: "contextTokens",
		StatusFieldContextWindow: "contextWindow",
		StatusFieldStreamed:      "streamed",
		StatusFieldCallStarted:   "callStarted",
	}
	for field, want := range named {
		if field != want {
			t.Errorf("status field is %q, want %q", field, want)
		}
	}
}
