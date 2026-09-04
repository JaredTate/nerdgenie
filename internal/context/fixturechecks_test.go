package context

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestNothingAboveTheCacheLineChangesAcrossTenTurns is the promise the whole
// layout rests on: an agent that rewrites the front of its prompt loses the
// provider's cache exactly when the prompt is biggest. Rounds two to eleven of
// the fixture write nothing into the record's goal or rules, so the bytes above
// the cache line have to be the same on all ten turns.
func TestNothingAboveTheCacheLineChangesAcrossTenTurns(t *testing.T) {
	run := newFixtureRun(t)
	run.playTo(t, 1)
	builder := newGoldenBuilder(t)

	first := ""
	for round := 2; round <= 11; round++ {
		run.playTo(t, round)
		request, err := builder.Build(t.Context(), run.input(24000))
		if err != nil {
			t.Fatalf("cannot build the working context at round %d: %v", round, err)
		}
		above := aboveTheCacheLine(request)
		if round == 2 {
			first = above
			continue
		}
		if above != first {
			t.Fatalf("what is above the cache line changed at round %d, and it must not move during a task", round)
		}
	}

	// The test would prove nothing if the top of the prompt never changed at
	// all, so this is the change it is supposed to notice: the user's correction
	// at round twelve goes into the record's rules, which are above the line.
	run.playTo(t, 12)
	request, err := builder.Build(t.Context(), run.input(24000))
	if err != nil {
		t.Fatalf("cannot build the working context at round 12: %v", err)
	}
	if aboveTheCacheLine(request) == first {
		t.Error("the user's correction at round twelve did not reach the top of the prompt")
	}
}

// TestTheUsersOwnWordsAreNeverRewritten proves the first assertion of the
// fixture on the built prompt rather than on the record alone: after forty
// rounds the ask and the correction are in front of the model, byte for byte as
// the user wrote them.
func TestTheUsersOwnWordsAreNeverRewritten(t *testing.T) {
	run := newFixtureRun(t)
	run.playTo(t, 40)
	request, err := newGoldenBuilder(t).Build(t.Context(), run.input(24000))
	if err != nil {
		t.Fatalf("cannot build the working context at round 40: %v", err)
	}

	if err := run.fixture.CheckAskAndCorrections(run.keeper.Record()); err != nil {
		t.Errorf("the record no longer holds the user's own words: %v", err)
	}
	whole := testkit.WholeRequestText(request)
	for _, wanted := range []string{run.fixture.Ask, run.fixture.Correction} {
		if !strings.Contains(whole, wanted) {
			t.Errorf("the prompt at round forty has lost %q", wanted)
		}
	}
}

// TestEveryResultThatLeftTheWindowIsStillReadable is the third assertion of the
// fixture, and the whole point of the library analogy: a book goes back on the
// shelf, its line stays on the notes page, and the call number brings it back.
func TestEveryResultThatLeftTheWindowIsStillReadable(t *testing.T) {
	run := newFixtureRun(t)
	run.playTo(t, 40)
	builder := newTestBuilder(t, Options{Home: roomyHome(t), MaxOutputTokens: 512, Boundary: goldenBoundary})

	request, err := builder.Build(t.Context(), run.input(3000))
	if err != nil {
		t.Fatalf("cannot build the working context at round 40: %v", err)
	}
	whole := testkit.WholeRequestText(request)

	left, held := 0, run.keeper.Record()
	for _, produced := range run.fixture.ToolResults() {
		if !strings.Contains(whole, produced.Text) {
			left++
		}
		if !holdsResultLine(held, produced.ID) {
			t.Errorf("the result %s has no line in the record, so nothing points at it any more", produced.ID)
		}
	}
	if left == 0 {
		t.Fatal("no result left the window, so nothing was proved about reading one back")
	}
	t.Logf("%d of the fixture's results have left the window and are read back from the log", left)

	readBack := func(id string) (string, error) { return run.keeper.Read(t.Context(), id) }
	if err := run.fixture.CheckResultsReadable(readBack); err != nil {
		t.Errorf("a result that left the window cannot be read back: %v", err)
	}
}

// TokensARealModelReadOfTheFixture is what the local Qwen, through the
// llama-server daemon on this machine, said it read when it was sent the
// forty-step fixture's prompt at round forty. It was measured by the live test
// in this package on 2 September 2026, and it is written down here so that the
// estimate can be checked against a real tokenizer on every run rather than only
// at a wave gate. When the live test reports a different number, this one
// changes with it.
const TokensARealModelReadOfTheFixture = 8013

// TestTheEstimateAgreesWithWhatTheModelReports proves the numbers this package
// counts tokens with are close enough to be worth sizing a window by. The count
// they are checked against was read off a real model, and the fake model here
// reports it as the input count of the call, so that the check runs on every
// commit and not only when the live suite does.
func TestTheEstimateAgreesWithWhatTheModelReports(t *testing.T) {
	run := newFixtureRun(t)
	run.playTo(t, 40)
	request, err := newGoldenBuilder(t).Build(t.Context(), run.input(262144))
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}

	model := testkit.NewFakeModel(testkit.Script{
		Name: "forty-step", ContextLength: 262144,
		Steps: []testkit.Step{{
			Text: "Reading the notes.", Finish: contract.FinishEnd,
			Usage: contract.Usage{InputTokens: TokensARealModelReadOfTheFixture, OutputTokens: 40},
		}},
	})
	reply, err := model.Send(t.Context(), request, nil)
	if err != nil {
		t.Fatalf("the fake model refused the request: %v", err)
	}

	estimated := EstimateRequestTokens(request)
	apart := estimated - reply.Usage.InputTokens
	if apart < 0 {
		apart = -apart
	}
	t.Logf("the estimate is %d tokens and a real model read %d, which is %d apart",
		estimated, reply.Usage.InputTokens, apart)
	if apart*5 > reply.Usage.InputTokens {
		t.Errorf("the estimate is %d and a real model read %d, which is more than a fifth apart",
			estimated, reply.Usage.InputTokens)
	}
	if estimated < reply.Usage.InputTokens {
		t.Errorf("the estimate is %d and a real model read %d, so the estimate runs low, and a low estimate builds a prompt the model refuses",
			estimated, reply.Usage.InputTokens)
	}
}

// holdsResultLine says whether the record still carries the one line standing
// for a result, which is what a reader follows back to the log.
func holdsResultLine(held contract.Record, id string) bool {
	for _, line := range held.Work.Results {
		if line.ID == id {
			return true
		}
	}
	return false
}
