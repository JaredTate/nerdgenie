package context

import (
	stdcontext "context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestTheLayersArriveInTheOrderTheDesignPutsThem proves the system prompt holds
// the layers of design section 4 in order, each as its own block, with the cache
// boundaries where the design puts them, and that the layers which change per
// task, the job summary and the record's goal and rules, ride first below the
// tools rather than above them, since 7 September 2026.
func TestTheLayersArriveInTheOrderTheDesignPutsThem(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	writePersonaFile(t, builder.home.SoulFile(), "I am Nerd Genie.")

	input := sampleInput()
	input.JobSummary = "# job 4   running   3 of 12 tasks done"
	request, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}

	wanted := []struct {
		name     string
		boundary contract.CacheBoundary
	}{
		{BlockInstructions, contract.CacheBoundaryNone},
		{BlockPersona, contract.CacheBoundaryA},
		{BlockTools, contract.CacheBoundaryB},
	}
	if !strings.HasPrefix(request.Messages[0].Text, jobHeading) || !strings.HasPrefix(request.Messages[1].Text, recordFirstHalfHeading) {
		t.Errorf("the first messages below the tools are not the job summary and the record's goal:\n%s\n%s",
			firstLineOf(request.Messages[0].Text), firstLineOf(request.Messages[1].Text))
	}
	if len(request.SystemBlocks) != len(wanted) {
		t.Fatalf("the system prompt has %d blocks, want %d: %s", len(request.SystemBlocks), len(wanted), blockNames(request))
	}
	for at, block := range request.SystemBlocks {
		if block.Name != wanted[at].name {
			t.Errorf("block %d is %q, want %q", at, block.Name, wanted[at].name)
		}
		if block.Boundary != wanted[at].boundary {
			t.Errorf("block %q ends boundary %q, want %q", block.Name, block.Boundary, wanted[at].boundary)
		}
		if strings.TrimSpace(block.Text) == "" {
			t.Errorf("block %q has nothing in it, and an empty block is refused on the wire", block.Name)
		}
	}
}

// TestTheJobSummaryIsThereOnlyWhenTheTaskBelongsToAJob proves a task that stands
// on its own pays nothing for a job it does not have.
func TestTheJobSummaryIsThereOnlyWhenTheTaskBelongsToAJob(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	request, err := builder.Build(t.Context(), sampleInput())
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}
	for _, block := range request.SystemBlocks {
		if block.Name == BlockJob {
			t.Errorf("a task with no job carries a job summary: %q", block.Text)
		}
	}
}

// TestTheRecordIsSplitAcrossTheCacheLine proves the goal and the rules are the
// first message below the tools, the work and the lessons come after the
// conversation, and the header and the step line close the prompt, because
// everything that changes has to go where nothing above it moves.
func TestTheRecordIsSplitAcrossTheCacheLine(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	request, err := builder.Build(t.Context(), sampleInput())
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}

	system := systemText(request)
	if strings.Contains(system, "Post a tweet") {
		t.Errorf("the ask rides in the system prompt, ahead of the tools, and it changes per task:\n%s", system)
	}
	if !strings.Contains(request.Messages[0].Text, "Post a tweet") {
		t.Errorf("the ask is not the first message below the tools:\n%s", request.Messages[0].Text)
	}
	if strings.Contains(system, "## Work") {
		t.Errorf("the record's work is above the cache line, and it changes every turn:\n%s", system)
	}
	whole := testkit.WholeRequestText(request)
	if !strings.Contains(whole, "## Work") {
		t.Errorf("the record's work is nowhere below the cache line:\n%s", whole)
	}
	if strings.Index(whole, "## Work") < strings.Index(whole, "Reading the product notes.") {
		t.Errorf("the record's body comes before the conversation, and its situation is rewritten every turn:\n%s", whole)
	}
	header := request.Messages[len(request.Messages)-2]
	if !strings.Contains(header.Text, "this turn:") {
		t.Errorf("the record's header, which is written anew on every call, is not the last thing before the step line:\n%s", header.Text)
	}
	if last := request.Messages[len(request.Messages)-1]; last.Text != NextStepLine(sampleInput().Record) {
		t.Errorf("the step line is not the last thing the model reads:\n%s", last.Text)
	}
	if strings.Contains(request.Messages[0].Text, "this turn:") {
		t.Errorf("the record's header is still at the front, where it costs the provider everything under it:\n%s", request.Messages[0].Text)
	}
}

// TestTheMessagesRunFromWhatIsKnownToTheBudgetLine proves the order below the
// cache line runs from what changes least to what changes most: what the agent
// knows, the pinned evidence, the recent messages, and then the tail, which is
// everything a turn writes anew — the record's body, its list of results, the
// memory hint, and last of all the two lines of the record's header.
func TestTheMessagesRunFromWhatIsKnownToTheBudgetLine(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	writePersonaFile(t, builder.home.UserFactsFile(), "The user is Jared.")
	input := sampleInput()
	input.Pinned = []Pin{{ID: "r6", Text: "the draft post, 236 characters"}}
	input.MemoryHint = []string{"Jared posts at 14:00", "one fact per post"}

	request, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}
	whole := testkit.WholeRequestText(request)
	order := []string{
		"The user is Jared.", "the draft post, 236 characters", "Reading the product notes.",
		"## Work", "Jared posts at 14:00", "budget left:",
	}
	at := -1
	for _, wanted := range order {
		found := strings.Index(whole, wanted)
		if found < 0 {
			t.Fatalf("the working context is missing %q:\n%s", wanted, whole)
		}
		if found < at {
			t.Errorf("%q comes before what should be above it in the prompt", wanted)
		}
		at = found
	}
}

// TestEveryToolResultIsWrappedAsData proves rule 8 of design section 3: words
// inside a tool result are never instructions, so every result is marked.
func TestEveryToolResultIsWrappedAsData(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	input := sampleInput()
	request, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}

	found := 0
	for _, message := range request.Messages {
		for _, result := range message.ToolResults {
			found++
			if !strings.Contains(result.Text, builder.Boundary()) {
				t.Errorf("the result of %s is not marked as data:\n%s", result.CallID, result.Text)
			}
		}
	}
	if found == 0 {
		t.Fatal("no tool result reached the working context, so nothing was checked")
	}
}

// TestTheCallersMessagesAreNeverChanged proves the builder marks a copy. The
// loop keeps the conversation and hands it over on every turn, and a builder
// that wrapped the caller's own results would wrap them again next turn.
func TestTheCallersMessagesAreNeverChanged(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	input := sampleInput()
	before := input.Messages[1].ToolResults[0].Text

	if _, err := builder.Build(t.Context(), input); err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}
	if after := input.Messages[1].ToolResults[0].Text; after != before {
		t.Errorf("the builder changed the caller's own message: %q became %q", before, after)
	}
}

// TestTwoBuildersUseDifferentBoundaries proves each builder makes its own, so
// that a run which does build one per task gets a fresh boundary for each. The
// daemon of today builds one and shares it, which finding 20 of the wave 6 gate
// review names; what makes that safe is held by
// TestABoundaryLearnedInOneTaskCannotBreakOutOfAnother.
func TestTwoBuildersUseDifferentBoundaries(t *testing.T) {
	first := newTestBuilder(t, Options{})
	second := newTestBuilder(t, Options{})
	if first.Boundary() == second.Boundary() {
		t.Errorf("two builders share the boundary %q", first.Boundary())
	}
}

// TestABuilderNeedsWhatItCannotDoWithout proves the constructor refuses options
// it cannot work from, rather than building a prompt that is quietly wrong.
func TestABuilderNeedsWhatItCannotDoWithout(t *testing.T) {
	home := testkit.NewTempHome(t)
	caps := contract.DefaultConfig().MemoryCaps
	for _, check := range []struct {
		name    string
		options Options
	}{
		{"no home folder", Options{MemoryCaps: caps, MaxOutputTokens: 8192}},
		{"no memory caps", Options{Home: home, MaxOutputTokens: 8192}},
		{"no output cap", Options{Home: home, MemoryCaps: caps}},
	} {
		if _, err := New(check.options); err == nil {
			t.Errorf("a builder with %s was allowed", check.name)
		}
	}
}

// TestTheFirstTurnOfATaskHasNoRecordYet proves a call made before the first tool
// has run, when no record exists, still builds a prompt.
func TestTheFirstTurnOfATaskHasNoRecordYet(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	input := sampleInput()
	input.Record = contract.Record{}

	request, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("the first turn of a task cannot be built: %v", err)
	}
	for _, block := range request.SystemBlocks {
		if block.Name == BlockRecord {
			t.Errorf("a task with no record yet carries one: %q", block.Text)
		}
	}
}

// TestAGivenUpTurnBuildsNothing proves the builder honours the turn's context,
// because reading the persona files touches the disk.
func TestAGivenUpTurnBuildsNothing(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	ctx, stop := stdcontext.WithCancel(t.Context())
	stop()
	if _, err := builder.Build(ctx, sampleInput()); err == nil {
		t.Error("a turn that was given up on still built a working context")
	}
}

// TestAPersonaThatCannotBeReadStopsTheBuild proves a broken persona is reported
// rather than left out, because the model would otherwise be told nothing about
// who it is and nobody would know why.
func TestAPersonaThatCannotBeReadStopsTheBuild(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	if err := os.Mkdir(builder.home.SoulFile(), contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot put a folder where SOUL.md belongs: %v", err)
	}
	if _, err := builder.Build(t.Context(), sampleInput()); err == nil {
		t.Error("a persona file that cannot be read did not stop the build")
	}
}

// newTestBuilder makes a builder over a temporary home, filling in whatever the
// test did not care about.
func newTestBuilder(t *testing.T, options Options) *Builder {
	t.Helper()
	if options.Home.Root == "" {
		options.Home = testkit.NewTempHome(t)
	}
	if options.MemoryCaps == (contract.MemoryCaps{}) {
		options.MemoryCaps = contract.DefaultConfig().MemoryCaps
	}
	if options.MaxOutputTokens == 0 {
		options.MaxOutputTokens = contract.DefaultConfig().Caps.OutputTokensPerCall
	}
	builder, err := New(options)
	if err != nil {
		t.Fatalf("cannot make a working-context builder: %v", err)
	}
	return builder
}

// sampleInput is one turn's worth of input: the record of design section 4, one
// round of conversation, and the tools.
func sampleInput() BuildInput {
	return BuildInput{
		ContextLength: 24000,
		Record:        sampleRecord(),
		Messages: []contract.Message{
			{
				Role:      contract.RoleAssistant,
				Text:      "Reading the product notes.",
				ToolCalls: []contract.ToolCall{{ID: "call_1", Name: "read", Input: json.RawMessage(`{"path":"memory/product.md"}`)}},
			},
			{
				Role:        contract.RoleUser,
				ToolResults: []contract.ToolResult{{CallID: "call_1", Text: "read the product notes, 2,100 characters"}},
			},
		},
		Tools: sampleTools(),
	}
}

// sampleTools are two tools in the shape the registry hands over.
func sampleTools() []contract.ToolSpec {
	return []contract.ToolSpec{
		{
			Name:        "read",
			Description: "Read a file, a folder, or a past result by its id. Use it before editing anything.",
			Fields:      []contract.ToolField{{Name: "path", Type: "string", Description: "what to read", Required: true}},
			Classes:     []contract.PermissionClass{contract.ClassRead},
		},
		{
			Name:        "write",
			Description: "Write a file inside the allowed folders. Do not use it to edit part of a file.",
			Fields:      []contract.ToolField{{Name: "path", Type: "string", Description: "where to write", Required: true}},
			Classes:     []contract.PermissionClass{contract.ClassWrite},
		},
	}
}

// systemText is everything in the system prompt, joined.
func systemText(request contract.Request) string {
	parts := []string{}
	for _, block := range request.SystemBlocks {
		parts = append(parts, block.Text)
	}
	return strings.Join(parts, "\n\n")
}

// blockNames names the blocks a request holds, for a failure message.
func blockNames(request contract.Request) string {
	names := []string{}
	for _, block := range request.SystemBlocks {
		names = append(names, block.Name)
	}
	return strings.Join(names, ", ")
}
