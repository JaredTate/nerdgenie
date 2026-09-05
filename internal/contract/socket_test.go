package contract_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

func TestEveryScreenMessageTypeIsFromTheScreenAndNotFromTheProgram(t *testing.T) {
	fromScreen := []contract.SocketMessageType{
		contract.SocketMessage,
		contract.SocketCommand,
		contract.SocketApprove,
		contract.SocketDeny,
		contract.SocketSecret,
		contract.SocketAttach,
		contract.SocketDetach,
		contract.SocketShow,
	}
	for _, kind := range fromScreen {
		if !kind.FromScreen() {
			t.Errorf("%q is not marked as coming from the screen, and it should be", kind)
		}
		if kind.FromProgram() {
			t.Errorf("%q is marked as coming from the program, and it should not be", kind)
		}
	}
}

func TestEveryProgramMessageTypeIsFromTheProgramAndNotFromTheScreen(t *testing.T) {
	fromProgram := []contract.SocketMessageType{
		contract.SocketDelta,
		contract.SocketReply,
		contract.SocketPreview,
		contract.SocketAsk,
		contract.SocketHandoff,
		contract.SocketStatus,
		contract.SocketError,
		contract.SocketShown,
	}
	for _, kind := range fromProgram {
		if !kind.FromProgram() {
			t.Errorf("%q is not marked as coming from the program, and it should be", kind)
		}
		if kind.FromScreen() {
			t.Errorf("%q is marked as coming from the screen, and it should not be", kind)
		}
	}
}

func TestAnUnknownMessageTypeBelongsToNeitherSide(t *testing.T) {
	unknown := contract.SocketMessageType("shout")
	if unknown.FromScreen() || unknown.FromProgram() {
		t.Error("an unknown socket message type was accepted by one of the two sides, and it should be accepted by neither")
	}
}

func TestASocketEnvelopeSurvivesEncodingAndDecoding(t *testing.T) {
	original := contract.SocketEnvelope{
		Type:        contract.SocketPreview,
		ID:          "3",
		TaskID:      "17",
		Text:        "post the draft to x.com",
		Attachments: []string{"/tmp/shot.png"},
		Reason:      "spending money is on the ask-me-first list",
	}

	var written bytes.Buffer
	if err := contract.EncodeSocketEnvelope(&written, original); err != nil {
		t.Fatalf("encoding the envelope failed: %v", err)
	}
	if !strings.HasSuffix(written.String(), "\n") {
		t.Error("the encoded envelope does not end in a newline, and the socket is newline delimited")
	}
	if strings.Count(written.String(), "\n") != 1 {
		t.Errorf("the encoded envelope holds %d newlines, want exactly one", strings.Count(written.String(), "\n"))
	}

	decoded, err := contract.DecodeSocketEnvelope(written.Bytes())
	if err != nil {
		t.Fatalf("decoding the envelope failed: %v", err)
	}
	if decoded.Type != original.Type || decoded.ID != original.ID || decoded.Text != original.Text {
		t.Errorf("the envelope came back as %+v, want %+v", decoded, original)
	}
	if len(decoded.Attachments) != 1 || decoded.Attachments[0] != original.Attachments[0] {
		t.Errorf("the attachments came back as %v, want %v", decoded.Attachments, original.Attachments)
	}
}

func TestDecodeSocketEnvelopeRefusesBadInput(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"empty input", ""},
		{"only whitespace", "   \n"},
		{"not json at all", "hello"},
		{"json that is not an object", "[1,2,3]"},
		{"an object with no type", `{"text":"hi"}`},
		{"an object with an unknown type", `{"type":"shout"}`},
		{"a truncated object", `{"type":"message"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := contract.DecodeSocketEnvelope([]byte(test.line)); err == nil {
				t.Fatalf("decoding %q was accepted, want an error naming what is wrong", test.line)
			}
		})
	}
}

func FuzzDecodeSocketEnvelope(f *testing.F) {
	seeds := []string{
		`{"type":"message","text":"hello"}`,
		`{"type":"delta","text":""}`,
		`{"type":`,
		"",
		"{\"type\":\"secret\",\"secret\":\"\x00\"}",
	}
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, line []byte) {
		envelope, err := contract.DecodeSocketEnvelope(line)
		if err != nil {
			return
		}
		if !envelope.Type.FromScreen() && !envelope.Type.FromProgram() {
			t.Fatalf("decoding accepted the unknown message type %q", envelope.Type)
		}
	})
}

func TestTheStatusMessageFieldsAndStateWordsAreNamedOnce(t *testing.T) {
	fields := []string{
		contract.StatusFieldModel, contract.StatusFieldTask, contract.StatusFieldTaskState,
		contract.StatusFieldTokensIn, contract.StatusFieldTokensOut, contract.StatusFieldCost,
		contract.StatusFieldBudget, contract.StatusFieldState, contract.StatusFieldTool,
		contract.StatusFieldToolLine, contract.StatusFieldCommands, contract.StatusFieldHealthy,
	}
	seen := map[string]bool{}
	for _, field := range fields {
		if field == "" || seen[field] {
			t.Errorf("the status field %q is empty or repeated", field)
		}
		seen[field] = true
	}
	for _, state := range []string{contract.StateIdle, contract.StateThinking, contract.StateUsingTool, contract.StateWaitingForYou, contract.StatePaused} {
		if !contract.KnownScreenState(state) {
			t.Errorf("the state word %q is not known", state)
		}
	}
	if contract.KnownScreenState("dancing") {
		t.Error("an unknown state word was accepted")
	}
	if contract.StatusCommandSeparator != "\t" {
		t.Errorf("the command list separates a name from its help with %q, want a tab", contract.StatusCommandSeparator)
	}
}

func TestAScreenCanCancelAPromptAndAPreviewAnswerCarriesItsReason(t *testing.T) {
	if !contract.SocketCancel.FromScreen() || contract.SocketCancel.FromProgram() {
		t.Error("cancel must be a message a screen sends and the program never does")
	}
	answer := contract.PreviewAnswerWithReason{Answer: contract.AnswerReject, Reason: "not on that account"}
	if answer.Answer != contract.AnswerReject || answer.Reason != "not on that account" {
		t.Errorf("a preview answer did not hold its reason: %+v", answer)
	}
}

func TestAReplyCanTellTheScreenToClearItsTranscript(t *testing.T) {
	sent := contract.SocketEnvelope{Type: contract.SocketReply, Clear: true, Text: "cleared: the next message starts a fresh task"}
	line := bytes.Buffer{}
	if err := contract.EncodeSocketEnvelope(&line, sent); err != nil {
		t.Fatalf("encoding the reply failed: %v", err)
	}
	read, err := contract.DecodeSocketEnvelope(line.Bytes())
	if err != nil {
		t.Fatalf("decoding the reply failed: %v", err)
	}
	if !read.Clear {
		t.Error("the reply lost the word that tells the screen to empty its transcript")
	}
	if strings.Contains(line.String(), "clear") == false {
		t.Errorf("the line %q does not carry the clear field by name", line.String())
	}
	plain := bytes.Buffer{}
	if err := contract.EncodeSocketEnvelope(&plain, contract.SocketEnvelope{Type: contract.SocketReply, Text: "hello"}); err != nil {
		t.Fatalf("encoding a plain reply failed: %v", err)
	}
	if strings.Contains(plain.String(), "clear") {
		t.Errorf("a plain reply %q carries the clear field, and a field that is off is left out of the line", plain.String())
	}
}
