package context

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theRoundsTheCacheIsMeasuredAt are the three rounds the wave 6 gate review
// measured finding 17 at, so that its numbers and these can be read side by side.
var theRoundsTheCacheIsMeasuredAt = []int{10, 20, 30}

// leastSharedPercent is how much of a prompt has to be the same as the last
// one's for the layout to be doing its job. The gate review asked for eighty per
// cent and measured thirty-three at round thirty before the tail was built.
const leastSharedPercent = 80

// lastSharedResult is the result of round twenty of the fixture, which is the
// newest thing two rounds that follow each other have in common.
const lastSharedResult = "e13 text 228/280 shows the post is inside the limit."

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
// belongs below the cache line; only SOUL.md, which nothing but the user writes,
// stays above.
//
// It is the first thing below that line, and that is a trade worth saying out
// loud. USER.md and MEMORY.md hold still through almost every task, so up there
// they are read once and reused on every turn afterwards for nothing. The turn
// that does save a fact pays for the conversation under them once. The other
// place to put them is the tail, under the messages, and the tail is read again
// on every single call, so that would spend their whole size every turn to save
// one re-read that mostly never happens.
//
// So what this test holds is what finding 18 actually asked for: a saved fact
// never moves the system prompt, and it costs nothing above the block it is in.
func TestOneMemorySaveDoesNotMoveTheTopOfThePrompt(t *testing.T) {
	run := newFixtureRun(t)
	home := roomyHome(t)
	builder := newTestBuilder(t, Options{Home: home, MaxOutputTokens: contract.DefaultConfig().Caps.OutputTokensPerCall, Boundary: goldenBoundary})
	run.playTo(t, 20)

	first := buildWithTheBudgetSpent(t, builder, run, 80,
		contract.CostLine{InputTokens: 15200, CachedInputTokens: 3900, OutputTokens: 500})
	writePersonaFile(t, home.WorldFactsFile(),
		"DigiByte launched on the tenth of January 2014.\nA DigiByte block is mined about every fifteen seconds.")
	second := buildWithTheBudgetSpent(t, builder, run, 80,
		contract.CostLine{InputTokens: 15200, CachedInputTokens: 3900, OutputTokens: 500})

	before, after := renderPrompt(first), renderPrompt(second)
	if !strings.Contains(after, "about every fifteen seconds") {
		t.Fatal("the saved fact never reached the prompt, so this test is not measuring what it says it measures")
	}
	if aboveTheCacheLine(first) != aboveTheCacheLine(second) {
		t.Error("saving one fact rewrote the system prompt, and nothing the agent writes about itself may move what is above the cache line")
	}
	shared := sharedPrefix(before, after)
	t.Logf("one fact saved mid-task leaves %d characters of %d shared, which is %.1f per cent",
		len(shared), len(after), 100*float64(len(shared))/float64(len(after)))
	knownAt := strings.Index(before, whatIsKnownHeading)
	if knownAt < 0 {
		t.Fatalf("the prompt has no block of what the agent knows in it:\n%s", before)
	}
	if len(shared) < knownAt {
		t.Errorf("saving one fact left only %d characters of the prompt where they were, and what the agent knows does not begin until %d,"+
			" so the save cost something above the block it is in", len(shared), knownAt)
	}
}

// TestThePromptTheProviderCanReuseReachesTheEndOfTheConversation is the layout
// half of the cache rule, and it came out of the first live run on the local
// model: every call read about fifteen thousand tokens and the provider reused
// only about thirty-nine hundred of them, so each turn re-read eleven thousand
// tokens and took forty seconds. The cause was where the record's header sat.
// The budget line and the cost line change on every single call, and they were
// the first thing under the cache line, ahead of the goal, the plan and the
// results, so nothing below the system prompt could ever be reused. The second
// live run found the record's body doing the same thing to the conversation,
// which is what TestARewrittenSituationCostsOnlyTheTail holds.
//
// This builds the same task on two rounds that follow each other and measures
// what the two prompts share from the first byte. That shared run is what a
// provider may reuse, and it has to reach the instructions, the tool
// descriptions, what the agent knows, and the whole conversation down to the
// last result the two rounds have in common. Everything a turn writes anew --
// the record's body, its list of results, the memory hint and its header --
// belongs after all of that.
func TestThePromptTheProviderCanReuseReachesTheEndOfTheConversation(t *testing.T) {
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
		"## Goal", "## Rules",
		// What the agent knows, which is the first thing under the cache line.
		"The user is Jared",
		// The result of round twenty, which is the last thing the two rounds
		// have in common. Reaching it means the shared run covers the whole
		// conversation.
		lastSharedResult,
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
		if strings.LastIndex(prompt, recordSecondHalfHeading) < strings.LastIndex(prompt, lastSharedResult) {
			t.Errorf("at %s the record's body comes before the conversation, and its situation is rewritten every turn,"+
				" so every message under it is read again on every call", name)
		}
		if strings.LastIndex(prompt, memoryHintHeading) < strings.LastIndex(prompt, resultListLabel) {
			t.Errorf("at %s the memory hint comes before the list of results, and the list grows every round,"+
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

// TestARewrittenSituationCostsOnlyTheTail is the finding the first benchmark
// against opencode turned up, on the local daemon: a Coeus call read 18,658
// prompt tokens and the daemon reused only 4,322 of them, which is the system
// blocks and not one byte more, so 14,336 tokens were read from scratch and one
// call spent forty seconds of prefill on forty-five words of reply. opencode, on
// the same model and the same task, read about six hundred tokens a call.
//
// The cause was where the record's body sat. The body holds the Work section,
// whose Situation the turn loop rewrites after every single round, and it was
// the first message under the cache line, ahead of the whole conversation. So
// the growing, otherwise identical message history under it was thrown away on
// every call. Everything that is written anew every turn belongs in the tail,
// under the messages, and everything that only grows at its end belongs above
// it.
//
// This builds two turns whose only difference is a rewritten situation and one
// new message, and measures what the two prompts share from the first byte. The
// shared run has to cover everything the earlier turn sent except its tail.
func TestARewrittenSituationCostsOnlyTheTail(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	input := sampleInput()
	input.ContextLength = 200000
	input.Messages = roundsOfConversation(12, 1200)

	earlier, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the earlier turn: %v", err)
	}
	input.Record.Work.Situation = []string{"last command: go test ./..., exit 1", "where the work stands: reading the failing test"}
	input.Messages = append(input.Messages, contract.Message{Role: contract.RoleAssistant, Text: "Reading the failing test."})
	later, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the later turn: %v", err)
	}

	before, after := renderPrompt(earlier), renderPrompt(later)
	shared := sharedPrefix(before, after)
	exceptTheTail := renderPrompt(withoutTheTail(earlier))
	t.Logf("a rewritten situation and one new message leave %d characters of the earlier turn's %d shared, and all but its tail is %d",
		len(shared), len(before), len(exceptTheTail))
	if !strings.HasPrefix(after, exceptTheTail) {
		t.Errorf("the two turns share only %d characters from the start and everything but the earlier turn's tail is %d,"+
			" so something written anew every turn is sitting above the conversation", len(shared), len(exceptTheTail))
	}
	if !strings.Contains(shared, "the result of round 12,") {
		t.Error("the shared run does not reach the newest result of the earlier turn, so the provider reads the conversation again on every call")
	}
	if len(shared) == len(after) {
		t.Error("the two turns are the same bytes, so the rewritten situation never reached the prompt")
	}
}

// withoutTheTail is a request with the blocks the turn writes anew left off, so
// that what is left is what a provider may still have from the call before.
func withoutTheTail(request contract.Request) contract.Request {
	kept := contract.Request{SystemBlocks: request.SystemBlocks, Tools: request.Tools, MaxOutputTokens: request.MaxOutputTokens}
	for _, message := range request.Messages {
		if isTail(message) {
			break
		}
		kept.Messages = append(kept.Messages, message)
	}
	return kept
}

// isTail says whether a message is one of the four blocks of the tail: the
// record's body, its list of results, the memory hint, and its header.
func isTail(message contract.Message) bool {
	for _, heading := range []string{recordSecondHalfHeading, recordResultsHeading, memoryHintHeading, recordHeaderHeading} {
		if strings.HasPrefix(message.Text, heading) {
			return true
		}
	}
	return false
}

// TestAPinAddedBetweenTurnsDoesNotMoveTheMessages holds the other half of the
// order. Pinned evidence changes only when somebody pins something, so it sits
// above the conversation, where a pin costs its own text and rewrites nothing:
// every message is still there, in the same order, byte for byte, and the tail
// is still under them.
func TestAPinAddedBetweenTurnsDoesNotMoveTheMessages(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	input := sampleInput()
	input.ContextLength = 200000
	input.Messages = roundsOfConversation(12, 1200)

	before, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the turn before the pin: %v", err)
	}
	input.Pinned = []Pin{{ID: "r6", Text: "the draft post, 236 characters"}}
	after, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the turn after the pin: %v", err)
	}

	if !strings.Contains(testkit.WholeRequestText(after), "the draft post, 236 characters") {
		t.Fatal("the pin never reached the prompt, so this test is not measuring what it says it measures")
	}
	if was, is := conversationText(before), conversationText(after); was != is {
		t.Errorf("pinning one thing rewrote the conversation:\n--- before ---\n%s\n--- after ---\n%s", was, is)
	}
	whole := renderPrompt(after)
	if strings.Index(whole, "the draft post, 236 characters") > strings.Index(whole, "the result of round 1,") {
		t.Error("the pinned evidence is under the conversation, and it changes only when something is pinned")
	}
	if strings.LastIndex(whole, "the result of round 12,") > strings.Index(whole, recordSecondHalfHeading) {
		t.Error("the conversation runs past the start of the tail, and the tail is what is written anew every turn")
	}
}

// TestTheTailRunsFromTheRecordBodyToTheHeader pins the order of the tail itself.
// Under the conversation come the record's body, then its list of results, which
// grows by a line every round, then the memory hint, and last of all the header,
// whose budget line and cost line are written anew on every single call.
func TestTheTailRunsFromTheRecordBodyToTheHeader(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	input := sampleInput()
	input.MemoryHint = []string{"Jared posts at 14:00", "one fact per post"}

	request, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}
	whole := renderPrompt(request)
	at := -1
	for _, wanted := range []string{
		"Reading the product notes.", recordSecondHalfHeading, resultListLabel, memoryHintHeading, "budget left:",
	} {
		found := strings.LastIndex(whole, wanted)
		if found < 0 {
			t.Fatalf("the working context is missing %q:\n%s", firstLineOf(wanted), whole)
		}
		if found < at {
			t.Errorf("%q comes before what should be above it in the prompt", firstLineOf(wanted))
		}
		at = found
	}
	last := request.Messages[len(request.Messages)-1]
	if !strings.Contains(last.Text, "budget left:") {
		t.Errorf("the record's header is not the last thing the model reads:\n%s", last.Text)
	}
}

// conversationText is the messages the caller handed over, rendered in order,
// with the blocks this package writes around them left out. Two turns whose
// conversations render the same have had nothing rewritten, dropped or moved.
func conversationText(request contract.Request) string {
	kept := []contract.Message{}
	for _, message := range request.Messages {
		if isABlockThisPackageWrote(message) {
			continue
		}
		kept = append(kept, message)
	}
	return renderPrompt(contract.Request{Messages: kept})
}

// isABlockThisPackageWrote says whether a message is one of the named blocks the
// builder puts around the conversation rather than a message of the task itself.
func isABlockThisPackageWrote(message contract.Message) bool {
	for _, heading := range []string{
		recordSecondHalfHeading, recordResultsHeading, recordHeaderHeading,
		whatIsKnownHeading, pinnedHeading, memoryHintHeading,
	} {
		if strings.HasPrefix(message.Text, heading) {
			return true
		}
	}
	return false
}
