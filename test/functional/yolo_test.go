// The whole-program test for "/yolo": a person types it in the terminal, asks
// for something whose shell command is on the ask-me-first list, and the
// command runs with no preview; then they type "/yolo off", ask again, and the
// preview is back and waits for their answer. Nothing here believes the model:
// the proof that the command ran is a folder only the command could have
// removed.
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

// The ask the person types to start each task, and the folder inside the
// agent's work folder that the scripted model removes with a recursive delete.
const (
	theAskToClearTheScratchFolder = "clear out the scratch folder"
	theScratchFolder              = "scratch"
)

func TestYoloRunsABulkDeleteWithNoPreviewAndYoloOffBringsThePreviewBack(t *testing.T) {
	agent := startTheAgentWorkingIn(t, aTaskThatClearsTheScratchFolderTwice)
	screen := agent.attach(t)

	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "yolo"})
	screen.waitForReplySaying(t, "yolo is on", 15*time.Second)

	makeTheScratchFolder(t, agent)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAskToClearTheScratchFolder})
	screen.waitUntil(t, 90*time.Second, "the reply after the delete", func(envelope contract.SocketEnvelope) bool {
		if envelope.Type == contract.SocketPreview {
			t.Fatalf("with yolo on, the agent still showed a preview: %s", envelope.Text)
		}
		return envelope.Type == contract.SocketReply
	})
	if theScratchFolderIsThere(agent) {
		t.Error("with yolo on, the delete never ran, and the whole point is that it runs without asking")
	}

	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "yolo off"})
	screen.waitForReplySaying(t, "yolo is off", 15*time.Second)

	makeTheScratchFolder(t, agent)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAskToClearTheScratchFolder})
	preview := screen.waitFor(t, contract.SocketPreview, 90*time.Second)
	if !strings.Contains(preview.Text, "rm -rf") {
		t.Errorf("the preview says %q, and the person has to see the whole command", preview.Text)
	}
	if !theScratchFolderIsThere(agent) {
		t.Error("the delete ran before the person answered the preview")
	}

	screen.send(t, contract.SocketEnvelope{Type: contract.SocketApprove, ID: preview.ID})
	screen.waitFor(t, contract.SocketReply, 90*time.Second)
	if theScratchFolderIsThere(agent) {
		t.Error("the delete did not run after the person approved the preview")
	}
}

// makeTheScratchFolder makes the folder the scripted model deletes, with one
// file in it so that only a recursive delete can remove it.
func makeTheScratchFolder(t *testing.T, agent runningAgent) {
	t.Helper()
	folder := filepath.Join(agent.work, theScratchFolder)
	if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
		t.Fatalf("making the scratch folder failed: %v", err)
	}
	writeTheFixtureFile(t, filepath.Join(folder, "leftover.txt"), "a file from last time")
}

// theScratchFolderIsThere says whether the folder still exists.
func theScratchFolderIsThere(agent runningAgent) bool {
	_, err := os.Stat(filepath.Join(agent.work, theScratchFolder))
	return err == nil
}

// aTaskThatClearsTheScratchFolderTwice is the script for two tasks with the
// same shape, one under yolo and one without it: a recursive delete of the
// scratch folder, and then the reply that ends the task.
func aTaskThatClearsTheScratchFolderTwice(work string) testkit.Script {
	command := "rm -rf " + filepath.Join(work, theScratchFolder)
	clearing := func(callID string) testkit.Step {
		return testkit.Step{
			Expect: []string{theAskToClearTheScratchFolder},
			Text:   "I will clear it out.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{{ID: callID, Name: contract.ToolShell, Input: json.RawMessage(
				`{"command":` + quotedForJSON(command) + `}`)}},
			Usage: contract.Usage{InputTokens: 400, OutputTokens: 20},
		}
	}
	done := testkit.Step{
		Text:   "The scratch folder is gone.",
		Finish: contract.FinishEnd,
		Usage:  contract.Usage{InputTokens: 500, OutputTokens: 12},
	}
	return testkit.Script{Name: "local", ContextLength: 32768, Steps: []testkit.Step{
		clearing("call-clear-under-yolo"), done,
		clearing("call-clear-with-a-preview"), done,
	}}
}
