// The whole-program tests for the fence the shell tool runs inside: a command it
// runs can reach the network, and the browser profile the configuration names is
// never inside a folder the fence opens. The fence unshares the network unless
// it is told otherwise, so without this wiring the shell tool has none at all.
//
// Both tests assert on something the model cannot author. A scripted model can
// say any words asked of it, so a test that reads the reply proves nothing about
// what really ran; one reads a file only the command could have written, and the
// other reads the line the serve itself printed.
package functional

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// What the shell command writes when it reached the server, and the file it
// writes it into. Neither the words nor the file can be written by the model:
// only the command itself writes them, and only when curl came back with an
// answer.
const (
	whatTheCommandWrites = "the server answered"
	theProofFile         = "reached.txt"
)

// aTaskThatReachesTheNetwork is one whole task: run a command that has to open a
// connection to a server on this machine, point the done line at the result, and
// report. A fence that unshares the network leaves even the loopback interface
// down, so the command fails there and works here.
func aTaskThatReachesTheNetwork(address string) func(string) testkit.Script {
	return func(work string) testkit.Script {
		command := "curl --silent --max-time 5 " + address + " > /dev/null && echo '" +
			whatTheCommandWrites + "' > " + filepath.Join(work, theProofFile)
		return testkit.Script{Name: "local", ContextLength: 32768, Steps: []testkit.Step{
			{
				Text:   "I will run the command.",
				Finish: contract.FinishToolCalls,
				ToolCalls: []contract.ToolCall{
					{ID: "call-shell", Name: contract.ToolShell, Input: json.RawMessage(
						`{"command":` + quotedForJSON(command) + `}`)},
					{ID: "call-task", Name: contract.ToolTask, Input: json.RawMessage(
						`{"why":"the user wants to know whether the network is there","doneWhen":["the command has run"]}`)},
				},
				Usage: contract.Usage{InputTokens: 400, OutputTokens: 20},
			},
			{
				Text:   "I will point the done line at the result.",
				Finish: contract.FinishToolCalls,
				ToolCalls: []contract.ToolCall{{ID: "call-task-again", Name: contract.ToolTask, Input: json.RawMessage(
					`{"doneWhen":[{"text":"the command has run","done":true,"resultId":"r1"}]}`)}},
				Usage: contract.Usage{InputTokens: 500, OutputTokens: 20},
			},
			{
				Text:   "The command ran. What is left: nothing.",
				Finish: contract.FinishEnd,
				Usage:  contract.Usage{InputTokens: 600, OutputTokens: 20},
			},
		}}
	}
}

func TestAShellCommandInsideTheFenceCanReachTheNetwork(t *testing.T) {
	agent := startTheAgentWorkingIn(t, aTaskThatReachesTheNetwork(aServerToReach(t)))

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "can you reach the network?"})

	// The tool line says a shell call ran; the file says what it did. The line
	// is waited for so that the file is looked at after the command, and the
	// file is what the test believes, because a scripted model can say any
	// words and cannot write a file.
	screen.waitForStatusWhere(t, 90*time.Second, func(fields map[string]string) bool {
		return strings.Contains(fields[contract.StatusFieldToolLine], contract.ToolShell)
	})
	screen.waitFor(t, contract.SocketReply, 90*time.Second)

	written, err := os.ReadFile(filepath.Join(agent.work, theProofFile))
	if err != nil {
		t.Fatalf("the command inside the fence never reached the server, so it wrote nothing: %v", err)
	}
	if !strings.Contains(string(written), whatTheCommandWrites) {
		t.Errorf("the command wrote %q, want the words it writes only when the server answered", written)
	}
}

func TestTheBrowserProfileIsNeverInsideAFolderTheFenceOpens(t *testing.T) {
	agent := startTheAgentWithABrowserProfileInsideTheWorkFolder(t)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "readyz"})
	screen.waitFor(t, contract.SocketReply, 20*time.Second)

	said := whatItSaid(agent.saidPath)
	if !strings.Contains(said, "no sandbox could be built") {
		t.Errorf("the agent came up with a fence that holds the browser profile, and that profile is the agent's own logins:\n%s", said)
	}
}

// startTheAgentWithABrowserProfileInsideTheWorkFolder starts the agent with a
// configuration that puts the browser profile inside the one folder a command
// may reach, which is exactly what the fence must refuse.
func startTheAgentWithABrowserProfileInsideTheWorkFolder(t *testing.T) runningAgent {
	t.Helper()
	return startTheAgentWorkingIn(t, func(string) testkit.Script {
		return testkit.Script{Name: "local", ContextLength: 32768}
	}, func(home contract.Home, work string) {
		profile := filepath.Join(work, "browser-profile")
		if err := os.MkdirAll(profile, contract.HomeFolderMode); err != nil {
			t.Fatalf("making the browser profile failed: %v", err)
		}
		addSettingToTheHome(t, home, "browser_profile_path = "+quotedForJSON(profile))
	})
}

// aServerToReach is a web server on a loopback port of its own, which a command
// inside the fence can only reach if the fence kept the network.
func aServerToReach(t *testing.T) string {
	t.Helper()
	serving := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("here"))
	}))
	t.Cleanup(serving.Close)
	return serving.URL
}

// quotedForJSON writes one string as JSON, so that a command with quotation
// marks in it reaches the tool as it was written.
func quotedForJSON(text string) string {
	written, err := json.Marshal(text)
	if err != nil {
		return `""`
	}
	return string(written)
}
