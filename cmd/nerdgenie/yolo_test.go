package main

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// runTheYoloCommand runs "/yolo" with the arguments given and returns what it
// answered, failing when it refused and the test did not expect it to.
func runTheYoloCommand(t *testing.T, running *agent, arguments string) string {
	t.Helper()
	answer, err := running.yoloCommand().Run(context.Background(), arguments, contract.CommandContext{Channel: running.userChannel()})
	if err != nil {
		t.Fatalf("running /yolo %q failed: %v", arguments, err)
	}
	return answer
}

// TestTheYoloCommandOnItsOwnTurnsYoloOnAndSaysSo holds what a person sees when
// they type "/yolo" with nothing after it: the switch is on from then on, and
// the answer says so and says how to turn it off.
func TestTheYoloCommandOnItsOwnTurnsYoloOnAndSaysSo(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()
	if running.decider.YoloIsOn() {
		t.Fatal("yolo is on before anybody typed /yolo, and a session starts with it off")
	}

	answer := runTheYoloCommand(t, running, "")

	if !running.decider.YoloIsOn() {
		t.Error("/yolo did not turn yolo on in the permission function")
	}
	if !strings.Contains(answer, "yolo is on") {
		t.Errorf("/yolo does not say yolo is on:\n%s", answer)
	}
	if !strings.Contains(answer, "/yolo off") {
		t.Errorf("/yolo does not say how to turn it off again:\n%s", answer)
	}
	if again := runTheYoloCommand(t, running, "on"); !strings.Contains(again, "yolo is on") {
		t.Errorf("/yolo on does not say yolo is on:\n%s", again)
	}
}

// TestTheYoloCommandOffTurnsYoloOffAndSaysSo holds the other half: "/yolo off"
// brings the asking back, and the answer says so.
func TestTheYoloCommandOffTurnsYoloOffAndSaysSo(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()
	runTheYoloCommand(t, running, "")

	answer := runTheYoloCommand(t, running, "  OFF ")

	if running.decider.YoloIsOn() {
		t.Error("/yolo off left yolo on in the permission function")
	}
	if !strings.Contains(answer, "yolo is off") {
		t.Errorf("/yolo off does not say yolo is off:\n%s", answer)
	}
}

// TestTheYoloCommandRefusesAWordItDoesNotKnow holds that a word other than on
// or off is refused with the two words a person can type, and the switch is
// left where it was.
func TestTheYoloCommandRefusesAWordItDoesNotKnow(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	_, err := running.yoloCommand().Run(context.Background(), "sideways", contract.CommandContext{Channel: running.userChannel()})

	if err == nil {
		t.Fatal("/yolo sideways was accepted, and the command takes only on and off")
	}
	for _, word := range []string{"on", "off"} {
		if !strings.Contains(err.Error(), word) {
			t.Errorf("the refusal is %q, and it leaves out the word %q a person could type instead", err, word)
		}
	}
	if running.decider.YoloIsOn() {
		t.Error("a refused word turned yolo on, and nothing should have changed")
	}
}

// TestTheStatusCommandSaysWhetherYoloIsOn holds that the switch shows up where
// a person looks for the state of the session, before and after "/yolo".
func TestTheStatusCommandSaysWhetherYoloIsOn(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	before := runTheStatusCommand(t, running)
	if !strings.Contains(before, "yolo: off") {
		t.Errorf("/status does not say yolo is off on a fresh session:\n%s", before)
	}

	runTheYoloCommand(t, running, "")

	after := runTheStatusCommand(t, running)
	if !strings.Contains(after, "yolo: on") {
		t.Errorf("/status does not say yolo is on after /yolo:\n%s", after)
	}
}

// runTheStatusCommand runs "/status" through the one registry, the way a typed
// line reaches it, and returns the report.
func runTheStatusCommand(t *testing.T, running *agent) string {
	t.Helper()
	answer, err := running.registry.Run(context.Background(), "/status", contract.CommandContext{Channel: running.userChannel()})
	if err != nil {
		t.Fatalf("running /status failed: %v", err)
	}
	return answer
}

// TestTheYoloCommandIsRegisteredWithAHelpLine holds that the command is in the
// one registry, so that "/help" and the palette both show it.
func TestTheYoloCommandIsRegisteredWithAHelpLine(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	found, there := running.registry.Lookup(yoloName)
	if !there {
		t.Fatalf("/%s is not registered, and the person asked for it", yoloName)
	}
	if found.Help == "" {
		t.Errorf("/%s has no help line, and the palette shows one for every command", yoloName)
	}
}
