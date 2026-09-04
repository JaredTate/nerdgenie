package memory_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// theWithdrawnSecret is the one word in the fact below that must never reach the
// model again once the user has withdrawn the fact holding it.
const theWithdrawnSecret = "hunter2"

// theWithdrawnFact is exactly one hundred and twenty-three runes long, which is
// three more than one hint line may be. That length is the whole point: a mark
// added to the end of the text saying the fact was withdrawn is cut off again
// before the line reaches the model, and the secret in the middle is not.
const theWithdrawnFact = "the greenhouse alarm code for the loading docks is " +
	theWithdrawnSecret + " and nobody outside this office may ever be told what it is today"

// theStepThatWouldFindIt is the text of a step whose words match the withdrawn
// fact, which is what a hint is built from.
const theStepThatWouldFindIt = "what is the greenhouse alarm code for the loading docks"

func TestAWithdrawnFactNeverReachesTheHint(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	if len([]rune(theWithdrawnFact)) != 123 {
		t.Fatalf("the fact under test is %d runes, and this test needs the 123 the reviewer used",
			len([]rune(theWithdrawnFact)))
	}
	if err := opened.memory.Save(ctx, []contract.Fact{
		{ID: "m1", Text: theWithdrawnFact, Source: "task 17"},
	}); err != nil {
		t.Fatalf("cannot save the fact that is about to be withdrawn: %v", err)
	}
	if reply := opened.runMemoryCommand(t, "forget m1"); !strings.Contains(reply, "m1 is withdrawn") {
		t.Fatalf("withdrawing the fact says:\n%s", reply)
	}

	hint, err := opened.memory.Hint(ctx, theStepThatWouldFindIt)
	if err != nil {
		t.Fatalf("cannot ask for a hint after the fact was withdrawn: %v", err)
	}
	for _, line := range hint {
		if strings.Contains(line, theWithdrawnSecret) {
			t.Errorf("the hint line %q carries the secret out of a fact the user withdrew", line)
		}
	}
}

func TestWithdrawingAFactWritesDownOnlyItsID(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	if err := opened.memory.Save(ctx, []contract.Fact{
		{ID: "m1", Text: theWithdrawnFact, Source: "task 17"},
	}); err != nil {
		t.Fatalf("cannot save the fact that is about to be withdrawn: %v", err)
	}
	opened.runMemoryCommand(t, "forget m1")

	found, err := opened.memory.Search(ctx, "withdrew", 10)
	if err != nil {
		t.Fatalf("cannot search for the withdrawal: %v", err)
	}
	withdrawal := ""
	for _, fact := range found {
		if fact.ID != "m1" {
			withdrawal = fact.Text
		}
	}
	if withdrawal == "" {
		t.Fatalf("the withdrawal itself is not searchable, and the search found %v", factTexts(found))
	}
	if !strings.Contains(withdrawal, "m1") {
		t.Errorf("the withdrawal says %q, and it must name the fact it withdrew", withdrawal)
	}
	if strings.Contains(withdrawal, theWithdrawnSecret) {
		t.Errorf("the withdrawal says %q, and it copied the withdrawn text into a fact that is still live", withdrawal)
	}
}

func TestASupersededFactSaysSoAtTheFrontOfTheLine(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	if err := opened.memory.Save(ctx, []contract.Fact{
		{ID: "m1", Text: theWithdrawnFact, Source: "task 17"},
	}); err != nil {
		t.Fatalf("cannot save the first fact: %v", err)
	}
	if err := opened.memory.Save(ctx, []contract.Fact{
		{ID: "m2", Text: "the loading docks have no alarm at all", Source: "task 19", Supersedes: "m1"},
	}); err != nil {
		t.Fatalf("cannot supersede the first fact: %v", err)
	}

	older, err := opened.memory.Get(ctx, "m1")
	if err != nil {
		t.Fatalf("cannot read the superseded fact back: %v", err)
	}
	if !strings.HasPrefix(older.Text, "(superseded by m2)") {
		t.Errorf("the superseded fact reads %q, and the mark has to be at the front of the line, "+
			"because anything on the end is cut off before a reader sees it", older.Text)
	}
}

func TestTheHintIsEmptyWhenAStepSharesOnlyStopWordsWithMemory(t *testing.T) {
	opened := newMemory(t, shippedCaps)
	ctx := context.Background()
	for _, fact := range []string{
		"the anniversary is on the tenth of January",
		"the blog is built with Hugo and the theme is ananke",
		"the user posts in the morning and never in the evening",
	} {
		opened.saveWorldFact(t, fact)
	}

	hint, err := opened.memory.Hint(ctx, "and then the one of them is on it for a while")
	if err != nil {
		t.Fatalf("cannot ask for a hint for a step made of stop words: %v", err)
	}
	if len(hint) != 0 {
		t.Errorf("the hint for a step sharing only stop words is %v, and it must be empty", hint)
	}
}
