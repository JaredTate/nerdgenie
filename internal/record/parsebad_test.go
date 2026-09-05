package record

import (
	"errors"
	"strings"
	"testing"
)

// brokenLine is one line of a golden record swapped for a broken one, with the
// name of what it breaks.
type brokenLine struct {
	name string
	from string
	to   string
}

// TestRefusesABrokenTaskRecord walks the task example, breaking one line at a
// time, and asks the parser to refuse each one with a message that says where.
func TestRefusesABrokenTaskRecord(t *testing.T) {
	checkEveryBreakIsRefused(t, "task.txt", brokenTaskLines())
}

// TestRefusesABrokenJobRecord does the same for the job example.
func TestRefusesABrokenJobRecord(t *testing.T) {
	checkEveryBreakIsRefused(t, "job.txt", brokenJobLines())
}

// brokenTaskLines is every way the task example can be broken one line at a time.
func brokenTaskLines() []brokenLine {
	header := "# task 17   running   from Signal   budget left: 86 rounds, 51 minutes"
	return []brokenLine{
		{"a first line that is not a header", header, "task 17"},
		{"a task number that is not a number", header, "# task seventeen   running   budget left: 86 rounds, 51 minutes"},
		{"a kind that is neither task nor job", header, "# thing 17   running   budget left: 86 rounds, 51 minutes"},
		{"a status nobody knows", header, "# task 17   sprinting   budget left: 86 rounds, 51 minutes"},
		{"a header field nobody knows", header, header + "   surprise"},
		{"a task header with no budget on it", header, "# task 17   running   from Signal"},
		{"a cost line that is not a cost line", "this turn: 6.1k tokens in, 5.2k of them cached, 0.4k out", "this turn: lots"},
		{"a heading nobody knows", "## Work", "## Notes"},
		{"the four headings out of their order", "## Rules", "## Work"},
		{"an item with no list open above it", "## Work", "## Work\n- a stray item"},
		{"a label nobody knows", "Situation:", "Notes:"},
		{"a done line marked done with nothing behind it",
			"- [x] it is under 280 characters and mentions the date -> r6",
			"- [x] it is under 280 characters and mentions the date ->"},
		{"a done line with no text on it", "- [ ] one post is up on the DigiByte account ->", "- [ ] "},
		{"a plan step marked done that names no result", "- [x] 2 draft the post -> r6", "- [x] 2 draft the post"},
		{"a plan step out of its numbering", "- [ ] 3 post it", "- [ ] 5 post it"},
		{"a plan step with no number", "- [ ] 3 post it", "- [ ] post it"},
		{"a result that counts backwards", "- r6 draft post, 236 characters", "- r1 draft post, 236 characters"},
		{"a result whose label is no result at all", "- r6 draft post, 236 characters", "- x6 draft post, 236 characters"},
		{"a result with no summary", "- r6 draft post, 236 characters", "- r6"},
		{"a correction that is not in the user's quotes",
			`- C1 "no, lead with the date not the features"`,
			"- C1 no, lead with the date not the features"},
		{"a correction whose label is no correction at all",
			`- C1 "no, lead with the date not the features"`,
			`- X1 "no, lead with the date not the features"`},
		{"a decision that carries no reason", "- D1 Lead with the date. Reason: correction C1.", "- D1 Lead with the date."},
		{"a decision with no full stop at the end", "- D1 Lead with the date. Reason: correction C1.", "- D1 Lead with the date. Reason: correction C1"},
		{"a failure that carries no cause",
			"- F1 Draft 1 was 312 characters. Cause: three facts in one post. Keep to one.",
			"- F1 Draft 1 was 312 characters."},
	}
}

// brokenJobLines is every way the job example can be broken one line at a time.
func brokenJobLines() []brokenLine {
	header := "# job 4   running   from Signal   3 of 12 tasks done   next: task 31 today at 14:00"
	return []brokenLine{
		{"a job header with no progress on it", header, "# job 4   running   from Signal"},
		{"a job number that is not a number, which its reports are labelled from",
			header, "# job four   running   from Signal   3 of 12 tasks done"},
		{"a job task marked done that names no report",
			"- [x] t17 post the anniversary tweet -> j4.1",
			"- [x] t17 post the anniversary tweet"},
		{"a job task whose identifier is not a task identifier",
			"- [ ] t31 post for day three · due today at 14:00",
			"- [ ] x31 post for day three · due today at 14:00"},
		{"job tasks that are not listed in order",
			"- [ ] t32 post for day four · due tomorrow at 14:00",
			"- [ ] t30 post for day four · due tomorrow at 14:00"},
		{"a job task with no text on it", "- [ ] t31 post for day three · due today at 14:00", "- [ ] t31"},
		{"a report that counts backwards",
			"- j4.2 draft saved to blog/anniversary.md, 900 words",
			"- j4.1 draft saved to blog/anniversary.md, 900 words"},
		{"a report that belongs to another job",
			"- j4.1 posted, 236 characters, link saved",
			"- j5.1 posted, 236 characters, link saved"},
		{"a plan under a job, which has tasks instead", "Tasks:", "Plan:"},
	}
}

// checkEveryBreakIsRefused makes each break in turn and asks the parser to say no.
func checkEveryBreakIsRefused(t *testing.T, name string, breaks []brokenLine) {
	t.Helper()
	golden := string(readGolden(t, name))
	for _, broken := range breaks {
		text := strings.Replace(golden, broken.from, broken.to, 1)
		if text == golden {
			t.Fatalf("the test for %q does not change %s, so its line no longer matches the golden record", broken.name, name)
		}
		if _, err := Parse([]byte(text)); err == nil {
			t.Errorf("the parser accepted %s:\n%s", broken.name, text)
		}
	}
}

// TestRefusesTextThatIsNoRecordAtAll covers the whole-text refusals: nothing at
// all, a header with no parts under it, and text past the two caps.
func TestRefusesTextThatIsNoRecordAtAll(t *testing.T) {
	golden := string(readGolden(t, "task.txt"))
	cases := map[string]string{
		"nothing at all":                       "",
		"a blank line and nothing else":        "\n",
		"a header with none of the four parts": "# task 17   running   budget left: 1 rounds, 1 minutes\nthis turn: 0.0k tokens in, 0.0k of them cached, 0.0k out\n",
		"a record that stops after the work":   golden[:strings.Index(golden, "## Lessons")],
		"more bytes than a record may have":    golden + strings.Repeat("x", MaxRecordBytes),
		"more lines than a record may have":    golden + strings.Repeat("\n", MaxRecordLines),
	}
	for name, text := range cases {
		if _, err := Parse([]byte(text)); err == nil {
			t.Errorf("the parser accepted %s", name)
		}
	}
}

// TestNamesTheRuleThatWasBroken proves a refusal carries the rule it broke and
// not only a message, so that a caller can tell one refusal from another with
// errors.Is and hand the model back the one line it needs.
func TestNamesTheRuleThatWasBroken(t *testing.T) {
	checkTheRuleIsNamed(t, "task.txt", map[string]error{
		"- [x] it is under 280 characters and mentions the date ->": ErrDoneLineNeedsProof,
		"- [x] 2 draft the post":                                    ErrPlanStepNeedsResult,
		"- r1 draft post, 236 characters":                           ErrIdentifiersOutOfOrder,
		"- D1 Lead with the date.":                                  ErrDecisionNeedsReason,
		"- F1 Draft 1 was 312 characters.":                          ErrFailureNeedsCause,
	}, map[string]string{
		"- [x] it is under 280 characters and mentions the date ->": "- [x] it is under 280 characters and mentions the date -> r6",
		"- [x] 2 draft the post":                                    "- [x] 2 draft the post -> r6",
		"- r1 draft post, 236 characters":                           "- r6 draft post, 236 characters",
		"- D1 Lead with the date.":                                  "- D1 Lead with the date. Reason: correction C1.",
		"- F1 Draft 1 was 312 characters.":                          "- F1 Draft 1 was 312 characters. Cause: three facts in one post. Keep to one.",
	})

	checkTheRuleIsNamed(t, "job.txt", map[string]error{
		"- [ ] t30 post for day four · due tomorrow at 14:00": ErrTasksOutOfOrder,
		"- [x] t17 post the anniversary tweet":                ErrJobTaskNeedsReport,
	}, map[string]string{
		"- [ ] t30 post for day four · due tomorrow at 14:00": "- [ ] t32 post for day four · due tomorrow at 14:00",
		"- [x] t17 post the anniversary tweet":                "- [x] t17 post the anniversary tweet -> j4.1",
	})
}

// checkTheRuleIsNamed swaps each good line for its broken one and asks that the
// error carry the rule the broken line breaks.
func checkTheRuleIsNamed(t *testing.T, name string, rules map[string]error, from map[string]string) {
	t.Helper()
	golden := string(readGolden(t, name))
	for broken, rule := range rules {
		text := strings.Replace(golden, from[broken], broken, 1)
		if text == golden {
			t.Fatalf("the test for %q does not change %s, so its line no longer matches the golden record", broken, name)
		}
		_, err := Parse([]byte(text))
		if !errors.Is(err, rule) {
			t.Errorf("the line %q was refused without naming its rule: %v", broken, err)
		}
	}
}

// TestRefusesAHeaderFieldThatIsOnlySpaces proves a header field padded with
// spaces is refused. The header separates its fields with three spaces, so a
// channel name or a next due task that begins or ends with one would lose it on
// the way back and the record would no longer say what it said. The fuzzing of
// the parser found this, and the input it found is in testdata beside it.
func TestRefusesAHeaderFieldThatIsOnlySpaces(t *testing.T) {
	parts := "\n## Goal\nAsk: \"do the thing\"\n\n## Rules\n\n## Work\n\n## Lessons\n"
	cost := "this turn: 0.0k tokens in, 0.0k of them cached, 0.0k out\n"
	headers := []string{
		"# job 1   running   0 of 0 tasks done   from  \n" + parts,
		"# job 1   running   from     0 of 0 tasks done\n" + parts,
		"# job 1   running   0 of 0 tasks done   next:  today\n" + parts,
		"# job 1   running   0 of 0 tasks done   next: today \n" + parts,
		"# task 1   running   from     budget left: 0 rounds, 0 minutes\n" + cost + parts,
	}
	for _, header := range headers {
		if _, err := Parse([]byte(header)); err == nil {
			t.Errorf("a header field padded with spaces read back anyway:\n%s", header)
		}
	}
}

// TestSaysWhichLineIsWrong proves a parse error points the reader at the line to
// fix, because an error message that does not say where wastes the reader's time.
func TestSaysWhichLineIsWrong(t *testing.T) {
	golden := string(readGolden(t, "task.txt"))
	text := strings.Replace(golden, "Situation:", "Notes:", 1)
	_, err := Parse([]byte(text))
	if err == nil {
		t.Fatal("the parser accepted a label nobody knows")
	}
	if !strings.Contains(err.Error(), "line 19") {
		t.Errorf("the error does not name line 19, where the broken label is: %v", err)
	}
}
