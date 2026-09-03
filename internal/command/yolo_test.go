package command_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/command"
	"github.com/JaredTate/coeus/internal/contract"
)

// TestStatusSaysWhetherYoloIsOn holds the half of the yolo switch a person sees
// without asking for it: the status report says the switch is on, in words
// that say what that means and how to turn it off, and says it is off when it
// is off, so that nobody has to remember whether they typed "/yolo" an hour ago.
func TestStatusSaysWhetherYoloIsOn(t *testing.T) {
	on := runOne(t, command.Deps{
		Settings: contract.DefaultConfig(),
		YoloIsOn: func() bool { return true },
	}, "/status")
	if !strings.Contains(on, "yolo: on") {
		t.Errorf("the status report does not say yolo is on:\n%s", on)
	}
	if !strings.Contains(on, "/yolo off") {
		t.Errorf("the status report does not say how to turn yolo off:\n%s", on)
	}

	off := runOne(t, command.Deps{
		Settings: contract.DefaultConfig(),
		YoloIsOn: func() bool { return false },
	}, "/status")
	if !strings.Contains(off, "yolo: off") {
		t.Errorf("the status report does not say yolo is off:\n%s", off)
	}
}

// TestStatusSaysNothingAboutYoloWhenNothingCanSwitchIt holds that a build with
// no switch wired in prints no line about it, which is what the golden files
// for the status report hold.
func TestStatusSaysNothingAboutYoloWhenNothingCanSwitchIt(t *testing.T) {
	answer := runOne(t, command.Deps{Settings: contract.DefaultConfig()}, "/status")

	if strings.Contains(answer, "yolo") {
		t.Errorf("the status report mentions yolo with nothing wired up to switch it:\n%s", answer)
	}
}
