// The whole-program test for the headline claim: a result leaves the model's
// window and is still readable by its label. Finding 65 of the wave 6 gate says
// nothing checks that the wiring passes the record to the read tool, so "read
// r7" would fail with the fixture still green.
package functional

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// whatTheEarlierResultHeld is written into a file, read by the first tool call,
// and then asked for again by its label long after the text has left the window.
const whatTheEarlierResultHeld = "the seed catalogue is behind the flour"

// aTaskThatReadsBackAnEarlierResult is one whole task: read a file, then read
// the result of that read back by its own label, and report what it said.
func aTaskThatReadsBackAnEarlierResult(work string) testkit.Script {
	return testkit.Script{Name: "local", ContextLength: 32768, Steps: []testkit.Step{
		{
			Text:   "I will open the file.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{
				{ID: "call-read", Name: contract.ToolRead, Input: json.RawMessage(
					`{"path":` + quotedForJSON(filepath.Join(work, "catalogue.txt")) + `}`)},
				{ID: "call-task", Name: contract.ToolTask, Input: json.RawMessage(
					`{"why":"the user wants what the note says","doneWhen":["the note has been read"]}`)},
			},
			Usage: contract.Usage{InputTokens: 400, OutputTokens: 20},
		},
		{
			// This is the claim: the label alone brings the whole text back,
			// with nothing of it left in the window.
			Text:   "I will read that result back by its label.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{
				{ID: "call-readback", Name: contract.ToolRead, Input: json.RawMessage(`{"path":"r1"}`)},
			},
			Usage: contract.Usage{InputTokens: 500, OutputTokens: 20},
		},
		{
			Text:   "The note has been read. What is left: nothing.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 600, OutputTokens: 20},
		},
	}}
}

func TestAResultIsStillReadableByItsLabelThroughTheWiredAgent(t *testing.T) {
	agent := startTheAgentWorkingIn(t, aTaskThatReadsBackAnEarlierResult)
	if err := os.WriteFile(filepath.Join(agent.work, "catalogue.txt"),
		[]byte(whatTheEarlierResultHeld+"\n"), contract.DataFileMode); err != nil {
		t.Fatalf("writing the note failed: %v", err)
	}

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "what does the catalogue note say?"})
	screen.waitFor(t, contract.SocketReply, 90*time.Second)

	// The record holds the summary of every result, and the second read's own
	// summary is what the label brought back. A read that found no record says
	// so in its result instead.
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "tasks 1"})
	written := screen.waitFor(t, contract.SocketReply, 30*time.Second).Text

	if strings.Contains(written, "no record") || strings.Contains(written, "nothing has been run") {
		t.Errorf("reading a result back by its label found no record, so the read tool was built without one:\n%s", written)
	}
	// The third result is the read of "r1", and its summary is the text the
	// first read found. That text is nowhere in the model's window by then: the
	// label alone brought it back, which is the claim the whole design rests on.
	readBack := theResultLine(t, written, "r3")
	if !strings.Contains(readBack, whatTheEarlierResultHeld) {
		t.Errorf("reading r1 back by its label gave %q, want the words the first read found", readBack)
	}
	if strings.Contains(readBack, "refused") || strings.Contains(readBack, "no record") {
		t.Errorf("reading r1 back by its label was refused, so the wiring did not hand the record to the read tool: %q", readBack)
	}
}

// theResultLine is the one line of the record for the result with that label.
func theResultLine(t *testing.T, written string, label string) string {
	t.Helper()
	for _, line := range strings.Split(written, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- "+label+" ") {
			return strings.TrimSpace(line)
		}
	}
	t.Fatalf("the record holds no result called %s, so the read never happened:\n%s", label, written)
	return ""
}
