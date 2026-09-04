package context

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// goldenBoundary stands in for the random boundary a running task makes, so that
// the golden files do not change on every run.
const goldenBoundary = "0f1e2d3c4b5a6978"

// TestTheGoldenPromptsForASmallModelAndABigOne builds the same task on a 24k
// model and on a 200k one, at round twenty of the forty-step fixture, and writes
// both out. The two are the proof of the one rule: the layers above the cache
// line are the same bytes on both, and the only thing that changes between a
// small model and a big one is how many results are on the table.
func TestTheGoldenPromptsForASmallModelAndABigOne(t *testing.T) {
	run := newFixtureRun(t)
	run.playTo(t, 20)
	builder := newGoldenBuilder(t)

	small, err := builder.Build(t.Context(), run.input(24000))
	if err != nil {
		t.Fatalf("cannot build the working context for a 24k model: %v", err)
	}
	big, err := builder.Build(t.Context(), run.input(200000))
	if err != nil {
		t.Fatalf("cannot build the working context for a 200k model: %v", err)
	}

	testkit.Golden(t, "prompt-24k.txt", []byte(renderPrompt(small)))
	testkit.Golden(t, "prompt-200k.txt", []byte(renderPrompt(big)))
	t.Logf("the 24k prompt is about %d tokens and the 200k prompt about %d",
		EstimateRequestTokens(small), EstimateRequestTokens(big))

	if aboveTheCacheLine(small) != aboveTheCacheLine(big) {
		t.Error("the two models are sent different bytes above the cache line, and everything up there is the same on every model")
	}
	if err := onlyOlderMessagesLeft(small, big); err != nil {
		t.Errorf("the two prompts differ by more than how many results are on the table: %v", err)
	}
	if EstimateRequestTokens(small) > EstimateRequestTokens(big) {
		t.Error("the small model was sent more than the big one")
	}
}

// TestASmallerWindowDropsTheOldestResultsOfTheFixture proves the window rule
// bites on the fixture itself. The fixture's results are one line each, so a
// 24k model holds all twenty rounds; a model with a few thousand tokens does
// not, and it is the oldest results that go.
func TestASmallerWindowDropsTheOldestResultsOfTheFixture(t *testing.T) {
	run := newFixtureRun(t)
	run.playTo(t, 20)
	roomy, err := newGoldenBuilder(t).Build(t.Context(), run.input(24000))
	if err != nil {
		t.Fatalf("cannot build the roomy working context: %v", err)
	}

	builder := newTestBuilder(t, Options{Home: roomyHome(t), MaxOutputTokens: 512, Boundary: goldenBoundary})
	input := run.input(3000)
	tight, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the tight working context: %v", err)
	}

	t.Logf("a 3k window keeps %d of the %d results a 24k window keeps", resultsIn(tight), resultsIn(roomy))
	if resultsIn(tight) >= resultsIn(roomy) {
		t.Fatalf("a 3k window kept %d results and a 24k window kept %d, so nothing left the table",
			resultsIn(tight), resultsIn(roomy))
	}
	if err := onlyOlderMessagesLeft(tight, roomy); err != nil {
		t.Errorf("the tight window changed more than which of the oldest messages are on the table: %v", err)
	}
	if counted := EstimateRequestTokens(tight) + builder.maxOutputTokens; counted > input.ContextLength {
		t.Errorf("the tight prompt and its reply come to %d tokens, and the model holds %d", counted, input.ContextLength)
	}
	// This is the whole point of keeping the record beside the window rather
	// than inside it. The message the user sent at round twelve has dropped off
	// the table, and their words are still in front of the model, because the
	// harness wrote them into the record's rules where nothing may touch them.
	whole := testkit.WholeRequestText(tight)
	if !strings.Contains(whole, "no, lead with the date not the features") {
		t.Error("a 3k window lost the user's correction, and a correction is never lost")
	}
}

// newGoldenBuilder makes a builder whose every input is fixed, so that the same
// task always builds the same bytes.
func newGoldenBuilder(t *testing.T) *Builder {
	t.Helper()
	return newTestBuilder(t, Options{
		Home:            roomyHome(t),
		MaxOutputTokens: contract.DefaultConfig().Caps.OutputTokensPerCall,
		Boundary:        goldenBoundary,
	})
}

// roomyHome is a home with the three persona files written, short enough that
// none of them is cut.
func roomyHome(t *testing.T) contract.Home {
	t.Helper()
	home := testkit.NewTempHome(t)
	writePersonaFile(t, home.SoulFile(), "You are Coeus. You are careful, you say what you are doing, and you never guess.")
	writePersonaFile(t, home.UserFactsFile(), "The user is Jared. He works on DigiByte and prefers short answers.")
	writePersonaFile(t, home.WorldFactsFile(), "DigiByte launched on the tenth of January 2014.")
	return home
}

// renderPrompt writes a whole request out as plain text, so that a golden file
// shows a reader the prompt the model is actually sent, in order, with the cache
// boundaries on it.
func renderPrompt(request contract.Request) string {
	lines := []string{fmt.Sprintf("output cap: %d tokens", request.MaxOutputTokens), ""}
	lines = append(lines, aboveTheCacheLine(request))
	for at, message := range request.Messages {
		lines = append(lines, fmt.Sprintf("=== message %d from the %s ===", at+1, message.Role))
		if message.Text != "" {
			lines = append(lines, message.Text)
		}
		for _, call := range message.ToolCalls {
			lines = append(lines, fmt.Sprintf("--- calls %s as %s: %s ---", call.Name, call.ID, call.Input))
		}
		for _, result := range message.ToolResults {
			lines = append(lines, fmt.Sprintf("--- result of %s ---", result.CallID), result.Text)
		}
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// aboveTheCacheLine writes out everything the provider may reuse: the system
// blocks with their boundaries, and the tools.
func aboveTheCacheLine(request contract.Request) string {
	lines := []string{}
	for _, block := range request.SystemBlocks {
		boundary := string(block.Boundary)
		if boundary == "" {
			boundary = "none"
		}
		lines = append(lines, fmt.Sprintf("=== system block: %s (cache boundary: %s) ===", block.Name, boundary), block.Text, "")
	}
	for _, spec := range request.Tools {
		lines = append(lines, fmt.Sprintf("=== tool: %s ===", spec.Name), spec.Description)
		for _, field := range spec.Fields {
			lines = append(lines, fmt.Sprintf("  %s (%s): %s", field.Name, field.Type, field.Description))
		}
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// onlyOlderMessagesLeft says whether the smaller prompt is the bigger one with
// one run of its oldest messages left out, which is the only difference the
// window rule may make between two models. Anything else missing, anything in a
// different order, or a gap in the middle is a fault.
func onlyOlderMessagesLeft(smaller contract.Request, bigger contract.Request) error {
	at, runs, inRun := 0, 0, false
	for _, message := range bigger.Messages {
		if at < len(smaller.Messages) && renderMessage(smaller.Messages[at]) == renderMessage(message) {
			at, inRun = at+1, false
			continue
		}
		if !inRun {
			runs, inRun = runs+1, true
		}
	}
	if at != len(smaller.Messages) {
		return fmt.Errorf("only %d of the smaller prompt's %d messages line up with the bigger one's", at, len(smaller.Messages))
	}
	if runs > 1 {
		return fmt.Errorf("the smaller prompt is missing %d separate runs of messages, and the window only ever drops the oldest", runs)
	}
	return nil
}

// renderMessage writes one message out as text, so that two can be compared.
func renderMessage(message contract.Message) string {
	return renderPrompt(contract.Request{Messages: []contract.Message{message}})
}
