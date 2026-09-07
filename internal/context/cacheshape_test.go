package context

import (
	"strconv"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The three shapes of the cache-shaped prompt of 7 September 2026, measured on
// the day before: 25 of 32 task starts re-read the ten to fourteen thousand
// token front, because the job summary and the record's goal sat in the system
// prompt ahead of the tools; 18 cache fall-backs came from a picture leaving
// the window and rewriting an older result; and the model answered the last
// thing it read, so the tail now ends with the step it is on.

// TestTheFrontIsTheSameBytesAcrossTwoTasksOfOneJob holds the rule that bytes
// which change per task sit after the bytes that never do: the instructions,
// the persona and every tool are one byte-identical run for every task of a
// run, and a fact saved to memory between the tasks does not shorten it.
func TestTheFrontIsTheSameBytesAcrossTwoTasksOfOneJob(t *testing.T) {
	builder := newGoldenBuilder(t)
	run := newFixtureRun(t)
	run.playTo(t, 12)
	first := run.input(24000)
	first.JobSummary = "# job 3   running   0 of 4 tasks done\n\nTasks:\n- [ ] t1 write the post\n- [ ] t2 post it"

	earlier, err := builder.Build(t.Context(), first)
	if err != nil {
		t.Fatalf("cannot build the first task's prompt: %v", err)
	}
	writePersonaFile(t, builder.home.WorldFactsFile(), "DigiByte launched on the tenth of January 2014.\nThe post went up at noon.")
	second := run.input(24000)
	second.Record.Goal.Ask = "Post the second draft and confirm it."
	second.Record.Header.ID = "t2"
	second.JobSummary = "# job 3   running   1 of 4 tasks done\n\nTasks:\n- [x] t1 write the post -> j3.1\n- [ ] t2 post it"
	second.RecentWork = []RecentTask{{Number: 1, Status: "done", Ask: "write the post", Standing: "the post is written"}}
	later, err := builder.Build(t.Context(), second)
	if err != nil {
		t.Fatalf("cannot build the second task's prompt: %v", err)
	}

	whole := renderPrompt(later)
	shared := sharedPrefix(renderPrompt(earlier), whole)
	front := aboveTheCacheLine(later)
	frontEnds := strings.Index(whole, front) + len(front)
	if !strings.Contains(whole, front) || len(shared) < frontEnds {
		t.Fatalf("the two tasks share %d characters, and the front of instructions, persona and tools ends at %d; the front must be the same bytes for every task:\n%s",
			len(shared), frontEnds, firstLineOf(strings.TrimPrefix(whole, shared)))
	}
	t.Logf("two tasks of one job share %d of %d characters from the start; the front of instructions, persona and tools is %d", len(shared), len(whole), len(front))
	for _, block := range later.SystemBlocks {
		if block.Name == BlockJob || block.Name == BlockRecord || block.Name == BlockRecentWork {
			t.Errorf("the %s block still rides in the system prompt ahead of the tools", block.Name)
		}
	}
	for _, wanted := range []string{second.JobSummary, "Post the second draft and confirm it.", "the post is written"} {
		if !strings.Contains(whole, wanted) {
			t.Errorf("the second task's prompt lost %q", wanted)
		}
	}
}

// TestPicturesRideLastAndLeaveNothingBehind: a picture rides in one message at
// the very end, replaced each round, and a tool result in the conversation
// never carries one, so that a picture leaving the window changes no byte
// above the tail.
func TestPicturesRideLastAndLeaveNothingBehind(t *testing.T) {
	builder := newGoldenBuilder(t)
	input := BuildInput{ContextLength: 24000, Tools: sampleTools(), Record: aRecordWithAPlan()}
	for at := 1; at <= MaxPicturesShown+1; at++ {
		input.Messages = append(input.Messages, aLookAndItsPicture(at)...)
	}

	request, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the prompt: %v", err)
	}

	for at, message := range request.Messages[:len(request.Messages)-2] {
		for _, result := range message.ToolResults {
			if result.Picture != "" {
				t.Errorf("message %d still carries a picture inside the conversation", at+1)
			}
		}
	}
	pictures := request.Messages[len(request.Messages)-2]
	if pictures.Role != contract.RoleUser || len(pictures.ToolResults) != MaxPicturesShown {
		t.Fatalf("the message before the last carries %d pictures, want the newest %d in a user message", len(pictures.ToolResults), MaxPicturesShown)
	}
	if pictures.ToolResults[0].Label != "r3" || pictures.ToolResults[0].CallID != "" || pictures.ToolResults[0].Picture != "picture-3" {
		t.Errorf("the newest picture is not first, or carries a call id: %+v", pictures.ToolResults[0])
	}
	if !strings.Contains(pictures.Text, "r3") || !strings.Contains(pictures.Text, ThePictureIsNoLongerShown) {
		t.Errorf("the pictures message does not say which pictures ride and how to see the older ones:\n%s", pictures.Text)
	}

	input.Messages = append(input.Messages, aLookAndItsPicture(MaxPicturesShown+2)...)
	next, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the next prompt: %v", err)
	}
	// The render numbers its messages, so the shared run is measured up to the
	// end of the last result both prompts hold, not up to the tail's heading.
	earlier := renderPrompt(request)
	lastResult := strings.LastIndex(earlier, "the page as it stands") + len("the page as it stands")
	if shared := sharedPrefix(earlier, renderPrompt(next)); len(shared) < lastResult {
		t.Errorf("a new picture changed bytes inside the conversation: %d characters shared, the last shared result ends at %d", len(shared), lastResult)
	}
}

// TestTheTailEndsWithTheStepTheModelIsOn: the last thing the model reads is
// one harness line naming the open step, the newest result and the next step,
// because the plan regression of 6 September showed the model answers the
// last thing it reads.
func TestTheTailEndsWithTheStepTheModelIsOn(t *testing.T) {
	builder := newGoldenBuilder(t)
	request, err := builder.Build(t.Context(), BuildInput{ContextLength: 24000, Tools: sampleTools(), Record: aRecordWithAPlan()})
	if err != nil {
		t.Fatalf("cannot build the prompt: %v", err)
	}

	last := request.Messages[len(request.Messages)-1]
	for _, wanted := range []string{"Step 2 of 3: post it.", "Next: step 3, confirm the compose box.", "Open done lines: 2.", "Last: r2 tests: all 128 passing."} {
		if !strings.Contains(last.Text, wanted) {
			t.Errorf("the last message does not say %q:\n%s", wanted, last.Text)
		}
	}
	if last.Text != NextStepLine(aRecordWithAPlan()) {
		t.Errorf("the last message is not the next-step line alone:\n%s", last.Text)
	}

	noPlan := aRecordWithAPlan()
	noPlan.Work.Plan = nil
	if line := NextStepLine(noPlan); !strings.HasPrefix(line, "Open done lines: 2.") || strings.Contains(line, "Step") {
		t.Errorf("a record with no plan should open with the open done lines: %q", line)
	}
	if line := NextStepLine(contract.Record{}); line != "" {
		t.Errorf("a task with no record yet has no step line, got %q", line)
	}
}

// aRecordWithAPlan is a small task record: three steps with the first done,
// three done lines with one proved, two results.
func aRecordWithAPlan() contract.Record {
	return contract.Record{
		Header: contract.Header{Kind: contract.RecordTask, ID: "t9", Status: contract.StatusRunning, Origin: "terminal", RoundsLeft: 10, MinutesLeft: 5},
		Goal: contract.Goal{Ask: "Post the draft.", DoneWhen: []contract.DoneLine{
			{Text: "the post is up"}, {Text: "the link works", Done: true, ResultID: "r2"}, {Text: "the user is told"},
		}},
		Work: contract.Work{
			Plan: []contract.PlanStep{
				{Number: 1, Text: "draft it", Done: true, ResultID: "r1"},
				{Number: 2, Text: "post it"},
				{Number: 3, Text: "confirm the compose box"},
			},
			Results: []contract.ResultLine{{ID: "r1", Summary: "write: the draft, 236 characters"}, {ID: "r2", Summary: "tests: all 128 passing"}},
		},
	}
}

// aLookAndItsPicture is one round of the conversation: a call to snap and its
// result carrying a picture labelled r<n>.
func aLookAndItsPicture(number int) []contract.Message {
	label := "r" + strconv.Itoa(number)
	call := "call_" + strconv.Itoa(number)
	return []contract.Message{
		{Role: contract.RoleAssistant, Text: "I will look.", ToolCalls: []contract.ToolCall{{ID: call, Name: "snap"}}},
		{Role: contract.RoleUser, ToolResults: []contract.ToolResult{{CallID: call, Label: label, Text: "the page as it stands", Picture: "picture-" + strconv.Itoa(number)}}},
	}
}
