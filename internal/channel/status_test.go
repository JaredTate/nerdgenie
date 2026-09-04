package channel

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theCommandList is what the palette is filled from: one command per line, its
// name and its help line apart.
const theCommandList = "help" + contract.StatusCommandSeparator + "lists the commands"

// streamThatReportsStatus is a stream that has something to say about itself,
// which is what the program's own stream has and what a screen needs to fill
// its header, its status strip, its health dot, and its command palette.
func streamThatReportsStatus(t *testing.T) (*Stream, *testkit.FakeClock) {
	t.Helper()
	clock := testkit.NewFakeClock(arrived)
	stream := NewStream(StreamOptions{
		Clock: clock,
		Status: func() map[string]string {
			return map[string]string{
				contract.StatusFieldState:    contract.StateIdle,
				contract.StatusFieldCommands: theCommandList,
			}
		},
	})
	t.Cleanup(stream.Close)
	return stream, clock
}

// nextEvent reads the next event a reader was sent and fails the test rather
// than hanging when none arrives.
func nextEvent(t *testing.T, reader *Subscription) contract.SocketEnvelope {
	t.Helper()
	select {
	case got, open := <-reader.Events():
		if !open {
			t.Fatal("the reader's stream closed before the event arrived")
		}
		return got
	case <-time.After(aReadWait):
		t.Fatalf("nothing reached the reader within %s", aReadWait)
		return contract.SocketEnvelope{}
	}
}

func TestAScreenIsSentAStatusTheMomentItAttaches(t *testing.T) {
	stream, _ := streamThatReportsStatus(t)

	// Without this the palette is empty and the health dot is hollow until the
	// program happens to publish something of its own.
	got := nextEvent(t, mustSubscribe(t, stream))
	if got.Type != contract.SocketStatus {
		t.Fatalf("the screen was sent a %s the moment it attached, want a status", got.Type)
	}
	if got.Fields[contract.StatusFieldCommands] != theCommandList {
		t.Errorf("the status carries the command list %q, want %q", got.Fields[contract.StatusFieldCommands], theCommandList)
	}
	if got.Fields[contract.StatusFieldState] != contract.StateIdle {
		t.Errorf("the status says the agent is %q, want %q", got.Fields[contract.StatusFieldState], contract.StateIdle)
	}
}

func TestAStatusGoesOutOnEveryHeartbeat(t *testing.T) {
	stream, clock := streamThatReportsStatus(t)
	reader := mustSubscribe(t, stream)
	if got := nextEvent(t, reader); got.Type != contract.SocketStatus {
		t.Fatalf("the screen was sent a %s the moment it attached, want a status", got.Type)
	}

	// A screen calls the program gone when no status has arrived for ten
	// seconds, so the stream sends one of its own accord every five.
	for beat := range 2 {
		waitForSleepers(t, clock, 1)
		clock.Advance(StatusHeartbeat)
		if got := nextEvent(t, reader); got.Type != contract.SocketStatus {
			t.Fatalf("heartbeat %d sent a %s, want a status", beat+1, got.Type)
		}
	}
}

func TestAStreamWithNothingToSayAboutItselfSendsNoStatusAtAll(t *testing.T) {
	stream := NewStream(StreamOptions{})
	defer stream.Close()

	reader := mustSubscribe(t, stream)
	select {
	case got, open := <-reader.Events():
		t.Errorf("a stream with no status to report sent %+v (still open: %t)", got, open)
	default:
	}
}

func TestTheStreamRefusesAStatusWhoseStateWordIsNotOneOfTheFive(t *testing.T) {
	stream := NewStream(StreamOptions{})
	defer stream.Close()
	reader := mustSubscribe(t, stream)

	written := contract.SocketEnvelope{
		Type: contract.SocketStatus,
		Fields: map[string]string{
			contract.StatusFieldModel:    "local",
			contract.StatusFieldTask:     "t17",
			contract.StatusFieldState:    contract.StateUsingTool,
			contract.StatusFieldTool:     contract.ToolShell,
			contract.StatusFieldCommands: "status" + contract.StatusCommandSeparator + "shows what is going on",
		},
	}
	if err := stream.Publish(written); err != nil {
		t.Fatalf("publishing a status written in the contract's own words failed: %v", err)
	}
	if got := <-reader.Events(); got.Fields[contract.StatusFieldState] != contract.StateUsingTool {
		t.Errorf("the reader saw the state %q, want %q", got.Fields[contract.StatusFieldState], contract.StateUsingTool)
	}

	misspelt := contract.SocketEnvelope{
		Type:   contract.SocketStatus,
		Fields: map[string]string{contract.StatusFieldState: "busy"},
	}
	err := stream.Publish(misspelt)
	if err == nil {
		t.Fatal("the stream carried a status whose state word no screen knows, and the screen would have had nothing to show")
	}
	if !strings.Contains(err.Error(), "busy") {
		t.Errorf("the error reads %q, and it has to name the word it did not know", err)
	}
	select {
	case got := <-reader.Events():
		t.Errorf("the reader was sent the refused status anyway: %+v", got)
	default:
	}
}

func TestAStatusMayCarryAsFewOrAsManyFieldsAsTheProgramHas(t *testing.T) {
	stream := NewStream(StreamOptions{})
	defer stream.Close()
	reader := mustSubscribe(t, stream)

	// A screen ignores a field it does not know, so the stream must not stand in
	// the way of a field a later wave adds.
	if err := stream.Publish(contract.SocketEnvelope{
		Type:   contract.SocketStatus,
		Fields: map[string]string{contract.StatusFieldHealthy: "true", "somethingNewer": "1"},
	}); err != nil {
		t.Fatalf("publishing a status with no state word failed: %v", err)
	}
	if got := <-reader.Events(); got.Fields[contract.StatusFieldHealthy] != "true" {
		t.Errorf("the reader saw %+v, want the health field", got.Fields)
	}
}

func TestAStatusGoesOutOverTheSocketWithItsFieldsIntact(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)

	if err := harness.stream.Publish(contract.SocketEnvelope{
		Type: contract.SocketStatus,
		Fields: map[string]string{
			contract.StatusFieldModel: "local",
			contract.StatusFieldState: contract.StateWaitingForYou,
		},
	}); err != nil {
		t.Fatalf("publishing the status failed: %v", err)
	}

	got := client.next()
	if got.Type != contract.SocketStatus {
		t.Fatalf("the screen saw a %s, want a status", got.Type)
	}
	if got.Fields[contract.StatusFieldModel] != "local" {
		t.Errorf("the status reached the screen with the model %q, want %q", got.Fields[contract.StatusFieldModel], "local")
	}
	if got.Fields[contract.StatusFieldState] != contract.StateWaitingForYou {
		t.Errorf("the status reached the screen with the state %q, want %q", got.Fields[contract.StatusFieldState], contract.StateWaitingForYou)
	}
}
