package browser_test

import (
	"context"
	"github.com/JaredTate/coeus/internal/contract"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/skill"
	"github.com/JaredTate/coeus/internal/skill/browser"
	"github.com/JaredTate/coeus/internal/testkit"
)

// recordTheThreeStepFlow drives the fixture browser through the flow every test
// in this file starts from, writing down what it did as it goes.
func recordTheThreeStepFlow(t *testing.T, built *bench) *browser.Recorder {
	t.Helper()
	recorder, err := browser.NewRecorder(built.worker)
	if err != nil {
		t.Fatalf("cannot build the recorder: %v", err)
	}
	ctx := context.Background()
	if _, err := recorder.Open(ctx, "Open the fixture page.", testkit.FixtureSimplePage, "a simple page with a form on it"); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	if _, err := recorder.Type(ctx, "Write the note.", "e3", "nine years", "the note box holds the words"); err != nil {
		t.Fatalf("cannot write the note: %v", err)
	}
	if _, err := recorder.Click(ctx, "Follow the link that changes the page.", "e1", "the page changed"); err != nil {
		t.Fatalf("cannot follow the link: %v", err)
	}
	return recorder
}

func TestARecordingKeepsTheIntentTheDescriptorAndTheExpectationOfEveryStep(t *testing.T) {
	built := newBench(t)
	steps := recordTheThreeStepFlow(t, built).Steps()

	if len(steps) != 3 {
		t.Fatalf("the recording holds %d steps, want the three that were driven: %+v", len(steps), steps)
	}
	wanted := theThreeStepFlow()
	for at, step := range steps {
		if step != wanted[at] {
			t.Errorf("step %d was written down as %+v, want %+v", at+1, step, wanted[at])
		}
	}
}

func TestARecordingBecomesASkillFolderThatLoadsAndReadsBack(t *testing.T) {
	built := newBench(t)
	recorder := recordTheThreeStepFlow(t, built)

	if err := recorder.Save(context.Background(), contract.SkillSavedByPerson, built.store, definitionOf("fixture-walk")); err != nil {
		t.Fatalf("cannot save the recording as a skill: %v", err)
	}

	folder := built.load(t, "fixture-walk")
	steps, err := browser.StepsOf(folder)
	if err != nil {
		t.Fatalf("the saved folder does not read back as a recording: %v", err)
	}
	wanted := theThreeStepFlow()
	for at, step := range steps {
		if step != wanted[at] {
			t.Errorf("step %d read back as %+v, want %+v", at+1, step, wanted[at])
		}
	}

	changelog, err := built.store.Changelog("fixture-walk")
	if err != nil {
		t.Fatalf("the saved recording has no changelog: %v", err)
	}
	if !strings.Contains(changelog, "saved for the first time") {
		t.Errorf("the changelog does not say the skill was saved:\n%s", changelog)
	}
}

func TestTheDryRunOfASavedRecordingOpensThePageTheRecordingOpened(t *testing.T) {
	built := newBench(t)
	recorder := recordTheThreeStepFlow(t, built)
	if err := recorder.Save(context.Background(), contract.SkillSavedByPerson, built.store, definitionOf("fixture-walk")); err != nil {
		t.Fatalf("cannot save the recording as a skill: %v", err)
	}

	if arguments := built.load(t, "fixture-walk").Plan.Arguments; arguments != testkit.FixtureSimplePage {
		t.Errorf("the dry run replays with %q, want the address the recording opened", arguments)
	}
}

func TestAStepThatDidNotDoWhatItWasSaidToDoIsNotWrittenDown(t *testing.T) {
	built := newBench(t)
	recorder, err := browser.NewRecorder(built.worker)
	if err != nil {
		t.Fatalf("cannot build the recorder: %v", err)
	}
	ctx := context.Background()
	if _, err := recorder.Open(ctx, "Open the fixture page.", testkit.FixtureSimplePage, "a simple page"); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}

	built.worker.NextActionChangesNothing()
	change, err := recorder.Click(ctx, "Follow the link.", "e1", "the invoice was paid")
	if err != nil {
		t.Fatalf("the click itself failed, and only its expectation should have: %v", err)
	}
	if change.ExpectationMet {
		t.Fatal("the fixture browser says the expectation was met, so this test proves nothing")
	}
	if steps := recorder.Steps(); len(steps) != 1 {
		t.Fatalf("the recording holds %d steps, and only the one that worked belongs in it: %+v", len(steps), steps)
	}
}

func TestAnOpeningStepThatLandedSomewhereElseIsNotWrittenDown(t *testing.T) {
	built := newBench(t)
	recorder, err := browser.NewRecorder(built.worker)
	if err != nil {
		t.Fatalf("cannot build the recorder: %v", err)
	}
	if _, err := recorder.Open(context.Background(), "Open the shop.", testkit.FixtureSimplePage, "the shopping basket is on the screen"); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	if steps := recorder.Steps(); len(steps) != 0 {
		t.Fatalf("the recording holds %d steps, and the page was not the one expected: %+v", len(steps), steps)
	}
}

// The fifty in this test is written out rather than read from the package,
// because a loop that counted to the constant would take whatever the constant
// became and still pass.
func TestARecordingRefusesMoreStepsThanASkillFolderHolds(t *testing.T) {
	built := newBench(t)
	recorder, err := browser.NewRecorder(built.worker)
	if err != nil {
		t.Fatalf("cannot build the recorder: %v", err)
	}
	ctx := context.Background()
	for taken := 0; taken < 50; taken++ {
		if _, err := recorder.Open(ctx, "Open the fixture page.", testkit.FixtureSimplePage, "a simple page"); err != nil {
			t.Fatalf("cannot open the fixture page on step %d: %v", taken+1, err)
		}
	}
	if _, err := recorder.Open(ctx, "Open it once more.", testkit.FixtureSimplePage, "a simple page"); err == nil {
		t.Fatal("the recorder took a step past the cap, and every list has a cap")
	}
}

func TestARecordingWithNoStepsInItIsNotSaved(t *testing.T) {
	built := newBench(t)
	recorder, err := browser.NewRecorder(built.worker)
	if err != nil {
		t.Fatalf("cannot build the recorder: %v", err)
	}
	if err := recorder.Save(context.Background(), contract.SkillSavedByPerson, built.store, definitionOf("empty-walk")); err == nil {
		t.Fatal("an empty recording was saved, and a skill with no steps replays nothing")
	}
}

func TestARecorderWithNoBrowserBehindItIsRefused(t *testing.T) {
	if _, err := browser.NewRecorder(nil); err == nil {
		t.Fatal("a recorder was built with no browser behind it")
	}
}

func TestARecordedElementCarriesItsRoleAndNameFromThePage(t *testing.T) {
	built := newBench(t)
	recorder := recordTheThreeStepFlow(t, built)
	typed := recorder.Steps()[1]
	if typed.Element.Role != "textbox" || typed.Element.Name != "Note" {
		t.Errorf("the typed-into element is %+v, want the role and name the page gave it", typed.Element)
	}
	if written := built.worker.TypedInto("e3"); len(written) != 1 || written[0] != "nine years" {
		t.Errorf("the fixture browser was given %v, want the words the recorder typed", written)
	}
}

func TestARecordedStepAgainstAnElementThePageDoesNotHaveIsReported(t *testing.T) {
	built := newBench(t)
	recorder, err := browser.NewRecorder(built.worker)
	if err != nil {
		t.Fatalf("cannot build the recorder: %v", err)
	}
	ctx := context.Background()
	if _, err := recorder.Open(ctx, "Open the fixture page.", testkit.FixtureSimplePage, "a simple page"); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	if _, err := recorder.Click(ctx, "Click something that is not there.", "e99", "something happens"); err == nil {
		t.Fatal("a click on an element the page does not have was recorded as though it worked")
	}
}

func TestARecordingSaysWhatItIsMadeOfWhenTheSkillNameIsWrong(t *testing.T) {
	built := newBench(t)
	recorder := recordTheThreeStepFlow(t, built)
	if err := recorder.Save(context.Background(), contract.SkillSavedByPerson, built.store, skill.Definition{Name: "Not A Name"}); err == nil {
		t.Fatal("a recording was saved under a name no folder may have")
	}
}
