//go:build integration

// This file is the integration test for the permission function: it puts the
// permission function, a channel, and the real filesystem together through their
// real interfaces, in a temporary home, rather than in memory. It runs under the
// integration build tag, which is what "make test" and "make check" pass.
package permission_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/permission"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestAWriteThatEmptiesARealFileOnDiskAsksAndNamesItsRealSize(t *testing.T) {
	home := testkit.NewTempHome(t)
	big := filepath.Join(home.MemoryFolder(), "the-long-notes.md")
	writeFileOfSize(t, big, 12000)

	decider := newDecider(t, permission.DefaultSettings())

	decision := decide(t, decider, contract.PermissionRequest{
		ToolName: contract.ToolWrite,
		Input:    jsonInput(t, map[string]any{"path": big, "content": ""}),
	})

	if decision.Ruling != contract.RulingAsk {
		t.Fatalf("emptying a real file of 12000 bytes was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
	if !strings.Contains(decision.PreviewText, "12000") {
		t.Errorf("the preview is %q, and it has to name the size the file really is on disk", decision.PreviewText)
	}
}

func TestAWriteToARealSmallFileOnDiskJustRuns(t *testing.T) {
	home := testkit.NewTempHome(t)
	small := filepath.Join(home.MemoryFolder(), "the-short-notes.md")
	writeFileOfSize(t, small, 40)

	decider := newDecider(t, permission.DefaultSettings())

	decision := decide(t, decider, contract.PermissionRequest{
		ToolName: contract.ToolWrite,
		Input:    jsonInput(t, map[string]any{"path": small, "content": ""}),
	})

	if decision.Ruling != contract.RulingAllow {
		t.Errorf("emptying a real file of 40 bytes was ruled %q, want %q", decision.Ruling, contract.RulingAllow)
	}
}

func TestAFolderIsNotAFileAndSoIsNeverEmptiedByAWrite(t *testing.T) {
	home := testkit.NewTempHome(t)

	decider := newDecider(t, permission.DefaultSettings())

	decision := decide(t, decider, contract.PermissionRequest{
		ToolName: contract.ToolWrite,
		Input:    jsonInput(t, map[string]any{"path": home.MemoryFolder(), "content": ""}),
	})

	if decision.Ruling != contract.RulingAllow {
		t.Errorf("writing over a folder was ruled %q, want %q, because a folder has no size to empty", decision.Ruling, contract.RulingAllow)
	}
}

func TestTheWholeRoundThroughAChannelFromAskToAlwaysToNotAskingAgain(t *testing.T) {
	ctx := context.Background()
	channel := testkit.NewFakeChannel("terminal")
	channel.AnswerPreviewsWith(contract.AnswerAlways)
	decider := newDecider(t, permission.DefaultSettings())
	request := shellRequest(t, "rm -rf /tmp/the-build-folder")

	first := decide(t, decider, request)
	if first.Ruling != contract.RulingAsk {
		t.Fatalf("the first bulk delete was ruled %q, want %q", first.Ruling, contract.RulingAsk)
	}

	answer, err := channel.ShowPreview(ctx, contract.Preview{ID: "1", Title: first.Reason, Body: first.PreviewText})
	if err != nil {
		t.Fatalf("showing the preview on the channel failed: %v", err)
	}
	if err := decider.Remember(request, answer, ""); err != nil {
		t.Fatalf("remembering the user's answer failed: %v", err)
	}

	second := decide(t, decider, request)
	if second.Ruling != contract.RulingAllow {
		t.Errorf("after the user said always, the same call was ruled %q, want %q", second.Ruling, contract.RulingAllow)
	}
	if shown := channel.Previews(); len(shown) != 1 || !strings.Contains(shown[0].Body, "rm -rf /tmp/the-build-folder") {
		t.Errorf("the user was shown %v, want one preview carrying the whole command", shown)
	}
}

func TestAScheduledRunOnARealHomeStopsAndReportsInsteadOfWaiting(t *testing.T) {
	home := testkit.NewTempHome(t)
	big := filepath.Join(home.MemoryFolder(), "the-long-notes.md")
	writeFileOfSize(t, big, 12000)

	channel := testkit.NewFakeChannel("signal")
	decider := newDecider(t, permission.DefaultSettings())

	decision := decide(t, decider, contract.PermissionRequest{
		ToolName:   contract.ToolWrite,
		Input:      jsonInput(t, map[string]any{"path": big, "content": ""}),
		Unattended: true,
	})

	if !permission.StoppedForNobodyToAsk(decision) {
		t.Fatalf("a scheduled run was ruled %q with the preview %q, want the stop verdict", decision.Ruling, decision.PreviewText)
	}
	if len(channel.Previews()) != 0 {
		t.Errorf("a scheduled run showed the user %v, and there is nobody there to see it", channel.Previews())
	}
}

func TestAStandingApprovalRunsOutOnTheRealClock(t *testing.T) {
	decider, err := permission.New(permission.DefaultSettings(), realClock{})
	if err != nil {
		t.Fatalf("building the permission function on the real clock failed: %v", err)
	}
	registerStanding(t, decider, permission.StandingApproval{
		Skill:       "clear-the-build-folder",
		ReducedForm: "rm -rf",
		Limit:       5,
		Expires:     time.Now().Add(-time.Second),
	})

	decision := decide(t, decider, shellRequest(t, "rm -rf /tmp/x"))
	if decision.Ruling != contract.RulingAsk {
		t.Errorf("an approval that ran out a second ago was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
}

// realClock is the clock the integration test uses, because an expiry read
// against the real clock is the thing this test is about.
type realClock struct{}

// Now is the time this machine says it is.
func (realClock) Now() time.Time {
	return time.Now()
}

// Sleep waits, or returns early when the context is cancelled first.
func (realClock) Sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// NewTicker starts a ticker on the interval.
func (realClock) NewTicker(interval time.Duration) contract.Ticker {
	return realTicker{ticker: time.NewTicker(interval)}
}

// realTicker is a real ticker behind the contract's ticker.
type realTicker struct {
	ticker *time.Ticker
}

// Ticks is the channel the times arrive on.
func (ticking realTicker) Ticks() <-chan time.Time {
	return ticking.ticker.C
}

// Stop ends the ticker.
func (ticking realTicker) Stop() {
	ticking.ticker.Stop()
}
