// This file was written by "nerdgenie replay 1 --as-test". It replays the
// recording that sits beside it against the code as it stands now, and fails
// when the run no longer ends where it ended when it was recorded.
//
// The rulebook here allows every call, because a test that has to run on any
// machine cannot read this one's config.toml. A recorded task that had a call
// refused needs that rule written in below by hand.

package replays

import (
	"context"
	"os"
	"testing"
	"time"

	workingcontext "github.com/JaredTate/nerdgenie/internal/context"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/replay"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestTask1ReplaysTheSameWay(t *testing.T) {
	held, err := os.ReadFile("testdata/task-1.json")
	if err != nil {
		t.Fatalf("cannot read the recording beside this test: %v", err)
	}
	recording, err := replay.Decode(held)
	if err != nil {
		t.Fatalf("cannot read the recording beside this test: %v", err)
	}
	built, err := workingcontext.New(workingcontext.Options{
		Home:            testkit.NewTempHome(t),
		MemoryCaps:      contract.DefaultConfig().MemoryCaps,
		MaxOutputTokens: contract.DefaultConfig().Caps.OutputTokensPerCall,
	})
	if err != nil {
		t.Fatalf("cannot build the working context this replay needs: %v", err)
	}
	result, err := replay.RunRecording(context.Background(), replay.Options{
		Into:       testkit.NewFakeStore(),
		Context:    loop.TheWorkingContext(built),
		Permission: testkit.NewFakePermission(contract.RulingAllow),
		Clock:      testkit.NewFakeClock(time.Date(2026, time.January, 10, 9, 0, 0, 0, time.UTC)),
	}, recording)
	if err != nil {
		t.Fatalf("cannot replay task 1: %v", err)
	}
	if !result.Passed {
		t.Fatalf("task 1 no longer replays the way it was recorded:\n%s", result.Report)
	}
}
