// The whole-program test for the data boundary: the mark that says where a tool
// result begins and ends is made once per task, not once per program. Finding 20
// of the context review found one boundary for the whole life of a serve, so a
// page that learned it from one task could forge the mark in every later task
// and have its own words read as the harness speaking.
package functional

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theBoundaryInAPrompt finds the boundary identifier the harness wrapped a tool
// result in. It is written the way internal/context writes it, and it is sixteen
// characters of hexadecimal.
var theBoundaryInAPrompt = regexp.MustCompile(`boundary ([0-9a-f]{16})`)

// aTaskThatReadsAndReports is one whole task in two calls: read a file, which is
// what makes the harness wrap a result, and then report.
func aTaskThatReadsAndReports(work string) testkit.Script {
	read := contract.ToolCall{ID: "call-read", Name: contract.ToolRead, Input: json.RawMessage(
		`{"path":` + quotedForJSON(filepath.Join(work, "note.txt")) + `}`)}

	// The steps repeat, because a call the provider retries takes a step with
	// it and a task that is asked to try again takes several. Every odd step
	// asks for the file, so whichever step a task lands on, it wraps a result
	// and the boundary of that task appears in the prompt after it.
	steps := []testkit.Step{}
	for range 8 {
		steps = append(steps,
			testkit.Step{Text: "I will open it.", Finish: contract.FinishToolCalls,
				ToolCalls: []contract.ToolCall{read},
				Usage:     contract.Usage{InputTokens: 400, OutputTokens: 20}},
			testkit.Step{Text: "It says what it says. What is left: nothing.", Finish: contract.FinishEnd,
				Usage: contract.Usage{InputTokens: 500, OutputTokens: 20}})
	}
	return testkit.Script{Name: "local", ContextLength: 32768, Steps: steps}
}

func TestTwoTasksInOneServeGetDifferentDataBoundaries(t *testing.T) {
	agent := startTheAgentWorkingIn(t, aTaskThatReadsAndReports)
	if err := os.WriteFile(filepath.Join(agent.work, "note.txt"),
		[]byte("the kettle is on the third shelf\n"), contract.DataFileMode); err != nil {
		t.Fatalf("writing the note failed: %v", err)
	}

	screen := agent.attach(t)
	for _, ask := range []string{"what does the note say?", "read it again please"} {
		screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: ask})
		screen.waitFor(t, contract.SocketReply, 90*time.Second)
	}

	boundaries := theBoundariesTheAgentUsed(t, agent)
	if len(boundaries) < 2 {
		t.Fatalf("only %d boundary was used across two tasks, and each task must have one of its own: %v",
			len(boundaries), boundaries)
	}
}

// theBoundariesTheAgentUsed reads every prompt the agent sent and gives back the
// distinct boundary identifiers in them.
func theBoundariesTheAgentUsed(t *testing.T, agent runningAgent) []string {
	t.Helper()
	seen := map[string]bool{}
	distinct := []string{}
	for _, asked := range agent.model.Requests() {
		for _, found := range theBoundaryInAPrompt.FindAllStringSubmatch(string(asked.Body), -1) {
			if !seen[found[1]] {
				seen[found[1]] = true
				distinct = append(distinct, found[1])
			}
		}
	}
	if len(distinct) == 0 {
		t.Fatalf("no prompt carried a data boundary at all, so no tool result was ever marked as data:\n%s",
			strings.Join(everyPromptSent(agent), "\n---\n"))
	}
	return distinct
}

// everyPromptSent is the body of every request the model server was sent, for a
// failure message that shows what the agent really asked.
func everyPromptSent(agent runningAgent) []string {
	sent := []string{}
	for _, asked := range agent.model.Requests() {
		sent = append(sent, string(asked.Body))
	}
	return sent
}
