package channel

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

func TestTheStreamRefusesAStatusWhoseStateWordIsNotOneOfTheFive(t *testing.T) {
	stream := NewStream()
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
	stream := NewStream()
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
