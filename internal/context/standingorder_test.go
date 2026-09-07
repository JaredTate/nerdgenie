package context

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theStandingOrder is a project's AGENTS.md as a person would write it.
const theStandingOrder = `# Tater Tots Tetris

## What this is
A browser Tetris with two hazards.

## Run and test
- npm start serves it on 8091
- npm test runs the whole suite

## Rules
- Tests first.
- Hazard values live in src/config.js.
`

// TestTheStandingOrderRidesUnderTheJobSummary: a folder's AGENTS.md is in
// front of the model on every call, under the job summary and before the
// record's goal and rules, under a heading that names the file and its length.
func TestTheStandingOrderRidesUnderTheJobSummary(t *testing.T) {
	run := newFixtureRun(t)
	run.playTo(t, 20)
	input := run.input(24000)
	input.JobSummary = "# job 3   running   0 of 4 tasks done\n\nTasks:\n- [ ] t1 write the post\n- [ ] t2 post it"
	input.StandingOrder = theStandingOrder

	request, err := newGoldenBuilder(t).Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the working context with a standing order: %v", err)
	}
	whole := renderPrompt(request)
	testkit.Golden(t, "prompt-standing-order.txt", []byte(whole))

	heading := "The project's standing order (AGENTS.md, 12 lines):"
	job := strings.Index(whole, input.JobSummary)
	order := strings.Index(whole, heading)
	rules := strings.Index(whole, recordFirstHalfHeading)
	if job < 0 || order < 0 || rules < 0 || !(job < order && order < rules) {
		t.Fatalf("the standing order must sit after the job summary (%d) and before the record's goal and rules (%d); its heading is at %d", job, rules, order)
	}
	if !strings.Contains(whole, "Hazard values live in src/config.js.") {
		t.Errorf("the standing order's text is not in the prompt")
	}
	for _, block := range request.SystemBlocks {
		if strings.Contains(block.Text, heading) {
			t.Errorf("the standing order rides in the system block %s, and it belongs below the tools with the job summary", block.Name)
		}
	}
}

// TestAFolderWithoutAStandingOrderAddsNothing: with no AGENTS.md the prompt is
// the same bytes as before the standing order existed.
func TestAFolderWithoutAStandingOrderAddsNothing(t *testing.T) {
	run := newFixtureRun(t)
	run.playTo(t, 20)
	request, err := newGoldenBuilder(t).Build(t.Context(), run.input(24000))
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}
	if strings.Contains(renderPrompt(request), "standing order") {
		t.Errorf("a prompt with no standing order names one")
	}
}

// TestAStandingOrderOverSixtyLinesIsCutAndSaysSo: a long AGENTS.md is shown
// to its sixtieth line and one line says how to read the rest, so a rules
// file cannot crowd the task out of the window.
func TestAStandingOrderOverSixtyLinesIsCutAndSaysSo(t *testing.T) {
	var long strings.Builder
	for line := 1; line <= 80; line++ {
		fmt.Fprintf(&long, "- rule number %d\n", line)
	}
	run := newFixtureRun(t)
	run.playTo(t, 20)
	input := run.input(24000)
	input.StandingOrder = long.String()

	request, err := newGoldenBuilder(t).Build(t.Context(), input)
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}
	whole := renderPrompt(request)
	if !strings.Contains(whole, "The project's standing order (AGENTS.md, 80 lines):") {
		t.Errorf("the heading does not say the file is 80 lines")
	}
	if !strings.Contains(whole, "- rule number 60\n") || strings.Contains(whole, "- rule number 61\n") {
		t.Errorf("the standing order is not cut at its sixtieth line")
	}
	if !strings.Contains(whole, TheStandingOrderCutLine) {
		t.Errorf("the cut standing order does not say how to read the rest")
	}
}
