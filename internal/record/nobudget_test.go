package record

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The tests in this file hold the user's rule on the record's side: a task has
// no budget unless the user set one, and the header says so. With neither limit
// set it reads "no budget"; with one set it names only that one; with both set
// it reads as it always has.

// aTaskHeader is a task header with the given budget on it and nothing optional.
func aTaskHeader(roundsLeft int, noRounds bool, minutesLeft int, noMinutes bool) contract.Header {
	return contract.Header{
		Kind: contract.RecordTask, ID: "9", Status: contract.StatusRunning,
		RoundsLeft: roundsLeft, NoRoundBudget: noRounds,
		MinutesLeft: minutesLeft, NoTimeBudget: noMinutes,
	}
}

// budgetForm is one way a task header can say what is left.
type budgetForm struct {
	name   string
	header contract.Header
	field  string
}

// theFormsOfTheBudget is every way a task header can say what is left.
func theFormsOfTheBudget() []budgetForm {
	return []budgetForm{
		{"no budget at all", aTaskHeader(0, true, 0, true), "no budget"},
		{"rounds only", aTaskHeader(86, false, 0, true), "budget left: 86 rounds"},
		{"minutes only", aTaskHeader(0, true, 51, false), "budget left: 51 minutes"},
		{"both", aTaskHeader(86, false, 51, false), "budget left: 86 rounds, 51 minutes"},
		{"a round budget that is spent", aTaskHeader(0, false, 0, true), "budget left: 0 rounds"},
	}
}

func TestPrintsTheBudgetAsWhatIsSet(t *testing.T) {
	for _, form := range theFormsOfTheBudget() {
		first := firstLine(string(Print(contract.Record{Header: form.header})))
		if want := "# task 9   running   " + form.field; first != want {
			t.Errorf("the header with %s reads %q, want %q", form.name, first, want)
		}
	}
}

func TestReadsBackEveryFormOfTheBudget(t *testing.T) {
	for _, form := range theFormsOfTheBudget() {
		printed := Print(contract.Record{Header: form.header, Goal: contract.Goal{Ask: "do it"}})
		read, err := Parse(printed)
		if err != nil {
			t.Errorf("the header with %s does not read back: %v\n%s", form.name, err, printed)
			continue
		}
		if read.Header != form.header {
			t.Errorf("the header with %s read back as %+v, want %+v", form.name, read.Header, form.header)
		}
		if again := Print(read); string(again) != string(printed) {
			t.Errorf("the header with %s changed on a round trip:\n%s\n%s", form.name, printed, again)
		}
	}
}

// TestRefusesABudgetWrittenAnyOtherWay holds the parser to the four forms and
// nothing else, so that a header cannot say two things about one limit or put
// the minutes before the rounds and be read as something else.
func TestRefusesABudgetWrittenAnyOtherWay(t *testing.T) {
	good := string(Print(contract.Record{Header: aTaskHeader(86, false, 51, false), Goal: contract.Goal{Ask: "do it"}}))
	for _, bad := range []string{
		"budget left: 51 minutes, 86 rounds",
		"budget left: 86 rounds, 86 rounds",
		"budget left: 86 rounds, 51 minutes, 3 seconds",
		"budget left: -1 rounds",
		"budget left: 086 rounds",
		"budget left: 86 rounds,",
		"budget left:",
		"budget left: ",
		"no budget left",
		"no budget, 51 minutes",
	} {
		text := strings.Replace(good, "budget left: 86 rounds, 51 minutes", bad, 1)
		if _, err := Parse([]byte(text)); err == nil {
			t.Errorf("the parser accepted the budget field %q", bad)
		}
	}
}

// TestTheHarnessWritesABudgetWithEitherLimitOff is the keeper's side: the loop
// writes what is left on the limits the task has, and the header keeps only
// those.
func TestTheHarnessWritesABudgetWithEitherLimitOff(t *testing.T) {
	keeper, _ := newKeeper(t, taskStart())
	ctx := t.Context()

	if err := keeper.SetBudget(ctx, Budget{RoundsLeft: 7, NoRoundBudget: true, MinutesLeft: 51}); err != nil {
		t.Fatalf("cannot write a budget with the rounds off: %v", err)
	}
	held := keeper.Record().Header
	if !held.NoRoundBudget || held.RoundsLeft != 0 || held.NoTimeBudget || held.MinutesLeft != 51 {
		t.Errorf("the header is %+v after a budget with the rounds off was written, and a count on a limit that is off is not kept", held)
	}
	if first := firstLine(string(Print(keeper.Record()))); !strings.HasSuffix(first, "budget left: 51 minutes") {
		t.Errorf("the header reads %q, want it to name only the minutes", first)
	}

	if err := keeper.SetBudget(ctx, Budget{NoRoundBudget: true, NoTimeBudget: true}); err != nil {
		t.Fatalf("cannot write no budget at all: %v", err)
	}
	if first := firstLine(string(Print(keeper.Record()))); !strings.HasSuffix(first, "no budget") {
		t.Errorf("the header reads %q, want it to say there is no budget", first)
	}
	if err := keeper.SetBudget(ctx, Budget{RoundsLeft: -1, NoRoundBudget: true}); err == nil {
		t.Error("the budget took a count below zero on a limit that is off, and a count below zero is never a budget")
	}
}

// TestARecordStartsWithNoBudgetWhenNoneIsSet proves a task made on the defaults
// carries no budget from its first checkpoint, and loads back that way.
func TestARecordStartsWithNoBudgetWhenNoneIsSet(t *testing.T) {
	start := taskStart()
	start.RoundsLeft, start.MinutesLeft = 5, 5
	start.NoRoundBudget, start.NoTimeBudget = true, true
	keeper, store := newKeeper(t, start)

	if first := firstLine(string(Print(keeper.Record()))); !strings.HasSuffix(first, "no budget") {
		t.Errorf("a record started with no budget reads %q, and a count on a limit that is off is not kept", first)
	}
	loaded, err := Load(t.Context(), store, contract.RecordTask, keeper.ID())
	if err != nil {
		t.Fatalf("cannot load the record back: %v", err)
	}
	if held := loaded.Record().Header; !held.NoRoundBudget || !held.NoTimeBudget || held.RoundsLeft != 0 {
		t.Errorf("the record loaded back as %+v, and it was started with no budget", held)
	}
}
