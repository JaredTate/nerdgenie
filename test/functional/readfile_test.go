// The whole-program test that proves the turn loop is really wired into "coeus
// serve": the scripted model asks for the read tool, the agent runs it against a
// file in the folder it is allowed to work in, and the reply that comes back
// over the socket carries what the file says.
package functional

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// whatTheFileSays is written into the work folder before the agent starts and
// has to come back in the agent's reply, which is only possible if the read tool
// really ran.
const whatTheFileSays = "the kettle is on the third shelf."

// theAskThatNeedsAFileRead is what the user types, and what the first step of
// the script expects to find in the request.
const theAskThatNeedsAFileRead = "Read the file and tell me what it says."

func TestTheAgentRunsTheReadToolAndTheReplyCarriesWhatTheFileSays(t *testing.T) {
	agent := startTheAgentWorkingIn(t, theReadingScriptFor)
	writeTheFileToRead(t, agent.work)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAskThatNeedsAFileRead})

	reply := screen.waitFor(t, contract.SocketReply, 60*time.Second)
	if !strings.Contains(reply.Text, whatTheFileSays) {
		t.Errorf("the reply was %q, and it does not carry what the file says", reply.Text)
	}
}

// writeTheFileToRead puts the file the script names into the folder the agent
// may work in. It is written after the agent has started, because the folder is
// the one the script was built from and nothing reads it until the model asks.
func writeTheFileToRead(t *testing.T, work string) {
	t.Helper()
	if err := os.WriteFile(theFileIn(work), []byte(whatTheFileSays+"\n"), contract.DataFileMode); err != nil {
		t.Fatalf("writing the file the agent is asked to read failed: %v", err)
	}
}

// theFileIn is the whole path of the file the script asks the read tool for.
func theFileIn(work string) string { return filepath.Join(work, "note.txt") }

// theReadingScriptFor is one whole task in three model calls: ask for the file
// and write the done list, point the done line at the result, and report what
// the file said. It is the same shape as the record in design section 4: a
// record begins on the first tool call and no task closes with a done line that
// has nothing behind it.
func theReadingScriptFor(work string) testkit.Script {
	return testkit.Script{Name: "local", ContextLength: 32768, Steps: []testkit.Step{
		{
			Expect: []string{theAskThatNeedsAFileRead, "You are the reasoning engine inside Coeus"},
			Text:   "Nothing has been read yet. I will open the file the user means.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{
				{ID: "call-read", Name: contract.ToolRead, Input: theReadInputFor(work)},
				{ID: "call-task", Name: contract.ToolTask, Input: json.RawMessage(
					`{"why":"the user wants to know what the file says","doneWhen":["the file has been read"]}`)},
			},
			Usage: contract.Usage{InputTokens: 400, OutputTokens: 20},
		},
		{
			Text:   "The file is open. I will point the done line at the result that proves it.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{{ID: "call-task-again", Name: contract.ToolTask, Input: json.RawMessage(
				`{"doneWhen":[{"text":"the file has been read","done":true,"resultId":"r1"}]}`)}},
			Usage: contract.Usage{InputTokens: 500, OutputTokens: 20},
		},
		{
			Text:   "What it says: " + whatTheFileSays + " What is left: nothing.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 600, OutputTokens: 20},
		},
	}}
}

// theReadInputFor is the arguments the scripted model writes for the read tool:
// the whole path of the file in the folder the agent may work in.
func theReadInputFor(work string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"path":%q}`, theFileIn(work)))
}
