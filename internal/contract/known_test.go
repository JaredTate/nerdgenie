package contract_test

import (
	"errors"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

func TestKnownPreviewAnswerAcceptsTheThreeAndNothingElse(t *testing.T) {
	for _, answer := range []contract.PreviewAnswer{contract.AnswerOnce, contract.AnswerAlways, contract.AnswerReject} {
		if !contract.KnownPreviewAnswer(answer) {
			t.Errorf("the answer %q was not recognised, and it is one of the three", answer)
		}
	}
	for _, answer := range []contract.PreviewAnswer{"", "maybe", "yes"} {
		if contract.KnownPreviewAnswer(answer) {
			t.Errorf("the answer %q was recognised, and it should not be", answer)
		}
	}
}

func TestKnownEventKindAcceptsTheEightTheLogWrites(t *testing.T) {
	kinds := []contract.EventKind{
		contract.EventMessage, contract.EventToolCall, contract.EventToolResult,
		contract.EventPermissionDecision, contract.EventFileChange,
		contract.EventRecordChange, contract.EventCheckpoint, contract.EventReply,
	}
	for _, kind := range kinds {
		if !contract.KnownEventKind(kind) {
			t.Errorf("the event kind %q was not recognised, and the log writes it", kind)
		}
	}
	for _, kind := range []contract.EventKind{"", "gossip"} {
		if contract.KnownEventKind(kind) {
			t.Errorf("the event kind %q was recognised, and it should not be", kind)
		}
	}
}

func TestKnownPermissionClassAcceptsTheFiveLetters(t *testing.T) {
	for _, class := range []contract.PermissionClass{
		contract.ClassRead, contract.ClassWrite, contract.ClassExecute,
		contract.ClassNetwork, contract.ClassIrreversible,
	} {
		if !contract.KnownPermissionClass(class) {
			t.Errorf("the permission class %q was not recognised, and it is one of the five", class)
		}
	}
	for _, class := range []contract.PermissionClass{"", "Q", "r"} {
		if contract.KnownPermissionClass(class) {
			t.Errorf("the permission class %q was recognised, and it should not be", class)
		}
	}
}

func TestARateLimitSaysHowLongToWait(t *testing.T) {
	limited := contract.RateLimitedError{RetryAfter: 30 * time.Second}

	var wrapped error = limited
	if !errors.As(wrapped, &contract.RateLimitedError{}) {
		t.Error("a rate-limit error cannot be recognised with errors.As, and the provider needs to recognise it")
	}
	if limited.Error() == "" {
		t.Error("the rate-limit error has no message, and it must say how long to wait")
	}
}

// failingWriter stands in for a socket that has already been closed.
type failingWriter struct{}

// Write always fails, the way a write to a closed socket does.
func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("the socket is closed, so reattach before sending anything else")
}

func TestEncodingToAClosedSocketReportsWhatWentWrong(t *testing.T) {
	err := contract.EncodeSocketEnvelope(failingWriter{}, contract.SocketEnvelope{Type: contract.SocketReply})
	if err == nil {
		t.Fatal("encoding to a closed socket was reported as a success, want an error")
	}
}

func TestDefaultHomeFailsPlainlyWithNoHomeDirectory(t *testing.T) {
	t.Setenv("HOME", "")

	if _, err := contract.DefaultHome(); err == nil {
		t.Fatal("finding the home folder with no HOME set was reported as a success, want an error saying to set HOME")
	}
}
