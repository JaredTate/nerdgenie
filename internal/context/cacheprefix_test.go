package context

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

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

	shared := sharedPrefix(renderPrompt(earlier), renderPrompt(later))
	t.Logf("two rounds that follow each other share %d bytes of %d from the start", len(shared), len(renderPrompt(later)))
	for _, wanted := range []string{
		InstructionText,
		"Read a file, a folder, or a past result by its id.",
		"Write a file inside the allowed folders.",
		"## Goal", "## Rules", "## Work", "Plan:", "- [ ] 10 confirm it is up",
		resultListLabel,
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
		if strings.LastIndex(prompt, "budget left:") < strings.LastIndex(prompt, resultListLabel) {
			t.Errorf("at %s the budget line comes before the list of results, and it changes on every call", name)
		}
		if strings.LastIndex(prompt, "budget left:") < strings.LastIndex(prompt, "--- end tool result,") {
			t.Errorf("at %s the budget line comes before the newest tool result, and it changes on every call", name)
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
