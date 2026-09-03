package functional

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// TestTheUsersOwnToolsAreAskedOncePerServeNotOncePerTask is the wiring's half
// of brief 6.7's finding 24: a program in the tools folder is asked what it is
// once when the agent starts, and every task's registry shares the answer,
// rather than every task asking every program again.
func TestTheUsersOwnToolsAreAskedOncePerServeNotOncePerTask(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "asked.txt")
	agent := startTheAgentWorkingIn(t, twoTasksThatEachReadANote, func(home contract.Home, work string) {
		if err := os.MkdirAll(home.ToolsFolder(), contract.HomeFolderMode); err != nil {
			t.Fatalf("making the tools folder failed: %v", err)
		}
		program := "#!/bin/sh\nif [ \"$1\" = \"" + contract.UserToolDescribeFlag + "\" ]; then echo asked >> " + counter + "; fi\nexit 0\n"
		if err := os.WriteFile(filepath.Join(home.ToolsFolder(), "counter"), []byte(program), 0o755); err != nil {
			t.Fatalf("writing the user's tool failed: %v", err)
		}
		if err := os.WriteFile(filepath.Join(work, "note.txt"), []byte("a note\n"), 0o644); err != nil {
			t.Fatalf("writing the note failed: %v", err)
		}
	})
	screen := agent.attach(t)
	for _, ask := range []string{"read the note", "read the note again"} {
		screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: ask})
		screen.waitFor(t, contract.SocketReply, 60*time.Second)
	}

	written, _ := os.ReadFile(counter)
	if asked := strings.Count(string(written), "asked"); asked != 1 {
		t.Errorf("the user's tool was asked what it is %d times across two tasks, want once for the whole serve", asked)
	}
}

// twoTasksThatEachReadANote scripts two tasks in a row, each reading the note
// in the working folder and then finishing.
func twoTasksThatEachReadANote(work string) testkit.Script {
	input, _ := json.Marshal(map[string]string{"path": filepath.Join(work, "note.txt")})
	reading := testkit.Step{
		Text:      "I will read the note.",
		Finish:    contract.FinishToolCalls,
		ToolCalls: []contract.ToolCall{{ID: "c1", Name: contract.ToolRead, Input: input}},
		Usage:     contract.Usage{InputTokens: 100, OutputTokens: 10},
	}
	done := testkit.Step{Text: "The note says: a note.", Finish: contract.FinishEnd, Usage: contract.Usage{InputTokens: 120, OutputTokens: 10}}
	return testkit.Script{Name: "local", ContextLength: 32768, Steps: []testkit.Step{reading, done, reading, done}}
}
