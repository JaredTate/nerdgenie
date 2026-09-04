package tui

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// aRecordLine is a status carrying one line about the latest change to the
// record, which is what the program sends when a task or a job is created,
// started, finished, or fails.
func aRecordLine(line string) contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldRecordLine: line,
	}}
}

// pillTexts is the text of every pill in the transcript, in the order they were
// put there.
func pillTexts(screen *Screen) []string {
	said := []string{}
	for _, one := range screen.blocks {
		if one.kind == blockTool {
			said = append(said, one.text)
		}
	}
	return said
}

func TestARecordLineIsShownAsOnePill(t *testing.T) {
	screen, _ := screenWithLink()
	screen.Update(linkMessage{up: true})
	send(screen, aRecordLine("task 3 started · Build the game"))

	if said := pillTexts(screen); len(said) != 1 || said[0] != "task 3 started · Build the game" {
		t.Fatalf("a record line drew the pills %q, and it is one pill saying what changed", said)
	}
	drawn := plainText(strings.Join(screen.blockLines(screen.blocks[0]), "\n"))
	if !strings.Contains(drawn, string(toolArrowGlyph)+" task 3 started · Build the game") {
		t.Errorf("the record line is drawn as %q, and it is drawn exactly as a tool line is", drawn)
	}
}

func TestTheSameRecordLineTwiceDrawsOnePill(t *testing.T) {
	screen, _ := screenWithLink()
	screen.Update(linkMessage{up: true})
	for range 4 {
		send(screen, aRecordLine("task 3 started · Build the game"))
	}

	if said := pillTexts(screen); len(said) != 1 {
		t.Errorf("the same record line on four heartbeats drew the pills %q, and the record changed once", said)
	}
}

func TestEachChangeToTheRecordGetsItsOwnPill(t *testing.T) {
	screen, _ := screenWithLink()
	screen.Update(linkMessage{up: true})
	send(screen, aRecordLine("task 3 started · Build the game"))
	send(screen, aRecordLine("task 3 started · Build the game"))
	send(screen, aRecordLine("task 3 finished · Build the game"))
	send(screen, aRecordLine("job 8 created · Post the weekly note"))

	wanted := []string{
		"task 3 started · Build the game",
		"task 3 finished · Build the game",
		"job 8 created · Post the weekly note",
	}
	said := pillTexts(screen)
	if len(said) != len(wanted) {
		t.Fatalf("the record drew the pills %q, and each change gets one: %q", said, wanted)
	}
	for at, one := range wanted {
		if said[at] != one {
			t.Errorf("pill %d says %q, and it should say %q", at+1, said[at], one)
		}
	}
}

func TestARecordLineOfNothingIsNotDrawnAtAll(t *testing.T) {
	screen, _ := screenWithLink()
	screen.Update(linkMessage{up: true})
	send(screen, aRecordLine(""))

	if said := pillTexts(screen); len(said) != 0 {
		t.Errorf("an empty record line drew the pills %q, and there was nothing to say", said)
	}
}
