package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// aStatusWithTheStateBlock is what the program says in the middle of a task
// whose record has a situation, two failures, a round and a start: the plan
// status, with everything the header's meters read and the record's own
// state as the second look draws it.
func aStatusWithTheStateBlock() contract.SocketEnvelope {
	status := aStatusWithAPlan()
	for name, value := range aStatusWithEverything().Fields {
		status.Fields[name] = value
	}
	status.Fields[contract.StatusFieldTask] = "17"
	status.Fields[contract.StatusFieldTaskAsk] = theTaskAsk
	status.Fields[contract.StatusFieldCallStarted] = startOfTest.Add(-14 * time.Second).Format(time.RFC3339)
	status.Fields[contract.StatusFieldStreamed] = "212"
	status.Fields[contract.StatusFieldSituation] = "tests: all 51 passing\n" +
		"last command: npm test, exit 0\n" +
		"files changed in this task: game.js, index.html and 3 more"
	status.Fields[contract.StatusFieldFailures] = "F1 Draft 1 was 312 characters. Cause: three facts in one post.\n" +
		"F2 The build broke on the second run. Cause: a missing import in game.js that the first run did not need."
	return status
}

// panelRowsOf is the panel's rows as a person reads them, edge and blank taken
// off, with the blanks on the end trimmed.
func panelRowsOf(screen *Screen) []string {
	rows := []string{}
	for _, line := range checklistRowsOf(screen) {
		rows = append(rows, strings.TrimRight(line, " "))
	}
	return rows
}

// TestThePanelIsTwentyEightColumnsUnderAHundredAndFortyAndAThirdFromThere
// holds the width rule: twenty-eight columns on a screen under a hundred and
// forty, a third of the screen at a hundred and forty and over, and never more
// than fifty-six.
func TestThePanelIsTwentyEightColumnsUnderAHundredAndFortyAndAThirdFromThere(t *testing.T) {
	for _, one := range []struct{ width, panel int }{{100, 28}, {139, 28}, {140, 46}, {150, 50}, {168, 56}, {200, 56}} {
		screen, _ := newTestScreen(one.width, 40)
		if got := screen.panelWidth(); got != one.panel {
			t.Errorf("at %d columns the panel is %d wide, want %d", one.width, got, one.panel)
		}
		if got := screen.width - screen.transcriptColumns(); got != one.panel {
			t.Errorf("at %d columns the transcript leaves %d for the panel, want %d", one.width, got, one.panel)
		}
		if got := screen.panelTextWidth(); got != one.panel-3 {
			t.Errorf("at %d columns the panel's words are %d wide, want %d", one.width, got, one.panel-3)
		}
	}
	if widestPanelColumns != 56 || wideScreenFrom != 140 {
		t.Errorf("the panel is capped at %d from %d columns, and the brief says fifty-six from a hundred and forty", widestPanelColumns, wideScreenFrom)
	}
}

// TestThePanelNamesTheJobAndTaskRowsAsClickTargets holds the seam the other
// worker's clicks land on: one target per line the panel draws, "job:4" on
// the job's header, "task:t19" on each of its task rows, "task:17" on a plain
// task's header, and nothing on every other line.
func TestThePanelNamesTheJobAndTaskRowsAsClickTargets(t *testing.T) {
	screen, _ := newTestScreen(120, 40)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithANamedJob())
	lines, targets := screen.panelLines(), screen.panelTargets()
	if len(lines) != len(targets) {
		t.Fatalf("the panel draws %d lines and names %d targets", len(lines), len(targets))
	}
	named := map[string]string{}
	for at, line := range lines {
		text := ""
		for _, piece := range line.spans {
			text += piece.text
		}
		if targets[at] != "" {
			named[targets[at]] = text
		}
	}
	for target, prefix := range map[string]string{"job:4": "JOB 4", "task:t17": "✓ t17", "task:t19": "▶ t19", "task:t22": "○ t22"} {
		if !strings.HasPrefix(named[target], prefix) {
			t.Errorf("the target %q names the line %q, and it should name the one beginning %q", target, named[target], prefix)
		}
	}
	if len(named) != 4 {
		t.Errorf("the panel names %d targets %v, and a job of three tasks has four", len(named), named)
	}

	plain, _ := newTestScreen(120, 40)
	plain.Update(linkMessage{up: true})
	send(plain, aStatusWithAPlan())
	found := false
	for at, target := range plain.panelTargets() {
		if target == "task:17" {
			found = true
			if text := plain.panelLines()[at].spans[0].text; text != "TASK" {
				t.Errorf("the target task:17 names a line beginning %q, and it is the task's header", text)
			}
		} else if target != "" {
			t.Errorf("a plain task's panel names the target %q, and only its header is a target", target)
		}
	}
	if !found {
		t.Error("a plain task's header is not a click target")
	}
}

// TestThePanelGroupsAreLabelledInOrderAndLeftOutWhenEmpty holds the groups
// and their order: MODEL, NOW, the checklist, STATE, FAILURES, ROUND and the
// count of jobs, each label dim and upper-case with the checklist's rule after
// it, and a group with nothing to say left out entirely.
func TestThePanelGroupsAreLabelledInOrderAndLeftOutWhenEmpty(t *testing.T) {
	screen, _ := newTestScreen(120, 60)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithTheStateBlock())
	rows := panelRowsOf(screen)

	last := -1
	for _, label := range []string{"MODEL " + string(ruleGlyph), "NOW " + string(ruleGlyph), "TASK 17", "STATE " + string(ruleGlyph), "FAILURES " + string(ruleGlyph), "ROUND " + string(ruleGlyph), "3 jobs waiting"} {
		at := rowStarting(rows, label)
		if at < 0 {
			t.Errorf("no row of the panel begins %q:\n%s", label, strings.Join(rows, "\n"))
			continue
		}
		if at <= last {
			t.Errorf("the group %q comes before the one it should follow:\n%s", label, strings.Join(rows, "\n"))
		}
		if at > 0 && rows[at-1] != "" {
			t.Errorf("the group %q has no blank line above it:\n%s", label, strings.Join(rows, "\n"))
		}
		last = at
	}
	for _, line := range rows {
		if strings.HasSuffix(line, string(ruleGlyph)) && displayWidth(line) != screen.panelTextWidth() {
			t.Errorf("the label %q runs to %d columns, and its rule reaches the panel's edge at %d", line, displayWidth(line), screen.panelTextWidth())
		}
	}

	quiet, _ := newTestScreen(120, 40)
	quiet.Update(linkMessage{up: true})
	send(quiet, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{contract.StatusFieldModel: "local", contract.StatusFieldState: contract.StateIdle}})
	panel := strings.Join(panelRowsOf(quiet), "\n")
	for _, unwanted := range []string{"STATE", "FAILURES", "ROUND", "TASK", "JOB", "cache"} {
		if strings.Contains(panel, unwanted) {
			t.Errorf("the panel draws %q for a program that said nothing about it:\n%s", unwanted, panel)
		}
	}

	themed := newThemedScreen(120, 60)
	themed.Update(linkMessage{up: true})
	send(themed, aStatusWithTheStateBlock())
	if frame := themed.frame(); !strings.Contains(frame, themed.colors.wrap(styleDim, "STATE ")+themed.colors.wrap(styleBrand, strings.Repeat(string(ruleGlyph), themed.panelTextWidth()-6))) {
		t.Error("the label STATE is not dim with the rule in DigiByte's own blue after it")
	}
}

// TestTheModelGroupSaysTheAliasTheMeterTheCacheAndTheCost holds the first
// group: the alias, the context meter with its numbers, the cache share in
// its own colour, and the cost.
func TestTheModelGroupSaysTheAliasTheMeterTheCacheAndTheCost(t *testing.T) {
	screen := newThemedScreen(120, 60)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithTheStateBlock())
	rows := panelRowsOf(screen)
	start := rowStarting(rows, "MODEL")
	for at, wanted := range []string{"MODEL", "opus", "▰▱▱▱▱▱▱▱▱▱ 26.8k/262k 10%", "cache 92%", "6.1k in 0.4k out · $0.04"} {
		if start < 0 || start+at >= len(rows) || !strings.HasPrefix(rows[start+at], wanted) {
			t.Errorf("row %d of the model group is %q, want it to begin %q:\n%s", at+1, rows[start+at], wanted, strings.Join(rows, "\n"))
		}
	}
	if !strings.Contains(screen.frame(), screen.colors.wrap(styleDone, "cache 92%")) {
		t.Error("the cache share in the panel is not green at ninety-two percent")
	}
}

// TestTheNowGroupSaysWhatIsHappening holds the second group: the state word,
// the call in flight from the last tool line, the seconds and the tokens of
// the model call, or "idle" when nothing is happening, and nothing at all
// before the program has reported itself.
func TestTheNowGroupSaysWhatIsHappening(t *testing.T) {
	screen, _ := newTestScreen(120, 60)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithTheStateBlock())
	send(screen, aToolLine("▸ shell npm test"))
	rows := panelRowsOf(screen)
	start := rowStarting(rows, "NOW")
	for at, wanted := range []string{"NOW", "thinking", string(runningGlyph) + " shell npm test", "14 s · 212 tokens"} {
		if start < 0 || start+at >= len(rows) || !strings.HasPrefix(rows[start+at], wanted) {
			t.Errorf("row %d of the now group is %q, want it to begin %q:\n%s", at+1, rows[start+at], wanted, strings.Join(rows, "\n"))
		}
	}

	send(screen, aToolLine("▸ shell npm test · r27 tests: all 51 passing"))
	if rows := panelRowsOf(screen); rowStarting(rows, string(runningGlyph)+" shell") >= 0 {
		t.Errorf("the now group still shows a call that has its result:\n%s", strings.Join(rows, "\n"))
	}
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{contract.StatusFieldState: contract.StateIdle}})
	rows = panelRowsOf(screen)
	if start := rowStarting(rows, "NOW"); start < 0 || rows[start+1] != "idle" || rows[start+2] != "" {
		t.Errorf("an idle program's now group is not the one word idle:\n%s", strings.Join(rows, "\n"))
	}

	fresh, _ := newTestScreen(120, 40)
	if rows := panelRowsOf(fresh); rowStarting(rows, "NOW") >= 0 {
		t.Errorf("the now group is drawn before the program has reported itself:\n%s", strings.Join(rows, "\n"))
	}
}

// TestTheStateGroupShowsEachFactUnderAShortLabelWithTheTestsLineColoured
// holds the record's situation on the panel: each fact under the panel's
// short word for its label, dim, the tests line green when it says all and
// red when it says failing, and a fact longer than the panel run onto a
// second row rather than cut, because "files changed in this task:" alone was
// the whole of a twenty-eight column row and the files were never seen.
func TestTheStateGroupShowsEachFactUnderAShortLabelWithTheTestsLineColoured(t *testing.T) {
	// At a hundred and sixty columns the panel's words are fifty wide, which
	// holds the first two facts whole and runs the third onto a second row.
	screen := newThemedScreen(160, 60)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithTheStateBlock())
	rows := panelRowsOf(screen)
	start := rowStarting(rows, "STATE")
	for at, wanted := range []string{"STATE", "tests: all 51 passing", "ran: npm test, exit 0", "files: game.js, index.html"} {
		if start < 0 || start+at >= len(rows) || !strings.HasPrefix(rows[start+at], wanted) {
			t.Errorf("row %d of the state group is %q, want it to begin %q:\n%s", at+1, rows[start+at], wanted, strings.Join(rows, "\n"))
		}
	}
	for at := start + 1; at < len(rows) && rows[at] != ""; at++ {
		if strings.HasSuffix(rows[at], string(ellipsisGlyph)) || displayWidth(rows[at]) > screen.panelTextWidth() {
			t.Errorf("row %q of the state group is cut or too wide, and a long fact runs onto a second row instead", rows[at])
		}
	}
	colors := screen.colors
	frame := screen.frame()
	if !strings.Contains(frame, colors.wrap(styleDim, "tests:")+colors.wrap(styleDone, " all 51 passing")) {
		t.Error("the tests line is not a dim label and green words")
	}
	if !strings.Contains(frame, colors.wrap(styleDim, "ran:")+colors.wrap(styleNormal, " npm test, exit 0")) {
		t.Error("the last command line is not a dim label and plain words")
	}

	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{contract.StatusFieldSituation: "tests: 2 failing of 51"}})
	if !strings.Contains(screen.frame(), colors.wrap(styleDim, "tests:")+colors.wrap(styleBad, " 2 failing of 51")) {
		t.Error("a tests line that says failing is not red")
	}
}

// TestTheFailuresGroupCountsThemAndShowsTheNewestOnTwoLines holds the
// failures on the panel: the count in red, and the newest failure wrapped to
// two lines with the rest cut, so a long cause is not the whole panel.
func TestTheFailuresGroupCountsThemAndShowsTheNewestOnTwoLines(t *testing.T) {
	screen := newThemedScreen(120, 60)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithTheStateBlock())
	rows := panelRowsOf(screen)
	start := rowStarting(rows, "FAILURES")
	if start < 0 || rows[start+1] != "2 failures" {
		t.Fatalf("the failures group does not count two failures:\n%s", strings.Join(rows, "\n"))
	}
	if !strings.HasPrefix(rows[start+2], "F2 The build broke") {
		t.Errorf("the failures group shows %q first, and it shows the newest failure", rows[start+2])
	}
	if rows[start+3] == "" || !strings.HasSuffix(rows[start+3], string(ellipsisGlyph)) || rows[start+4] != "" {
		t.Errorf("the newest failure is drawn as %q and %q, and it is two lines with the rest cut:\n%s", rows[start+3], rows[start+4], strings.Join(rows, "\n"))
	}
	if strings.Contains(strings.Join(rows, "\n"), "F1 Draft") {
		t.Error("the failures group shows an older failure, and it shows the newest alone")
	}
	if !strings.Contains(screen.frame(), screen.colors.wrap(styleBad, "2 failures")) {
		t.Error("the count of failures is not red")
	}

	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{contract.StatusFieldFailures: "F1 One thing went wrong. Cause: a small slip."}})
	if rows := panelRowsOf(screen); rowStarting(rows, "1 failure") < 0 || rowStarting(rows, "1 failures") >= 0 {
		t.Errorf("one failure is not counted as \"1 failure\":\n%s", strings.Join(rows, "\n"))
	}
}

// TestTheRoundGroupSaysTheRoundAndHowLongTheTaskHasRun holds the last group
// before the count of jobs: "round 27 · 12m in", the round alone when the
// start is unknown, and nothing when the program has not said the round.
func TestTheRoundGroupSaysTheRoundAndHowLongTheTaskHasRun(t *testing.T) {
	screen, _ := newTestScreen(120, 60)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithTheStateBlock())
	rows := panelRowsOf(screen)
	if start := rowStarting(rows, "ROUND"); start < 0 || rows[start+1] != "round 27 · 12m in" {
		t.Errorf("the round group does not say the round and the time:\n%s", strings.Join(rows, "\n"))
	}

	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{contract.StatusFieldTaskStarted: ""}})
	rows = panelRowsOf(screen)
	if start := rowStarting(rows, "ROUND"); start < 0 || rows[start+1] != "round 27" {
		t.Errorf("the round group with no start does not say the round alone:\n%s", strings.Join(rows, "\n"))
	}

	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{contract.StatusFieldRound: ""}})
	if rows := panelRowsOf(screen); rowStarting(rows, "ROUND") >= 0 {
		t.Errorf("the round group is drawn with no round to say:\n%s", strings.Join(rows, "\n"))
	}
}

// TestNoPanelRowIsWiderThanThePanelAtAnyWidth holds the bound every group
// keeps: on a screen of each width, with everything the panel can say, no row
// runs past the panel's words and the transcript beside it is never touched.
func TestNoPanelRowIsWiderThanThePanelAtAnyWidth(t *testing.T) {
	for _, width := range []int{100, 120, 140, 160, 200} {
		screen, _ := newTestScreen(width, 60)
		screen.Update(linkMessage{up: true})
		send(screen, aStatusWithTheStateBlock())
		send(screen, aToolLine("▸ shell npm run build -- --watch --verbose --and-a-very-long-argument-list"))
		for _, line := range screen.panelLines() {
			if line.width > screen.panelTextWidth() {
				t.Errorf("at %d columns the panel row %v is %d wide, and the panel's words are %d", width, line.spans, line.width, screen.panelTextWidth())
			}
		}
		for number, drawn := range strings.Split(plainText(screen.frame()), "\n") {
			if displayWidth(drawn) > width {
				t.Errorf("at %d columns row %d is %d wide", width, number+1, displayWidth(drawn))
			}
		}
	}
}
