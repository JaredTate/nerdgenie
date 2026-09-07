package context

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// sampleRecentWork is two just-finished tasks, newest first, in the shape the
// next wave's wiring will hand over.
func sampleRecentWork() []RecentTask {
	return []RecentTask{
		{Number: 4, Ask: "Post a tweet about the DigiByte anniversary.", Standing: "posted, 236 characters"},
		{Number: 3, Ask: "Draft the anniversary blog piece.", Standing: "saved to blog/anniversary.md"},
	}
}

// TestRecentWorkRidesBelowTheToolsAndAboveTheRecordsGoal proves the recent
// work is one of the first messages below the tools, before the record's goal
// and before the conversation: it holds still through a sitting, so it sits
// where the provider reuses it, and never in the system prompt, which since 7
// September 2026 is the same bytes for every task of a run.
func TestRecentWorkRidesBelowTheToolsAndAboveTheRecordsGoal(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	input := sampleInput()
	input.RecentWork = sampleRecentWork()

	request, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}

	if blockIndex(request, BlockRecentWork) >= 0 || blockIndex(request, BlockRecord) >= 0 {
		t.Fatalf("the recent work or the record rides in the system prompt, ahead of the tools: %s", blockNames(request))
	}
	recentAt, goalAt := messageIndex(request, recentWorkHeading), messageIndex(request, recordFirstHalfHeading)
	if recentAt < 0 || goalAt < 0 {
		t.Fatalf("the recent work (%d) or the record's goal (%d) is missing below the tools", recentAt, goalAt)
	}
	if recentAt >= goalAt {
		t.Errorf("the recent work is message %d and the record's goal %d, but recent work goes above the goal", recentAt, goalAt)
	}
	if goalAt >= messageIndex(request, "Reading the product notes.") {
		t.Errorf("the record's goal comes after the conversation, and it holds still through a task")
	}

	block := request.Messages[recentAt].Text
	for _, want := range []string{"task 4:", "Post a tweet about the DigiByte anniversary.", "posted, 236 characters", "task 3:", "saved to blog/anniversary.md"} {
		if !strings.Contains(block, want) {
			t.Errorf("the recent-work message is missing %q:\n%s", want, block)
		}
	}
	whole := testkit.WholeRequestText(request)
	if strings.Count(whole, recentWorkHeading) != 1 {
		t.Errorf("the recent-work heading appears %d times, want once", strings.Count(whole, recentWorkHeading))
	}
}

// messageIndex is the position of the first message whose text carries the
// words, or minus one.
func messageIndex(request contract.Request, words string) int {
	for at, message := range request.Messages {
		if strings.Contains(message.Text, words) {
			return at
		}
	}
	return -1
}

// TestRecentWorkIsOneLinePerTask proves the block writes one line per task under
// the heading, so the model reads it as a short list and not a paragraph.
func TestRecentWorkIsOneLinePerTask(t *testing.T) {
	text := recentWorkText(sampleRecentWork())
	lines := strings.Split(text, "\n")
	if len(lines) != 2 {
		t.Fatalf("two tasks wrote %d lines, want one each:\n%s", len(lines), text)
	}
	if lines[0] != "task 4: Post a tweet about the DigiByte anniversary. — posted, 236 characters" {
		t.Errorf("the line is not in the form \"task N: ask — standing\":\n%q", lines[0])
	}
}

// TestRecentWorkShowsAtMostThreeTasks proves the cap, so a long history cannot
// push the record down the prompt for a list nobody reads to the end.
func TestRecentWorkShowsAtMostThreeTasks(t *testing.T) {
	many := []RecentTask{
		{Number: 9, Ask: "nine", Standing: "done"},
		{Number: 8, Ask: "eight", Standing: "done"},
		{Number: 7, Ask: "seven", Standing: "done"},
		{Number: 6, Ask: "six", Standing: "done"},
		{Number: 5, Ask: "five", Standing: "done"},
	}
	text := recentWorkText(many)
	if lines := strings.Split(text, "\n"); len(lines) != MaxRecentTasks {
		t.Fatalf("five tasks wrote %d lines, want the cap of %d:\n%s", len(lines), MaxRecentTasks, text)
	}
	for _, dropped := range []string{"task 6:", "task 5:"} {
		if strings.Contains(text, dropped) {
			t.Errorf("a task past the three most recent was kept: %q\n%s", dropped, text)
		}
	}
	if !strings.Contains(text, "task 9:") || !strings.Contains(text, "task 7:") {
		t.Errorf("the three most recent tasks were not the ones kept:\n%s", text)
	}
}

// TestRecentWorkTrimsTheAskToASentenceAndBoundsEachLine proves a line cannot
// grow the prompt without limit: the ask is cut to its first sentence and both
// the ask and the standing are held inside a rune cap.
func TestRecentWorkTrimsTheAskToASentenceAndBoundsEachLine(t *testing.T) {
	tasks := []RecentTask{
		{Number: 7, Ask: "Buy milk today. Then walk the dog and tidy the whole house before noon.", Standing: "not started"},
		{Number: 6, Ask: strings.Repeat("verylongword ", 40), Standing: strings.Repeat("standingword ", 40)},
	}
	text := recentWorkText(tasks)
	lines := strings.Split(text, "\n")
	if len(lines) != 2 {
		t.Fatalf("recentWorkText wrote %d lines, want 2:\n%s", len(lines), text)
	}
	if !strings.Contains(lines[0], "Buy milk today.") {
		t.Errorf("the ask was not kept to its first sentence: %q", lines[0])
	}
	if strings.Contains(lines[0], "walk the dog") {
		t.Errorf("the ask kept text past its first sentence: %q", lines[0])
	}
	bound := MaxRecentAskRunes + MaxRecentStandingRunes + len([]rune("task 000 (stopped):  — "))
	for _, line := range lines {
		if runes := len([]rune(line)); runes > bound {
			t.Errorf("a recent-work line is %d runes, over the bound of %d: %q", runes, bound, line)
		}
	}
}

// TestRecentWorkRidesAboveAnEmptyRecord proves the "where are we" case: the
// current record is empty, and the recent work is still the first message
// below the tools, so a small window keeps it and the provider reuses it.
func TestRecentWorkRidesAboveAnEmptyRecord(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	input := sampleInput()
	input.Record = contract.Record{}
	input.RecentWork = sampleRecentWork()

	request, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the working context with an empty record: %v", err)
	}
	if messageIndex(request, recordFirstHalfHeading) >= 0 {
		t.Errorf("a task with no record carries one")
	}
	if len(request.Messages) == 0 || !strings.HasPrefix(request.Messages[0].Text, recentWorkHeading) {
		t.Fatalf("the recent work is not the first message below the tools when the record is empty")
	}
}

// TestAnEmptyRecentWorkAddsNothing proves a task with no recent work pays
// nothing for it: no block, and the same bytes a nil field builds.
func TestAnEmptyRecentWorkAddsNothing(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	base := sampleInput()

	withNil, err := builder.Build(t.Context(), base)
	if err != nil {
		t.Fatalf("cannot build with a nil recent-work field: %v", err)
	}
	if blockIndex(withNil, BlockRecentWork) >= 0 {
		t.Errorf("a nil recent-work field wrote a block: %s", blockNames(withNil))
	}

	base.RecentWork = []RecentTask{}
	withEmpty, err := builder.Build(t.Context(), base)
	if err != nil {
		t.Fatalf("cannot build with an empty recent-work field: %v", err)
	}
	if testkit.WholeRequestText(withNil) != testkit.WholeRequestText(withEmpty) {
		t.Error("an empty recent-work field changed the prompt, but nothing should be rendered for it")
	}
}

// blockIndex is where a named system block sits, or minus one when it is absent.
func blockIndex(request contract.Request, name string) int {
	for at, block := range request.SystemBlocks {
		if block.Name == name {
			return at
		}
	}
	return -1
}

// TestRecentWorkNamesHowATaskEndedWhenItWasNotFinished holds that a task the
// person set aside is shown with the word for it, so the model does not read
// stopped work as finished work, and a finished task carries no such word.
func TestRecentWorkNamesHowATaskEndedWhenItWasNotFinished(t *testing.T) {
	text := recentWorkText([]RecentTask{
		{Number: 2, Status: "stopped", Ask: "build the game", Standing: "the tests pass"},
		{Number: 1, Status: "done", Ask: "read the note", Standing: "read: the note"},
	})
	if !strings.HasPrefix(text, "task 2 (stopped): build the game") {
		t.Errorf("the stopped task reads %q, want its status after its number", strings.Split(text, "\n")[0])
	}
	if !strings.Contains(text, "\ntask 1: read the note") {
		t.Errorf("the finished task reads %q, and done is not worth a word", text)
	}
}
