// The whole-program tests for what a person sees while tools run: a pill on the
// strip for every tool call and its result, a reply to a read-only command while
// the model is busy, and a real time on every event in the log. All three came
// out of driving the real agent on the local model.
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

// theFileTheToolReads is the file the scripted model asks the read tool for, so
// that a real tool really runs and a real result label is handed out.
const theFileTheToolReads = "note.txt"

// aTaskThatReadsAFile is one whole task in three calls: read the file and open
// the record, point the done line at the result, and report.
func aTaskThatReadsAFile(work string) testkit.Script {
	path := filepath.Join(work, theFileTheToolReads)
	return testkit.Script{Name: "local", ContextLength: 32768, Steps: []testkit.Step{
		{
			Text:   "I will open the file.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{
				{ID: "call-read", Name: contract.ToolRead, Input: json.RawMessage(`{"path":"` + path + `"}`)},
				{ID: "call-task", Name: contract.ToolTask, Input: json.RawMessage(
					`{"why":"the user wants what the file says","doneWhen":["the file has been read"]}`)},
			},
			Usage: contract.Usage{InputTokens: 400, OutputTokens: 20},
		},
		{
			Text:   "I will point the done line at the result.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{{ID: "call-task-again", Name: contract.ToolTask, Input: json.RawMessage(
				`{"doneWhen":[{"text":"the file has been read","done":true,"resultId":"r1"}]}`)}},
			Usage: contract.Usage{InputTokens: 500, OutputTokens: 20},
		},
		{
			Text:   "What it says: the kettle is on the third shelf. What is left: nothing.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 600, OutputTokens: 20},
		},
	}}
}

func TestEveryToolCallAndItsResultShowOnTheStrip(t *testing.T) {
	agent := startTheAgentWorkingIn(t, aTaskThatReadsAFile)
	writeTheNote(t, agent.work)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "what does the note say?"})

	starting := screen.waitForStatusCarrying(t, contract.StatusFieldToolLine, 60*time.Second)
	line := starting.Fields[contract.StatusFieldToolLine]
	if !strings.HasPrefix(line, toolLineMark) {
		t.Errorf("the tool line is %q, and the design writes it beginning with %q", line, toolLineMark)
	}
	if !strings.Contains(line, contract.ToolRead) {
		t.Errorf("the tool line is %q, and it does not name the tool that is running", line)
	}

	finished := screen.waitForStatusWhere(t, 60*time.Second, func(fields map[string]string) bool {
		return strings.Contains(fields[contract.StatusFieldToolLine], " r")
	})
	if !strings.Contains(finished.Fields[contract.StatusFieldToolLine], "r1") {
		t.Errorf("the tool line for a finished call is %q, and it does not carry the result's own label",
			finished.Fields[contract.StatusFieldToolLine])
	}
}

// toolLineMark is what the design puts in front of a tool line on the strip.
const toolLineMark = "▸"

// writeTheNote puts the file the scripted model asks for into the folder the
// agent may work in.
func writeTheNote(t *testing.T, work string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(work, theFileTheToolReads),
		[]byte("the kettle is on the third shelf.\n"), contract.DataFileMode); err != nil {
		t.Fatalf("writing the note failed: %v", err)
	}
}
