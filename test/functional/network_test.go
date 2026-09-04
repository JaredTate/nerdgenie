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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
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
	// The sandbox is off unless the configuration asks for the fence, and this
	// test is about the fence, so it asks.
	agent := startTheAgentWorkingIn(t, aTaskThatReachesTheNetwork(aServerToReach(t)),
		func(home contract.Home, _ string) { addSettingToTheHome(t, home, `sandbox = "fence"`) })

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
	work := aWorkFolder(t)
	model := testkit.NewFakeProviderServer(testkit.Script{Name: "local", ContextLength: 32768})
	t.Cleanup(model.Close)
	program := buildTheBinary(t)
	home := aHomePointingAt(t, model.Address()+"/v1", work)
	profile := filepath.Join(work, "browser-profile")
	if err := os.MkdirAll(profile, contract.HomeFolderMode); err != nil {
		t.Fatalf("making the browser profile failed: %v", err)
	}
	addSettingToTheHome(t, home, "browser_profile_path = "+quotedForJSON(profile))

	// The configuration check refuses the root before anything is built, so
	// the program must quit while reading its settings, name the setting, and
	// never open its socket. The bound is there so a program that comes up
	// anyway fails the test instead of hanging it.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	started := exec.CommandContext(ctx, program, "serve")
	started.Env = append(os.Environ(), "NERDGENIE_HOME="+home.Root)
	said, err := started.CombinedOutput()
	if err == nil {
		t.Fatalf("nerdgenie serve came up with the browser profile inside a sandbox root; it said:\n%s", said)
	}
	if !strings.Contains(string(said), "sandbox_roots") || !strings.Contains(string(said), profile) {
		t.Errorf("the refusal does not name the setting and the profile:\n%s", said)
	}
	if _, err := os.Stat(home.SocketFile()); err == nil {
		t.Errorf("the socket was opened even though the settings were refused")
	}
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
