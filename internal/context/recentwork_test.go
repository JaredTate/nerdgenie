package context

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// sampleRecentWork is two just-finished tasks, newest first, in the shape the
// next wave's wiring will hand over.
func sampleRecentWork() []RecentTask {
	return []RecentTask{
		{Number: 4, Ask: "Post a tweet about the DigiByte anniversary.", Standing: "posted, 236 characters"},
		{Number: 3, Ask: "Draft the anniversary blog piece.", Standing: "saved to blog/anniversary.md"},
	}
}

// TestRecentWorkRidesAboveTheRecordInTheCachePrefix proves the recent-work block
// is a system block placed above the record, so it sits in the part of the
// prompt the provider can reuse rather than in the volatile tail.
func TestRecentWorkRidesAboveTheRecordInTheCachePrefix(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	input := sampleInput()
	input.RecentWork = sampleRecentWork()

	request, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}

	recentAt, recordAt := blockIndex(request, BlockRecentWork), blockIndex(request, BlockRecord)
	if recentAt < 0 {
		t.Fatalf("a populated recent-work field wrote no recent-work block: %s", blockNames(request))
	}
	if recordAt < 0 {
		t.Fatalf("the record block is missing: %s", blockNames(request))
	}
	if recentAt >= recordAt {
		t.Errorf("the recent-work block is at %d and the record at %d, but recent work goes above the record", recentAt, recordAt)
	}
	if request.SystemBlocks[recordAt].Boundary != contract.CacheBoundaryC {
		t.Errorf("the record no longer ends cache boundary C, so recent work would not be inside the cached prefix")
	}
	if request.SystemBlocks[recentAt].Boundary != contract.CacheBoundaryNone {
		t.Errorf("the recent-work block ends its own cache boundary %q, but with a record it rides above boundary C on the record",
			request.SystemBlocks[recentAt].Boundary)
	}

	block := request.SystemBlocks[recentAt].Text
	for _, want := range []string{"task 4:", "Post a tweet about the DigiByte anniversary.", "posted, 236 characters", "task 3:", "saved to blog/anniversary.md"} {
		if !strings.Contains(block, want) {
			t.Errorf("the recent-work block is missing %q:\n%s", want, block)
		}
	}

	// The block is a system block, so its heading is never repeated in the
	// messages below the cache line.
	whole := testkit.WholeRequestText(request)
	if strings.Count(whole, recentWorkHeading) != 1 {
		t.Errorf("the recent-work heading appears %d times, want once and only above the cache line", strings.Count(whole, recentWorkHeading))
	}
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
	bound := MaxRecentAskRunes + MaxRecentStandingRunes + len([]rune("task 000:  — "))
	for _, line := range lines {
		if runes := len([]rune(line)); runes > bound {
			t.Errorf("a recent-work line is %d runes, over the bound of %d: %q", runes, bound, line)
		}
	}
}

// TestRecentWorkRidesAboveAnEmptyRecord proves the "where are we" case: the
// current record is empty, and the recent-work block still rides above the cache
// line so a small window keeps it and the provider can reuse it. With no record
// to carry boundary C, the recent-work block carries it instead.
func TestRecentWorkRidesAboveAnEmptyRecord(t *testing.T) {
	builder := newTestBuilder(t, Options{})
	input := sampleInput()
	input.Record = contract.Record{}
	input.RecentWork = sampleRecentWork()

	request, err := builder.Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the working context with an empty record: %v", err)
	}
	if blockIndex(request, BlockRecord) >= 0 {
		t.Errorf("a task with no record carries one: %s", blockNames(request))
	}
	recentAt := blockIndex(request, BlockRecentWork)
	if recentAt < 0 {
		t.Fatalf("the recent-work block is missing when the record is empty: %s", blockNames(request))
	}
	if request.SystemBlocks[recentAt].Boundary != contract.CacheBoundaryC {
		t.Errorf("with no record, the recent-work block does not end cache boundary C, so it is not in the cached prefix: boundary %q",
			request.SystemBlocks[recentAt].Boundary)
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
