package tui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/testkit"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// aCallInProgress is the status the program sends on every heartbeat while a
// model call is running: that it is thinking, when the call began, and how many
// tokens have come back so far.
func aCallInProgress(began time.Time, streamed int) contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldState:       contract.StateThinking,
		contract.StatusFieldCallStarted: began.Format(time.RFC3339),
		contract.StatusFieldStreamed:    strconv.Itoa(streamed),
	}}
}

// aScreenWatchingACall is a screen that has just been told a model call began.
func aScreenWatchingACall(t *testing.T) (*Screen, *testkit.FakeClock) {
	t.Helper()
	screen, clock := newTestScreen(80, 24)
	screen.Update(linkMessage{up: true})
	send(screen, aCallInProgress(startOfTest, 0))
	return screen, clock
}

func TestTheStripCountsTheSecondsAndTheTokensWhileTheModelThinks(t *testing.T) {
	screen, clock := aScreenWatchingACall(t)

	if strip := statusStrip(screen); !strings.Contains(strip, "thinking · 0 s · 0 tokens") {
		t.Errorf("the strip is %q at the start of a call, and it should say %q", strip, "thinking · 0 s · 0 tokens")
	}

	advance(screen, clock, 14*time.Second)
	send(screen, aCallInProgress(startOfTest, 212))
	if strip := statusStrip(screen); !strings.Contains(strip, "thinking · 14 s · 212 tokens") {
		t.Errorf("the strip is %q fourteen seconds into a call, and it should say %q", strip, "thinking · 14 s · 212 tokens")
	}
}

func TestTheSecondsAndTheTokensGoTheMomentTheCallEnds(t *testing.T) {
	screen, clock := aScreenWatchingACall(t)
	advance(screen, clock, 14*time.Second)
	send(screen, aCallInProgress(startOfTest, 212))

	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldState: contract.StateIdle,
	}})
	strip := statusStrip(screen)
	for _, gone := range []string{" s ", "tokens", "14"} {
		if strings.Contains(strip, gone) {
			t.Errorf("the strip is %q after the call ended, and it still holds %q from the call that is over", strip, gone)
		}
	}
	if !strings.Contains(strip, "idle") {
		t.Errorf("the strip is %q after the call ended, and it should say the program is idle", strip)
	}
}

func TestTheCountIsThereBeforeTheSpinnerIsAndStaysWhileTheSpinnerHolds(t *testing.T) {
	screen, clock := aScreenWatchingACall(t)

	advance(screen, clock, 400*time.Millisecond)
	if screen.spinnerShowing() {
		t.Fatal("the spinner appeared before its half second, so this test is not measuring what it means to")
	}
	if strip := statusStrip(screen); !strings.Contains(strip, "0 s") {
		t.Errorf("the strip is %q before the spinner is due, and the count does not wait for the spinner", strip)
	}

	advance(screen, clock, 600*time.Millisecond)
	if !screen.spinnerShowing() {
		t.Fatal("the spinner did not appear after a second of thinking")
	}
	if strip := statusStrip(screen); !strings.Contains(strip, "1 s") {
		t.Errorf("the strip is %q a second into the call, and the count carries on beside the spinner", strip)
	}
}

func TestAStartTimeTheScreenCannotReadIsNotDrawnAtAll(t *testing.T) {
	screen, _ := newTestScreen(80, 24)
	screen.Update(linkMessage{up: true})
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldState:       contract.StateThinking,
		contract.StatusFieldCallStarted: "some time on Tuesday",
		contract.StatusFieldStreamed:    "212",
	}})

	strip := statusStrip(screen)
	if !strings.Contains(strip, "thinking") {
		t.Errorf("the strip is %q, and it should still say the model is thinking", strip)
	}
	if strings.Contains(strip, " s ") || strings.Contains(strip, "tokens") {
		t.Errorf("the strip is %q, and a start time the screen could not read must not be counted from", strip)
	}
}
