package loop_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// FuzzTheReplyHandler throws any reply text and any tool call at the loop. The
// loop must never panic on something the model wrote, and it must always reach
// an end state rather than hanging: every wait it makes is on the fake clock,
// which no test ever moves here.
func FuzzTheReplyHandler(f *testing.F) {
	f.Add("I will read it.", "read", `{"path":"notes.md"}`)
	f.Add("", "", "")
	f.Add("<tool_call>{\"name\": \"read\"}</tool_call>", "task", `{"why":"because"}`)
	f.Add("Shall I go on?", "task", `{"doneWhen":["a -> b"]}`)
	f.Add("no calls at all", "not_a_tool", `[1,2,3]`)
	f.Add("<think>I should stop</think>done", "task", `{"decision":{"text":"go","reason":"why"}}`)

	f.Fuzz(func(t *testing.T, said string, name string, arguments string) {
		ctx, giveUp := context.WithTimeout(context.Background(), 30*time.Second)
		defer giveUp()

		built := newHarness(t, []testkit.Step{{
			Text:      said,
			ToolCalls: []contract.ToolCall{{ID: "c1", Name: name, Input: json.RawMessage(arguments)}},
			Finish:    contract.FinishToolCalls,
		}}, scriptedTool("read", "the notes"))

		outcome, err := built.loop.Run(ctx, built.task("do the thing"))
		if err != nil {
			return
		}
		switch outcome.Status {
		case contract.StatusDone, contract.StatusWaiting, contract.StatusStopped, contract.StatusFailed:
		default:
			t.Fatalf("the task ended in the state %q, which is not one a task can end in", outcome.Status)
		}
	})
}
