package context

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// theRoundsTheCacheIsMeasuredAt are the three rounds the wave 6 gate review
// measured finding 17 at, so that its numbers and these can be read side by side.
var theRoundsTheCacheIsMeasuredAt = []int{10, 20, 30}

// leastSharedPercent is how much of a prompt has to be the same as the last
// one's for the layout to be doing its job. The gate review asked for eighty per
// cent and measured thirty-three at round thirty before the tail was built.
const leastSharedPercent = 80

// TestMostOfEveryPromptIsWhatTheLastOneAlreadySaid is finding 17 of the wave 6
// gate review, pinned. The review built the fixture at every round and compared
// each prompt with the one before it: the shared run stopped growing at cache
// boundary C while the prompt went on growing, so the share fell from forty-eight
// per cent at round ten to thirty-three at round thirty and would have kept
// falling. Whatever changes on a call drags everything under it along with it,
// so the two parts that change on every call — the record's list of results and
// its budget and cost lines — sit at the tail, under the conversation.
func TestMostOfEveryPromptIsWhatTheLastOneAlreadySaid(t *testing.T) {
	for _, round := range theRoundsTheCacheIsMeasuredAt {
		run := newFixtureRun(t)
		builder := newGoldenBuilder(t)

		run.playTo(t, round-1)
		earlier := buildWithTheBudgetSpent(t, builder, run, 101-round,
			contract.CostLine{InputTokens: 15200, CachedInputTokens: 3900, OutputTokens: 500})
		run.playTo(t, round)
		later := buildWithTheBudgetSpent(t, builder, run, 100-round,
			contract.CostLine{InputTokens: 15400, CachedInputTokens: 3900, OutputTokens: 600})

		whole := renderPrompt(later)
		shared := sharedPrefix(renderPrompt(earlier), whole)
		share := 100 * len(shared) / len(whole)
		t.Logf("at round %d the prompt is %d characters and shares %d of them with round %d, which is %.1f per cent",
			round, len(whole), len(shared), round-1, 100*float64(len(shared))/float64(len(whole)))
		if share < leastSharedPercent {
			t.Errorf("at round %d only %d per cent of the prompt is what the last call already said, and the layout has to hold %d;"+
				" something that changes every call has got in above something that does not", round, share, leastSharedPercent)
		}
	}
}

// TestOneMemorySaveDoesNotMoveTheTopOfThePrompt is finding 18 of the wave 6 gate
// review. The three persona files are read from disk on every build, and the
// `memory` tool and the after-action review both write into two of them, so a
// single fact saved in the middle of a task used to rewrite the block that ends
// cache boundary A and cost the whole prompt under it. What the agent knows
// belongs below the cache line, where saving a fact costs only the lines after
// it; only SOUL.md, which nothing but the user writes, stays above.
func TestOneMemorySaveDoesNotMoveTheTopOfThePrompt(t *testing.T) {
	run := newFixtureRun(t)
	home := roomyHome(t)
	builder := newTestBuilder(t, Options{Home: home, MaxOutputTokens: contract.DefaultConfig().Caps.OutputTokensPerCall, Boundary: goldenBoundary})
	run.playTo(t, 20)

	before := renderPrompt(buildWithTheBudgetSpent(t, builder, run, 80,
		contract.CostLine{InputTokens: 15200, CachedInputTokens: 3900, OutputTokens: 500}))
	writePersonaFile(t, home.WorldFactsFile(),
		"DigiByte launched on the tenth of January 2014.\nA DigiByte block is mined about every fifteen seconds.")
	after := renderPrompt(buildWithTheBudgetSpent(t, builder, run, 80,
		contract.CostLine{InputTokens: 15200, CachedInputTokens: 3900, OutputTokens: 500}))

	if !strings.Contains(after, "about every fifteen seconds") {
		t.Fatal("the saved fact never reached the prompt, so this test is not measuring what it says it measures")
	}
	shared := sharedPrefix(before, after)
	share := 100 * len(shared) / len(after)
	t.Logf("one fact saved mid-task leaves %d characters of %d shared, which is %d per cent", len(shared), len(after), share)
	if share < leastSharedPercent {
		t.Errorf("saving one fact left only %d per cent of the prompt where it was, and the layout has to hold %d;"+
			" what the agent writes about itself is sitting above what it does not", share, leastSharedPercent)
	}
}

// TestThePromptTheProviderCanReuseReachesPastTheRecordBody is the layout half of
// the cache rule, and it came out of the first live run on the local model: every
// call read about fifteen thousand tokens and the provider reused only about
// thirty-nine hundred of them, so each turn re-read eleven thousand tokens and
// took forty seconds. The cause was where the record's header sat. The budget
// line and the cost line change on every single call, and they were the first
// thing under the cache line, ahead of the goal, the plan and the results, so
// nothing below the system prompt could ever be reused.
//
// This builds the same task on two rounds that follow each other and measures
// what the two prompts share from the first byte. That shared run is what a
// provider may reuse, and it has to reach the instructions, the tool
// descriptions and the whole record body down to the list of results, which only
// grows. The two lines that change every call belong at the very end, after
// every result.
func TestThePromptTheProviderCanReuseReachesPastTheRecordBody(t *testing.T) {
	run := newFixtureRun(t)
	builder := newGoldenBuilder(t)

	run.playTo(t, 20)
	earlier := buildWithTheBudgetSpent(t, builder, run, 81, contract.CostLine{InputTokens: 15200, CachedInputTokens: 3900, OutputTokens: 500})
	run.playTo(t, 21)
	later := buildWithTheBudgetSpent(t, builder, run, 80, contract.CostLine{InputTokens: 15400, CachedInputTokens: 3900, OutputTokens: 600})

	whole := renderPrompt(later)
	shared := sharedPrefix(renderPrompt(earlier), whole)
	t.Logf("two rounds that follow each other share %d bytes of %d from the start, so %d bytes are read again",
		len(shared), len(whole), len(whole)-len(shared))
	for _, wanted := range []string{
		InstructionText,
		"Read a file, a folder, or a past result by its id.",
		"Write a file inside the allowed folders.",
		"## Goal", "## Rules", "## Work", "Plan:", "- [ ] 10 confirm it is up",
		// The result of round twenty, which is the last thing the two rounds
		// have in common. Reaching it means the shared run covers the whole
		// conversation, not only the record's body.
		"e13 text 228/280 shows the post is inside the limit.",
	} {
		if !strings.Contains(shared, wanted) {
			t.Errorf("two rounds that follow each other share only %d bytes from the start, and that run does not reach %q,"+
				" so the provider has to read it again on every call", len(shared), firstLineOf(wanted))
		}
	}

	for name, request := range map[string]contract.Request{"round twenty": earlier, "round twenty-one": later} {
		last := request.Messages[len(request.Messages)-1]
		for _, wanted := range []string{"budget left:", "this turn:"} {
			if !strings.Contains(last.Text, wanted) {
				t.Errorf("at %s the %q line is not in the last message before the model speaks, so it sits in front of"+
					" something the provider could otherwise reuse", name, wanted)
			}
		}
		prompt := renderPrompt(request)
		if strings.LastIndex(prompt, resultListLabel) < strings.LastIndex(prompt, memoryHintHeading) {
			t.Errorf("at %s the list of results comes before the memory hint, and the list grows every round,"+
				" so everything under it is read again on every call", name)
		}
		if strings.LastIndex(prompt, "budget left:") < strings.LastIndex(prompt, resultListLabel) {
			t.Errorf("at %s the budget line comes before the list of results, and it changes on every call", name)
		}
	}
}

// buildWithTheBudgetSpent builds one turn's working context with the budget and
// the cost of that turn written into the record's header, which is what the turn
// loop hands over and what makes those two lines different on every call.
func buildWithTheBudgetSpent(t *testing.T, builder *Builder, run *fixtureRun, roundsLeft int, cost contract.CostLine) contract.Request {
	t.Helper()
	input := run.input(24000)
	input.Record.Header.RoundsLeft = roundsLeft
	input.Record.Header.MinutesLeft = roundsLeft / 2
	input.Record.Header.Cost = cost
	request, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the working context with %d rounds left: %v", roundsLeft, err)
	}
	return request
}

// sharedPrefix is the run of bytes two prompts have in common from the start,
// which is the most a provider can reuse between two calls.
func sharedPrefix(earlier string, later string) string {
	at := 0
	for at < len(earlier) && at < len(later) && earlier[at] == later[at] {
		at++
	}
	return earlier[:at]
}

// firstLineOf keeps a failure readable when what is missing is a whole block of
// text, such as the instructions.
func firstLineOf(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	if len(line) > 80 {
		return line[:80] + "..."
	}
	return line
}
