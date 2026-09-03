//go:build integration

package context

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/log"
	"github.com/JaredTate/coeus/internal/record"
	"github.com/JaredTate/coeus/internal/testkit"
)

// TestTheFortyStepFixtureAgainstTheRealLog runs the whole of the fixture with
// the real SQLite file behind the record and the real persona files on disk,
// which is the arrangement the running program has. It proves the promise the
// design rests on end to end: after forty rounds the prompt is about the size it
// was at round ten, most of the results have left the window, and every one of
// them still reads back in full from the log.
func TestTheFortyStepFixtureAgainstTheRealLog(t *testing.T) {
	home := roomyHome(t)
	eventLog, err := log.Open(t.Context(), home.DatabaseFile())
	if err != nil {
		t.Fatalf("cannot open the event log at %s: %v", home.DatabaseFile(), err)
	}
	t.Cleanup(func() {
		if err := eventLog.Close(); err != nil {
			t.Errorf("cannot close the event log: %v", err)
		}
	})

	fixture, err := testkit.LoadFortyStepTask()
	if err != nil {
		t.Fatalf("cannot load the forty-step fixture: %v", err)
	}
	keeper, err := record.New(t.Context(), eventLog, record.Start{
		Kind: contract.RecordTask, ID: fixture.TaskID, Origin: fixture.Origin,
		Ask: fixture.Ask, RoundsLeft: 100, MinutesLeft: 60,
	})
	if err != nil {
		t.Fatalf("cannot open the fixture's task record on the real log: %v", err)
	}
	run := &fixtureRun{fixture: fixture, keeper: keeper}

	builder := newTestBuilder(t, Options{Home: home, MaxOutputTokens: 512, Boundary: goldenBoundary})
	run.playTo(t, 10)
	atRoundTen, err := builder.Build(t.Context(), run.input(3000))
	if err != nil {
		t.Fatalf("cannot build the working context at round ten: %v", err)
	}
	run.playTo(t, 40)
	atRoundForty, err := builder.Build(t.Context(), run.input(3000))
	if err != nil {
		t.Fatalf("cannot build the working context at round forty: %v", err)
	}

	ten, forty := EstimateRequestTokens(atRoundTen), EstimateRequestTokens(atRoundForty)
	t.Logf("the prompt is about %d tokens at round ten and about %d at round forty", ten, forty)
	if forty > ten*3/2 {
		t.Errorf("the prompt grew from %d tokens to %d over thirty rounds, and it should stay about the same", ten, forty)
	}

	whole := testkit.WholeRequestText(atRoundForty)
	left := 0
	for _, produced := range fixture.ToolResults() {
		if !strings.Contains(whole, produced.Text) {
			left++
		}
	}
	if left == 0 {
		t.Fatal("no result left the window, so nothing was proved about reading one back from the real log")
	}
	t.Logf("%d of the fixture's results have left the window", left)

	readBack := func(id string) (string, error) { return keeper.Read(t.Context(), id) }
	if err := fixture.CheckResultsReadable(readBack); err != nil {
		t.Errorf("a result cannot be read back out of the real log: %v", err)
	}
	if err := fixture.CheckAskAndCorrections(keeper.Record()); err != nil {
		t.Errorf("the user's own words did not survive forty rounds: %v", err)
	}
}
