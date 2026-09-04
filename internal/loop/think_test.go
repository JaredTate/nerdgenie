package loop_test

import (
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theLevelIsCarriedOn returns the think level the last call the model was sent
// carried, failing when no call was made at all.
func theLevelIsCarriedOn(t *testing.T, model *testkit.FakeModel) contract.Think {
	t.Helper()
	sent := model.Requests()
	if len(sent) == 0 {
		t.Fatal("the model was never called, so there is no request to read the think level off")
	}
	return sent[len(sent)-1].Think
}

// TestACallCarriesNoThinkLevelUntilOneIsChosen holds the behaviour Nerd Genie had
// before "/think" existed: nothing is said about thinking, and the level on the
// model alias in config.toml is the only thing the provider reads.
func TestACallCarriesNoThinkLevelUntilOneIsChosen(t *testing.T) {
	built := newHarness(t, []testkit.Step{answerStep("Two plus two is four.")})

	built.ask(t, "what is two plus two")

	if level := theLevelIsCarriedOn(t, built.model); level != contract.ThinkDefault {
		t.Errorf("the call was made at %q, and nobody has chosen a level, so it must carry none", level)
	}
}

// TestTheLevelChosenForTheModelInUseIsOnEveryCall holds what "/think medium"
// has to do: the level reaches the provider on the next call, without the model
// being built again.
func TestTheLevelChosenForTheModelInUseIsOnEveryCall(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		answerStep("Two plus two is four."),
		answerStep("Three plus three is six."),
	})

	built.loop.UseThink(built.model.Name(), contract.ThinkMedium)
	built.ask(t, "what is two plus two")
	if level := theLevelIsCarriedOn(t, built.model); level != contract.ThinkMedium {
		t.Errorf("the first call was made at %q, want %q", level, contract.ThinkMedium)
	}

	built.ask(t, "what is three plus three")
	if level := theLevelIsCarriedOn(t, built.model); level != contract.ThinkMedium {
		t.Errorf("the second call was made at %q, and the level lasts the whole session", level)
	}
}

// TestTheLevelChosenForOneModelIsNotPutOnAnother holds that the level belongs
// to the model it was chosen for: after "/model" switches to another one, that
// model's own setting from config.toml stands rather than the level somebody
// chose for the model before it.
func TestTheLevelChosenForOneModelIsNotPutOnAnother(t *testing.T) {
	built := newHarness(t, []testkit.Step{answerStep("Two plus two is four.")})

	built.loop.UseThink("some other model", contract.ThinkMax)
	built.ask(t, "what is two plus two")

	if level := theLevelIsCarriedOn(t, built.model); level != contract.ThinkDefault {
		t.Errorf("the call was made at %q, and the level was chosen for another model", level)
	}
}
