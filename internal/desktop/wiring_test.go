package desktop

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// Nothing in cmd/coeus builds this package yet, so the computer tool ships in
// every registry and refuses every call with "this tool has no desktop behind
// it". These tests walk the path the wiring will take, from the worker command
// through ProcessStart and New to a call and a close, so that the lines added to
// cmd/coeus/wiring.go are lines that are known to work. The worker here is a
// shell standing in for `node workers/desktop/main.js`, and nothing on this
// machine's screen is touched.

// standInWorker answers every request the way a healthy built worker answers
// health, so that a test can walk the whole path without Node and without a
// display. Requests are numbered from one in order, so counting them answers
// each with the number it asked under.
const standInWorker = `asked=0; while read line; do asked=$((asked+1)); ` +
	`printf '{"jsonrpc":"2.0","id":%d,"result":{"healthy":true,"driverVersion":"stand-in","display":"none"}}\n' "$asked"; done`

func TestTheDesktopComesUpFromThePiecesTheWiringWouldHandIt(t *testing.T) {
	lines := []string{}
	start, err := ProcessStart([]string{"sh", "-c", standInWorker}, func(format string, arguments ...any) {
		lines = append(lines, format)
	})
	if err != nil {
		t.Fatalf("the worker command the wiring would build was refused: %v", err)
	}

	// Everything below is what cmd/coeus already holds for the browser: the
	// user's channel, the permission decider, the system clock, and its own note
	// line. Nothing new has to be built for the desktop to be reachable.
	made, err := New(Options{
		Start:      start,
		Channel:    testkit.NewFakeChannel("terminal"),
		Permission: testkit.NewFakePermission(contract.RulingAllow),
		Clock:      testkit.NewFakeClock(time.Unix(0, 0).UTC()),
		Note:       func(format string, arguments ...any) { lines = append(lines, format) },
	})
	if err != nil {
		t.Fatalf("building the desktop from the pieces the wiring holds failed: %v", err)
	}
	t.Cleanup(func() {
		if err := made.Close(); err != nil {
			t.Errorf("closing the desktop failed: %v", err)
		}
	})

	ctx, done := context.WithTimeout(context.Background(), 10*time.Second)
	defer done()
	health, err := made.Health(ctx)

	if err != nil {
		t.Fatalf("asking the desktop whether it is healthy failed: %v", err)
	}
	if !health.Healthy || health.DriverVersion != "stand-in" {
		t.Errorf("the desktop reports %+v, want a healthy worker naming its driver", health)
	}
}

func TestAWorkerBundleThatIsNotThereIsReportedRatherThanCrashingTheAgent(t *testing.T) {
	// cmd/coeus switches the browser tools off with a line saying what to do when
	// its bundle is missing, and the desktop has to fail the same way: the agent
	// comes up, the computer tool refuses, and the line names the program.
	start, err := ProcessStart([]string{"no-such-program-anywhere", "workers/desktop/main.js"}, nil)
	if err != nil {
		t.Fatalf("building the start function failed: %v", err)
	}
	made, err := New(Options{
		Start:      start,
		Channel:    testkit.NewFakeChannel("terminal"),
		Permission: testkit.NewFakePermission(contract.RulingAllow),
		Clock:      testkit.NewFakeClock(time.Unix(0, 0).UTC()),
	})
	if err != nil {
		t.Fatalf("building the desktop failed: %v", err)
	}

	err = made.Launch(context.Background(), "zenity", "a window with a text box opens")

	if err == nil || !strings.Contains(err.Error(), "no-such-program-anywhere") {
		t.Fatalf("the error is %v, want one naming the program that could not be started", err)
	}
}
