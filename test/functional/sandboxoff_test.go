// The whole-program test for the sandbox setting, proved both ways. With the
// sandbox off, which is what a fresh install runs as, a shell command reads a
// file outside every sandbox root and the read tool reaches a file in the user's
// home; with the fence on, the same command and the same read are refused. The
// handful of paths that stay outside whichever way the setting is turned are
// proved too: the vault in the agent's own home folder is refused with the
// sandbox off.
//
// Nothing here believes the model. A scripted model says whatever it was told to
// say, so the proof is a file only the command could have written and the
// requests the agent itself sent, which carry what each tool really gave back.
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

// What the two fixture files say and where the command copies the first of them.
// Neither sentence can be written by the model: one is only in a file outside
// every root, and the other only reaches the model through the read tool.
const (
	whatTheFileOutsideEveryRootSays = "the spare key is under the mat"
	whatTheUsersOwnFileSays         = "the meeting moved to Thursday"
	whereTheCommandCopiesIt         = "reached.txt"
)

// theAskThatReachesAroundTheMachine is what the user types to start the task.
const theAskThatReachesAroundTheMachine = "read what is on this machine and tell me."

func TestWithTheSandboxOffTheAgentReachesOutsideEveryRootAndTheVaultStaysShut(t *testing.T) {
	for _, written := range []struct {
		what string
		line string
	}{
		{"a home that says nothing about the sandbox", ""},
		{"a home that says the sandbox is off", `sandbox = "off"`},
	} {
		t.Run(written.what, func(t *testing.T) {
			agent := startTheAgentWithTheSandboxSetTo(t, written.line)
			askAndWaitForTheReply(t, agent)

			if copied := whatTheCommandCopied(t, agent); !strings.Contains(copied, whatTheFileOutsideEveryRootSays) {
				t.Errorf("the command copied %q, want what the file outside every sandbox root says", copied)
			}
			told := strings.Join(everyPromptSent(agent), "\n")
			if !strings.Contains(told, whatTheUsersOwnFileSays) {
				t.Errorf("the read tool never gave back what the file in the user's home says, so it did not reach outside the roots")
			}
			if !strings.Contains(told, "leave it alone") || !strings.Contains(told, agent.home.VaultFile()) {
				t.Errorf("the read of %s was not refused, and the vault stays out of reach whichever way the sandbox is set",
					agent.home.VaultFile())
			}
		})
	}
}

func TestWithTheFenceOnTheSameCommandAndTheSameReadAreRefused(t *testing.T) {
	agent := startTheAgentWithTheSandboxSetTo(t, `sandbox = "fence"`)
	askAndWaitForTheReply(t, agent)

	if copied := whatTheCommandCopied(t, agent); strings.Contains(copied, whatTheFileOutsideEveryRootSays) {
		t.Errorf("the command inside the fence copied %q from outside every sandbox root", copied)
	}
	told := strings.Join(everyPromptSent(agent), "\n")
	if strings.Contains(told, whatTheUsersOwnFileSays) {
		t.Errorf("the read tool gave back a file in the user's home, and with the fence on it reaches only the sandbox roots")
	}
	if !strings.Contains(told, "outside every folder the agent may work in") {
		t.Errorf("the read outside the roots was not refused in the words that name the folders the agent may work in")
	}
}

// startTheAgentWithTheSandboxSetTo starts the agent over a home folder of its
// own with that one line added to its configuration, having first written the
// two fixture files the task reads: one outside every sandbox root and one in
// the user's home directory beside the agent's home folder.
//
// The home is made before the script, because the script names the vault inside
// it, which is why this test starts the agent through startTheServe rather than
// through startTheAgentWorkingIn.
func startTheAgentWithTheSandboxSetTo(t *testing.T, line string) runningAgent {
	t.Helper()
	work := aWorkFolder(t)
	userHome := t.TempDir()
	home := contract.NewHome(filepath.Join(userHome, contract.HomeFolderName))
	if err := os.MkdirAll(home.Root, contract.HomeFolderMode); err != nil {
		t.Fatalf("making the home folder failed: %v", err)
	}

	outside := filepath.Join(t.TempDir(), "outside-every-root.txt")
	writeTheFixtureFile(t, outside, whatTheFileOutsideEveryRootSays)
	inTheUsersHome := filepath.Join(userHome, "notes-from-the-user.txt")
	writeTheFixtureFile(t, inTheUsersHome, whatTheUsersOwnFileSays)

	model := testkit.NewFakeProviderServer(aTaskThatReachesAroundTheMachine(work, home, outside, inTheUsersHome))
	t.Cleanup(model.Close)
	writeTheConfiguration(t, home, model.Address()+"/v1", work)
	if line != "" {
		addSettingToTheHome(t, home, line)
	}
	return startTheServe(t, home, work, model, true)
}

// writeTheFixtureFile writes one line into one file for the task to find.
func writeTheFixtureFile(t *testing.T, path string, says string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(says+"\n"), contract.DataFileMode); err != nil {
		t.Fatalf("writing the fixture file %s failed: %v", path, err)
	}
}

// askAndWaitForTheReply sends the ask and waits for the shell call and the reply
// that ends the task, so that the files are looked at after the work is done.
func askAndWaitForTheReply(t *testing.T, agent runningAgent) {
	t.Helper()
	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAskThatReachesAroundTheMachine})

	screen.waitForStatusWhere(t, 90*time.Second, func(fields map[string]string) bool {
		return strings.Contains(fields[contract.StatusFieldToolLine], contract.ToolShell)
	})
	screen.waitFor(t, contract.SocketReply, 90*time.Second)
}

// whatTheCommandCopied is what the shell command wrote into the folder the agent
// works in, or nothing at all when it wrote no file.
func whatTheCommandCopied(t *testing.T, agent runningAgent) string {
	t.Helper()
	copied, err := os.ReadFile(filepath.Join(agent.work, whereTheCommandCopiesIt))
	if err != nil {
		t.Logf("the command wrote no file at all: %v", err)
		return ""
	}
	return string(copied)
}

// aTaskThatReachesAroundTheMachine is one whole task in three model calls: copy
// a file from outside every sandbox root, read a file in the user's home, read
// the vault in the agent's own home folder, open the record, point the done line
// at the result, and report.
func aTaskThatReachesAroundTheMachine(work string, home contract.Home, outside string, inTheUsersHome string) testkit.Script {
	command := "cat " + outside + " > " + filepath.Join(work, whereTheCommandCopiesIt)
	return testkit.Script{Name: "local", ContextLength: 32768, Steps: []testkit.Step{
		{
			Text:   "I will look at the three of them.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{
				{ID: "call-shell", Name: contract.ToolShell, Input: json.RawMessage(
					`{"command":` + quotedForJSON(command) + `}`)},
				{ID: "call-read-the-user", Name: contract.ToolRead, Input: json.RawMessage(
					`{"path":` + quotedForJSON(inTheUsersHome) + `}`)},
				{ID: "call-read-the-vault", Name: contract.ToolRead, Input: json.RawMessage(
					`{"path":` + quotedForJSON(home.VaultFile()) + `}`)},
				{ID: "call-task", Name: contract.ToolTask, Input: json.RawMessage(
					`{"why":"the user wants to know what is on this machine","doneWhen":["the files have been looked at"]}`)},
			},
			Usage: contract.Usage{InputTokens: 400, OutputTokens: 20},
		},
		{
			Text:   "I will point the done line at the result.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{{ID: "call-task-again", Name: contract.ToolTask, Input: json.RawMessage(
				`{"doneWhen":[{"text":"the files have been looked at","done":true,"resultId":"r1"}]}`)}},
			Usage: contract.Usage{InputTokens: 500, OutputTokens: 20},
		},
		{
			Text:   "I have looked at all three. What is left: nothing.",
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 600, OutputTokens: 20},
		},
	}}
}
