package record

import (
	"reflect"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The tests in this file hold the package to its central promise: a record and
// its text always say the same thing. A record that printed one way and read back
// another would quietly rewrite the user's own words the first time a task was
// put down and picked up, which is the one thing this package exists to prevent.

// TestRefusesAnEditThatWouldNotReadBack proves an edit whose printed form would
// read back as something else is refused rather than saved.
func TestRefusesAnEditThatWouldNotReadBack(t *testing.T) {
	ctx := t.Context()
	task, _ := newKeeper(t, taskStart())
	job, _ := newKeeper(t, jobStart())

	refused := map[string]func() error{
		"a plan step with an arrow in it": func() error {
			return task.Apply(ctx, Update{Plan: []string{"read the notes -> r1"}})
		},
		"a plan step that ends in an arrow": func() error {
			return task.Apply(ctx, Update{Plan: []string{"trailing arrow ->"}})
		},
		"a decision whose reason repeats the join": func() error {
			return task.Apply(ctx, Update{Decision: &NewDecision{Text: "go left", Reason: "it is short. Reason: it is safe"}})
		},
		"a failure whose cause repeats the join": func() error {
			return task.Apply(ctx, Update{Failure: &NewFailure{Text: "it broke", Cause: "one thing. Cause: another"}})
		},
		"a correction longer than a record may be": func() error {
			_, err := task.AddCorrection(ctx, strings.Repeat("x", MaxRecordBytes+1))
			return err
		},
		"a task whose text carries the due mark": func() error {
			return job.Apply(ctx, Update{Tasks: []NewJobTask{{TaskID: "t1", Text: "post the tweet" + dueMark + "then the blog"}}})
		},
	}

	for what, write := range refused {
		before := textOf(t, task, job, what)
		if err := write(); err == nil {
			t.Errorf("%s was written, and it does not read back as what was written", what)
		}
		if after := textOf(t, task, job, what); after != before {
			t.Errorf("%s changed the record although it was refused:\n%s", what, after)
		}
	}
}

// textOf returns the text of whichever record the named case writes to.
func textOf(t *testing.T, task *Keeper, job *Keeper, what string) string {
	t.Helper()
	if strings.HasPrefix(what, "a task ") {
		return job.Text()
	}
	return task.Text()
}

// TestRefusesARecordWhoseHeaderWouldNotReadBack proves the same check guards the
// moment a record is created, where the channel it came from is written.
func TestRefusesARecordWhoseHeaderWouldNotReadBack(t *testing.T) {
	start := taskStart()
	start.Origin = "Signal   room"
	if _, err := newKeeperOrError(t, start); err == nil {
		t.Error("a record was created with an origin that its header cannot hold")
	}
}

// TestADoneLineWhoseTextEndsInAnArrowReadsBack is the counter-example the fuzzing
// of the parser is meant to catch: the arrow is drawn on every line once any line
// needs one, so a line whose own words end in an arrow keeps them.
func TestADoneLineWhoseTextEndsInAnArrowReadsBack(t *testing.T) {
	held := contract.Record{
		Header: contract.Header{Kind: contract.RecordTask, ID: "1", Status: contract.StatusRunning},
		Goal: contract.Goal{Ask: "do the thing", DoneWhen: []contract.DoneLine{
			{Text: "a ->"},
			{Text: "an ordinary line"},
		}},
	}
	printed := Print(held)
	read, err := Parse(printed)
	if err != nil {
		t.Fatalf("a done line ending in an arrow does not read back: %v\n%s", err, printed)
	}
	if !reflect.DeepEqual(read, held) {
		t.Errorf("a done line ending in an arrow came back as something else.\nwant %+v\ngot  %+v\n%s", held, read, printed)
	}
}

// TestParseWantsTheUsersAsk proves a record that names no ask is refused, because
// the rule that the ask is never edited protects nothing when there is none.
func TestParseWantsTheUsersAsk(t *testing.T) {
	golden := string(readGolden(t, "task.txt"))
	without := strings.Replace(golden,
		"Ask: \"Post a tweet about the DigiByte anniversary. Use the product notes and keep it under 280 characters.\"\n", "", 1)
	if without == golden {
		t.Fatal("this test no longer takes the ask out of the golden record")
	}
	if _, err := Parse([]byte(without)); err == nil {
		t.Error("a record with no ask on it read back anyway")
	}
}

// TestCorrectionsCountWithNoGapsAndLessonsOnlyUpward proves the rule that a
// correction gets the next label and is never removed, so its labels have no
// gaps, while a decision or a failure may start anywhere, because the oldest
// lessons leave the record, and must only count upward, so that nothing a
// reader has seen is ever handed out twice.
func TestCorrectionsCountWithNoGapsAndLessonsOnlyUpward(t *testing.T) {
	golden := string(readGolden(t, "task.txt"))
	refused := map[string]string{
		`- C1 "no, lead with the date not the features"`:                                `- C2 "no, lead with the date not the features"`,
		"- D1 Lead with the date. Reason: correction C1.":                               "- D3 Lead with the date. Reason: correction C1.\n- D2 Keep the short form. Reason: correction C1.",
		"- F1 Draft 1 was 312 characters. Cause: three facts in one post. Keep to one.": "- F9 Draft 1 was 312 characters. Cause: three facts in one post. Keep to one.\n- F9 Draft 2 was 300 characters. Cause: the same three facts.",
	}
	for good, bad := range refused {
		text := strings.Replace(golden, good, strings.ReplaceAll(bad, "\\n", "\n"), 1)
		if text == golden {
			t.Fatalf("this test no longer changes the line %q", good)
		}
		if _, err := Parse([]byte(text)); err == nil {
			t.Errorf("a record with the labels %q in it read back anyway", bad)
		}
	}
	allowed := map[string]string{
		"- D1 Lead with the date. Reason: correction C1.":                               "- D3 Lead with the date. Reason: correction C1.",
		"- F1 Draft 1 was 312 characters. Cause: three facts in one post. Keep to one.": "- F9 Draft 1 was 312 characters. Cause: three facts in one post. Keep to one.",
	}
	for good, later := range allowed {
		text := strings.Replace(golden, good, later, 1)
		if _, err := Parse([]byte(text)); err != nil {
			t.Errorf("a record whose lessons start at %q was refused, and the oldest lessons leave a record: %v", later, err)
		}
	}
}
