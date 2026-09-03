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

// TestTheRecordLineStatusFieldIsNamed pins the status field that carries one
// line for the latest change to the record, so the screen can show a task or a
// job being created, started, finished, or failed as it happens.
func TestTheRecordLineStatusFieldIsNamed(t *testing.T) {
	if StatusFieldRecordLine != "recordLine" {
		t.Errorf("status field is %q, want %q", StatusFieldRecordLine, "recordLine")
	}
}

// TestADeltaCanWithdrawThePartialReply pins the field a delta carries when the
// call behind it is being tried again: the screen throws away what it has
// shown of the reply so far, so a retry never draws the answer twice.
func TestADeltaCanWithdrawThePartialReply(t *testing.T) {
	envelope := SocketEnvelope{Type: SocketDelta, Reset: true}
	if !envelope.Reset {
		t.Errorf("the reset field was not kept")
	}
}

// TestTheReplyLabelIsTheWordTheModelNames pins the label a done line names as
// its result when the answer to the user is its own proof.
func TestTheReplyLabelIsTheWordTheModelNames(t *testing.T) {
	if ReplyLabel != "reply" {
		t.Errorf("the reply label is %q, want %q", ReplyLabel, "reply")
	}
}
