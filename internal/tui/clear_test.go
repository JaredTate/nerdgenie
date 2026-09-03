// The screen's half of "/clear": a reply that carries Clear empties the
// transcript and whatever reply was still streaming, and leaves the header, the
// status strip and the side panel saying what they said, because those are
// drawn from what the program knows about itself and the person asked for a
// clean transcript, not a program that has forgotten its own state.
package tui

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// theClearedWords is what the program's "/clear" command answers with.
const theClearedWords = "cleared: the next message starts a fresh task"

// aClearingReply is the envelope the "/clear" command sends.
func aClearingReply() contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketReply, Clear: true, Text: theClearedWords}
}

// aBusyTrialScreen is the trial's screen part way through a conversation: a
// running task on the header and the panel, the person's message, a pill, a
// finished reply, and a second reply still on its way.
func aBusyTrialScreen() *Screen {
	screen := aTrialScreen()
	send(screen, aStatusWithAPlan())
	typeAndSend(screen, theHaikuMessage)
	send(screen, aToolLine("▸ write haiku.txt · r1 write: 3 lines"))
	send(screen, contract.SocketEnvelope{Type: contract.SocketReply, Text: "Salt on the wind."})
	send(screen, contract.SocketEnvelope{Type: contract.SocketDelta, Text: "A second reply on its way"})
	return screen
}

func TestAReplyCarryingClearEmptiesTheTranscriptAndThePendingReply(t *testing.T) {
	screen := aBusyTrialScreen()
	before := plainText(screen.frame())
	for _, shown := range []string{"haiku.txt", "Salt on the wind"} {
		if !strings.Contains(before, shown) {
			t.Fatalf("the frame does not hold %q before the clear, so the test is not set up:\n%s", shown, before)
		}
	}

	send(screen, aClearingReply())
	screen.Update(tickMessage{at: screen.now.Add(heartbeatInterval)})

	after := plainText(screen.frame())
	for _, gone := range []string{"haiku.txt", "Salt on the wind", "A second reply"} {
		if strings.Contains(after, gone) {
			t.Errorf("the frame still holds %q after the clear, and a clear empties the transcript and the reply on its way:\n%s", gone, after)
		}
	}
	if !strings.Contains(after, theClearedWords) {
		t.Errorf("the frame does not say %q after the clear, and the person is owed the one line that says what happened:\n%s", theClearedWords, after)
	}
}

func TestAClearKeepsTheHeaderTheStatusStripAndTheSidePanel(t *testing.T) {
	screen := aBusyTrialScreen()
	send(screen, aClearingReply())

	if header := plainText(headerOf(screen)); !strings.Contains(header, "opus") || !strings.Contains(header, "task 17 running") {
		t.Errorf("the header is %q after the clear, and it should still name the model and the running task", header)
	}
	if strip := statusStrip(screen); !strings.Contains(strip, "thinking") {
		t.Errorf("the status strip is %q after the clear, and it should still say what the program said last", strip)
	}
	panel := strings.Join(panelColumnOf(screen), "\n")
	for _, kept := range []string{"opus", "task 17 running", "the product notes are"} {
		if !strings.Contains(panel, kept) {
			t.Errorf("the side panel does not hold %q after the clear, and the panel is drawn from the status, not the transcript:\n%s", kept, panel)
		}
	}
}

func TestTheOldToolLineOnTheNextHeartbeatDoesNotRefillAClearedTranscript(t *testing.T) {
	screen := aBusyTrialScreen()
	send(screen, aClearingReply())

	// The program goes on sending the newest tool line on every heartbeat until
	// the call changes, and a screen that had forgotten it had drawn that line
	// would draw it again straight into the empty transcript.
	send(screen, aToolLine("▸ write haiku.txt · r1 write: 3 lines"))
	if frame := plainText(screen.frame()); strings.Contains(frame, "haiku.txt") {
		t.Errorf("the pill the screen had already drawn came back after the clear on the next heartbeat:\n%s", frame)
	}

	send(screen, aToolLine("▸ read haiku.txt"))
	if frame := plainText(screen.frame()); !strings.Contains(frame, "read haiku.txt") {
		t.Errorf("a new call after the clear drew no pill:\n%s", frame)
	}
}
