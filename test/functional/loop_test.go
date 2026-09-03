package functional

// This file is the functional test for the turn loop: a message goes in through
// a channel, the model asks for a tool, the real permission function shows the
// user exactly what is about to happen, the tool runs, the record is written
// into the real event log on disk, and the user gets the report. Everything
// outside the agent is a fake; everything inside it is the real thing. From
// brief 3.2 onward the same assertions run over the local socket.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/log"
	"github.com/JaredTate/coeus/internal/loop"
	"github.com/JaredTate/coeus/internal/permission"
	"github.com/JaredTate/coeus/internal/record"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theTaskTheUserAsksFor is the message the user sends in every test here.
const theTaskTheUserAsksFor = "clear the build folder and tell me what you did"

// TestAMessageBecomesATaskWithARecordOnDisk drives one whole task: the user
// asks, the model reads a file and writes the record, the user approves the one
// call on the ask-me-first list, and the record ends up in the event log with
// every done line pointing at the result that proves it.
func TestAMessageBecomesATaskWithARecordOnDisk(t *testing.T) {
	ctx, giveUp := context.WithTimeout(context.Background(), 30*time.Second)
	defer giveUp()

	home := testkit.NewTempHome(t)
	eventLog, err := log.Open(ctx, home.DatabaseFile())
	if err != nil {
		t.Fatalf("cannot open the event log: %v", err)
	}
	t.Cleanup(func() {
		if err := eventLog.Close(); err != nil {
			t.Errorf("cannot close the event log: %v", err)
		}
	})

	channel := testkit.NewFakeChannel("terminal")
	channel.AnswerPreviewsWith(contract.AnswerOnce)
	turns, shell := buildTheAgent(t, eventLog, channel)

	outcome, err := turns.Run(ctx, loop.Task{
		Message: contract.Inbound{ID: "m1", Sender: "the user", Text: theTaskTheUserAsksFor, Channel: "terminal"},
		Channel: channel,
	})
	if err != nil {
		t.Fatalf("the agent could not run the task: %v", err)
	}

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done: %s", outcome.Status, outcome.Report)
	}
	if len(shell.Inputs()) != 1 {
		t.Errorf("the command ran %d times, want the one the user approved", len(shell.Inputs()))
	}
	if shown := channel.Previews(); len(shown) != 1 || !strings.Contains(shown[0].Body, "rm -rf") {
		t.Fatalf("the user was shown %v, want one preview of the whole command", shown)
	}
	if sent := channel.Sent(); len(sent) != 1 || !strings.Contains(sent[0], "What changed") {
		t.Errorf("the user was sent %v, want one report of what changed", sent)
	}
	checkTheRecordOnDisk(ctx, t, eventLog, outcome.TaskID)
}

// checkTheRecordOnDisk reads the task back out of the event log the way the
// agent would after a restart, days later.
func checkTheRecordOnDisk(ctx context.Context, t *testing.T, eventLog contract.Store, taskID string) {
	t.Helper()
	keeper, err := record.Load(ctx, eventLog, contract.RecordTask, taskID)
	if err != nil {
		t.Fatalf("cannot load task %s back out of the log: %v", taskID, err)
	}
	held := keeper.Record()
	if held.Goal.Ask != theTaskTheUserAsksFor {
		t.Errorf("the ask reads %q, and the user wrote %q", held.Goal.Ask, theTaskTheUserAsksFor)
	}
	if err := record.DoneCheck(held); err != nil {
		t.Errorf("the record that closed does not pass the done-check: %v", err)
	}
	text, err := keeper.Read(ctx, contract.ResultID(1))
	if err != nil {
		t.Fatalf("cannot read the first result back: %v", err)
	}
	if !strings.Contains(text, "the build folder is empty") {
		t.Errorf("the first result reads back as %q, and the tool returned something else", text)
	}
}

// TestRefusingThePreviewStopsTheCommandAndTheModelIsTold proves the user's no
// reaches both the machine and the model.
func TestRefusingThePreviewStopsTheCommandAndTheModelIsTold(t *testing.T) {
	ctx, giveUp := context.WithTimeout(context.Background(), 30*time.Second)
	defer giveUp()

	home := testkit.NewTempHome(t)
	eventLog, err := log.Open(ctx, home.DatabaseFile())
	if err != nil {
		t.Fatalf("cannot open the event log: %v", err)
	}
	t.Cleanup(func() {
		if err := eventLog.Close(); err != nil {
			t.Errorf("cannot close the event log: %v", err)
		}
	})

	channel := testkit.NewFakeChannel("terminal")
	channel.AnswerPreviewsWith(contract.AnswerReject)
	turns, shell := buildTheAgent(t, eventLog, channel)

	if _, err := turns.Run(ctx, loop.Task{
		Message: contract.Inbound{ID: "m1", Sender: "the user", Text: theTaskTheUserAsksFor, Channel: "terminal"},
		Channel: channel,
	}); err != nil {
		t.Fatalf("the agent could not run the task: %v", err)
	}

	if len(shell.Inputs()) != 0 {
		t.Errorf("the command ran %d times after the user refused it", len(shell.Inputs()))
	}
	rulings, err := eventLog.ByKind(ctx, contract.EventPermissionDecision)
	if err != nil {
		t.Fatalf("cannot read the rulings out of the log: %v", err)
	}
	if len(rulings) == 0 {
		t.Error("no permission decision reached the log, and every ruling is written down")
	}
}

// buildTheAgent wires the real permission function, the real record, and the
// real event log into the loop, with a scripted model and scripted tools around
// them.
func buildTheAgent(t *testing.T, eventLog contract.Store, channel contract.Channel) (*loop.Loop, *testkit.ScriptedTool) {
	t.Helper()
	clock := testkit.NewFakeClock(time.Date(2026, time.January, 10, 9, 0, 0, 0, time.UTC))
	decider, err := permission.New(contract.DefaultConfig(), clock)
	if err != nil {
		t.Fatalf("cannot build the permission function: %v", err)
	}
	shell := testkit.NewScriptedTool(contract.ToolSpec{
		Name:        contract.ToolShell,
		Description: "Runs a command in the sandbox and hands back what it printed.",
		Classes:     []contract.PermissionClass{contract.ClassExecute},
	}, "the build folder is empty now")
	tools := testkit.NewFakeToolRegistry(shell, testkit.NewScriptedTool(contract.ToolSpec{
		Name:        contract.ToolTask,
		Description: "Update the task record: the why, the done list, the stop list, the plan, a decision, or a failure.",
		Classes:     []contract.PermissionClass{contract.ClassWrite},
	}))

	turns, err := loop.New(loop.Options{
		Model:      testkit.NewFakeModel(theScriptTheModelPlays()),
		Tools:      tools,
		Permission: decider,
		Store:      eventLog,
		Clock:      clock,
		Context:    loop.NewPlainBuilder(),
	})
	if err != nil {
		t.Fatalf("cannot build the turn loop: %v", err)
	}
	return turns, shell
}

// theScriptTheModelPlays is one whole task: write the done list and clear the
// folder, point the done line at the result, and report.
func theScriptTheModelPlays() testkit.Script {
	return testkit.Script{Name: "functional", ContextLength: 24000, Steps: []testkit.Step{
		{
			Text:   "Nothing has been done yet. I will clear the build folder.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{
				{ID: "c1", Name: contract.ToolShell, Input: json.RawMessage(`{"command":"rm -rf /home/jared/build"}`)},
				{ID: "c1t", Name: contract.ToolTask, Input: json.RawMessage(
					`{"why":"the user wants the build folder cleared","doneWhen":["the build folder is empty"]}`)},
			},
		},
		{
			Text:   "The folder is clear. I will point the done line at the result.",
			Finish: contract.FinishToolCalls,
			ToolCalls: []contract.ToolCall{{ID: "c2t", Name: contract.ToolTask, Input: json.RawMessage(
				`{"doneWhen":[{"text":"the build folder is empty","done":true,"resultId":"r1"}]}`)}},
		},
		{
			Text:   "What changed: the build folder is empty. What I checked: the command's output. What is left: nothing.",
			Finish: contract.FinishEnd,
		},
	}}
}
