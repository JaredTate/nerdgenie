package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	workingcontext "github.com/JaredTate/nerdgenie/internal/context"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/log"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/replay"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aHomeWithARecordedTask gives the test its own home folder whose event log
// holds one finished task, which is what "nerdgenie replay" is pointed at.
func aHomeWithARecordedTask(t *testing.T) contract.Home {
	t.Helper()
	home := testkit.NewTempHome(t)
	ctx := context.Background()
	opened, err := log.Open(ctx, home.DatabaseFile())
	if err != nil {
		t.Fatalf("opening the event log failed: %v", err)
	}
	built, err := workingcontext.New(workingcontext.Options{
		Home:            home,
		MemoryCaps:      contract.DefaultConfig().MemoryCaps,
		MaxOutputTokens: contract.DefaultConfig().Caps.OutputTokensPerCall,
	})
	if err != nil {
		t.Fatalf("building the working context failed: %v", err)
	}
	made, err := loop.New(loop.Options{
		Model:      testkit.NewFakeModel(theScriptOfTheRecordedTask()),
		Tools:      testkit.NewFakeToolRegistry(theSearchToolOfTheRecordedTask()),
		Permission: testkit.NewFakePermission(contract.RulingAllow),
		Store:      opened,
		Clock:      testkit.NewFakeClock(time.Date(2026, time.January, 10, 9, 0, 0, 0, time.UTC)),
		Context:    loop.TheWorkingContext(built),
	})
	if err != nil {
		t.Fatalf("building the loop that records the task failed: %v", err)
	}
	if _, err := made.Run(ctx, loop.Task{
		Message: contract.Inbound{ID: "in-1", Text: "count the files in the notes folder", Channel: "terminal"},
		Channel: testkit.NewFakeChannel("terminal"),
	}); err != nil {
		t.Fatalf("the recorded run did not finish: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("closing the event log failed: %v", err)
	}
	return home
}

// theScriptOfTheRecordedTask is what the model says while the task is recorded.
func theScriptOfTheRecordedTask() testkit.Script {
	return testkit.Script{Name: "test", ContextLength: 24000, Steps: []testkit.Step{
		{
			Text: "I am starting on the notes folder.",
			ToolCalls: []contract.ToolCall{
				{ID: "c1", Name: "search", Input: json.RawMessage(`{"pattern":"*"}`)},
				{ID: "c2", Name: "task", Input: json.RawMessage(
					`{"why":"the user wants a count","done_when":[{"text":"the files are counted","done":true,"resultId":"r1"}]}`)},
			},
		},
		{Text: "There are three files in the notes folder."},
	}}
}

// theSearchToolOfTheRecordedTask is the one tool the recorded task calls.
func theSearchToolOfTheRecordedTask() contract.Tool {
	return testkit.NewScriptedTool(contract.ToolSpec{
		Name:        "search",
		Description: "A tool the test scripted, which answers with what the test gave it.",
		Classes:     []contract.PermissionClass{contract.ClassRead},
	}, "three files")
}

func TestTheReplaySubcommandReportsOnARecordedTask(t *testing.T) {
	aHomeWithARecordedTask(t)
	var output, problems bytes.Buffer

	code := replaySubcommand.run([]string{"1"}, &output, &problems)

	if code != contract.ExitOK {
		t.Fatalf("nerdgenie replay 1 left with %d rather than %d: %s%s", code, contract.ExitOK, output.String(), problems.String())
	}
	if !strings.Contains(output.String(), "reproduced the recording") {
		t.Errorf("nerdgenie replay 1 printed:\n%s", output.String())
	}
}

func TestTheReplaySubcommandWritesTheTestWhenItIsAsked(t *testing.T) {
	aHomeWithARecordedTask(t)
	into := t.TempDir()
	var output, problems bytes.Buffer

	code := replaySubcommand.run([]string{"--as-test", "--into", into, "1"}, &output, &problems)

	if code != contract.ExitOK {
		t.Fatalf("nerdgenie replay 1 --as-test left with %d: %s%s", code, output.String(), problems.String())
	}
	for _, one := range []string{"task_1_test.go", filepath.Join("testdata", "task-1.json")} {
		path := filepath.Join(into, filepath.FromSlash(replay.TestsFolder), one)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("nerdgenie replay --as-test wrote no %s: %v", path, err)
		}
	}
	if !strings.Contains(output.String(), "go test") {
		t.Errorf("nerdgenie replay --as-test did not say how to run what it wrote:\n%s", output.String())
	}
}

func TestTheReplaySubcommandSaysSoWhenThereIsNoSuchTask(t *testing.T) {
	testkit.NewTempHome(t)
	var output, problems bytes.Buffer

	code := replaySubcommand.run([]string{"41"}, &output, &problems)

	if code != contract.ExitFailure {
		t.Errorf("nerdgenie replay 41 left with %d rather than %d on a log with no such task", code, contract.ExitFailure)
	}
	if !strings.Contains(problems.String(), "41") {
		t.Errorf("nerdgenie replay 41 said %q and it must name the task it could not find", problems.String())
	}
}

func TestTheReplaySubcommandRefusesACommandLineItCannotRead(t *testing.T) {
	testkit.NewTempHome(t)
	wrong := map[string][]string{
		"no task at all":   nil,
		"two tasks":        {"17", "18"},
		"a flag it has no": {"--every-task", "17"},
	}
	for what, arguments := range wrong {
		t.Run(what, func(t *testing.T) {
			var output, problems bytes.Buffer
			if code := replaySubcommand.run(arguments, &output, &problems); code != contract.ExitUsage {
				t.Errorf("nerdgenie replay with %s left with %d rather than %d", what, code, contract.ExitUsage)
			}
		})
	}
}
