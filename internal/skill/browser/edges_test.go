package browser_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/skill"
	"github.com/JaredTate/nerdgenie/internal/skill/browser"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestTheCascadeFindsNothingWhenTheDescriptorPointsAtNothingThere(t *testing.T) {
	page := contract.Snapshot{URL: "https://fixture.test/simple", Title: "A simple page", Elements: []contract.Element{
		{Ref: "e1", Role: "link", Name: "Change the page"},
	}}
	cases := []struct {
		name       string
		descriptor browser.Descriptor
		foundBy    string
	}{
		{"by its reference", browser.Descriptor{Ref: "e1"}, "its reference"},
		{"by its role and name", browser.Descriptor{Ref: "e4", Role: "LINK", Name: "change the page"}, "its role and name"},
		{"by the text it showed", browser.Descriptor{Ref: "e4", Role: "menuitem", Shown: "change the"}, "the text it showed"},
		{"by the name when no text was recorded", browser.Descriptor{Ref: "e4", Name: "Change the page"}, "the text it showed"},
		{"by nothing at all", browser.Descriptor{Ref: "e4"}, ""},
		{"by nothing on an empty descriptor", browser.Descriptor{}, ""},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			ref, foundBy, found := browser.FindElement(page, one.descriptor)
			if found != (one.foundBy != "") {
				t.Fatalf("the cascade says found is %v for %+v", found, one.descriptor)
			}
			if foundBy != one.foundBy {
				t.Errorf("the cascade found it by %q, want %q", foundBy, one.foundBy)
			}
			if found && ref != "e1" {
				t.Errorf("the cascade returned %q, want the reference the page uses now", ref)
			}
		})
	}
}

// thePageOfNearMisses holds the two elements the reviewer's probes resolved to
// wrongly: a link whose name happens to hold two letters of a button's name, and
// a button whose name begins with the word a link was called.
func thePageOfNearMisses() contract.Snapshot {
	return contract.Snapshot{URL: "https://fixture.test/settings", Title: "Settings", Elements: []contract.Element{
		{Ref: "e1", Role: "link", Name: "Cookie settings"},
		{Ref: "e2", Role: "button", Name: "Delete my account"},
	}}
}

func TestTheTextRungLooksForNothingShorterThanFourCharacters(t *testing.T) {
	ref, foundBy, found := browser.FindElement(thePageOfNearMisses(), browser.Descriptor{Role: "button", Name: "OK", Shown: "OK"})
	if found {
		t.Errorf("a step recorded on the button OK resolved to %s by %s, and two letters sit inside almost any name", ref, foundBy)
	}
}

func TestTheTextRungRefusesANameFarLongerThanTheTextWhenTheRolesDisagree(t *testing.T) {
	ref, foundBy, found := browser.FindElement(thePageOfNearMisses(), browser.Descriptor{Role: "link", Name: "Delete", Shown: "Delete"})
	if found {
		t.Errorf("a step recorded on the link Delete resolved to %s by %s, which is the button that closes the account", ref, foundBy)
	}
}

func TestTheTextRungTakesANameFarLongerThanTheTextWhenTheRolesAgree(t *testing.T) {
	ref, _, found := browser.FindElement(thePageOfNearMisses(), browser.Descriptor{Role: "button", Name: "Delete", Shown: "Delete"})
	if !found || ref != "e2" {
		t.Errorf("a step recorded on a button called Delete found %q, want the button whose name begins with the word", ref)
	}
}

func TestADescriptorWithOnlyARoleSaysSo(t *testing.T) {
	if said := (browser.Descriptor{Role: "button"}).String(); said != "the button" {
		t.Errorf("the descriptor says %q, want the role on its own", said)
	}
	if said := (browser.Descriptor{Ref: "e1"}).String(); said != "e1" {
		t.Errorf("the descriptor says %q, want the reference on its own", said)
	}
	if said := (browser.Descriptor{Name: "Sign in"}).String(); said != `the element named "Sign in"` {
		t.Errorf("the descriptor says %q, want the name on its own", said)
	}
}

func TestARecorderTellsTheCallerWhenTheBrowserIsGone(t *testing.T) {
	built := newBench(t)
	recorder, err := browser.NewRecorder(built.worker)
	if err != nil {
		t.Fatalf("cannot build the recorder: %v", err)
	}
	ctx := context.Background()
	if _, err := recorder.Open(ctx, "Open the fixture page.", testkit.FixtureSimplePage, "a simple page"); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	if err := built.worker.Close(); err != nil {
		t.Fatalf("cannot close the fixture browser: %v", err)
	}

	if _, err := recorder.Open(ctx, "Open it again.", testkit.FixtureSimplePage, "a simple page"); err == nil {
		t.Error("the recorder opened a page in a browser that is gone")
	}
	if _, err := recorder.Type(ctx, "Write the note.", "e3", "nine years", "the note box holds it"); err == nil {
		t.Error("the recorder typed into a browser that is gone")
	}
	if _, err := recorder.Click(ctx, "Follow the link.", "e1", "the page changed"); err == nil {
		t.Error("the recorder clicked in a browser that is gone")
	}
}

func TestARecordingWithNoOpeningStepDryRunsWithNoAddress(t *testing.T) {
	built := newBench(t)
	recorder, err := browser.NewRecorder(built.worker)
	if err != nil {
		t.Fatalf("cannot build the recorder: %v", err)
	}
	ctx := context.Background()
	if _, err := built.worker.Open(ctx, testkit.FixtureSimplePage); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	if _, err := recorder.Type(ctx, "Write the note.", "e3", "nine years", "the note box holds the words"); err != nil {
		t.Fatalf("cannot write the note: %v", err)
	}
	if err := recorder.Save(ctx, built.store, definitionOf("note-only")); err != nil {
		t.Fatalf("cannot save the recording: %v", err)
	}
	if arguments := built.load(t, "note-only").Plan.Arguments; arguments != "" {
		t.Errorf("the dry run replays with %q, and the recording opened nothing", arguments)
	}
}

func TestARecordingWithNoStoreToSaveIntoIsRefused(t *testing.T) {
	built := newBench(t)
	recorder := recordTheThreeStepFlow(t, built)
	if err := recorder.Save(context.Background(), nil, definitionOf("fixture-walk")); err == nil {
		t.Fatal("a recording was saved with no store to save into")
	}
}

func TestAReplayReportsAnAddressThatWillNotOpen(t *testing.T) {
	built := newBench(t)
	folder := built.save(t, "fixture-walk", []browser.Step{{
		Number: 1, Intent: "Open a page nobody built.", Tool: contract.ToolBrowserOpen,
		Address: "https://fixture.test/nowhere", Expectation: "a page appears",
	}})
	replayer, err := browser.New(browser.Options{Browser: built.worker})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("an address that will not open is a report, not a failure: %v", err)
	}
	if report.Met() {
		t.Fatalf("the replay says it opened a page that is not there:\n%s", report)
	}
	if seen := report.Outcomes[0].Seen; !strings.Contains(seen, "could not be opened") {
		t.Errorf("the report says %q, and it should say the page could not be opened", seen)
	}
}

func TestAReplayReportsAPageThatCannotBeReadAfterTheAction(t *testing.T) {
	built := newBench(t)
	folder := built.save(t, "fixture-walk", theThreeStepFlow())
	built.worker.NextActionCannotBeRead()
	replayer, err := browser.New(browser.Options{Browser: built.worker})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("a page that will not settle is a report, not a failure: %v", err)
	}
	if report.Met() {
		t.Fatalf("the replay says a step worked on a page it could not read:\n%s", report)
	}
	if seen := report.Outcomes[1].Seen; !strings.Contains(seen, "could not act on") {
		t.Errorf("the report says %q, and it should say the browser could not act", seen)
	}
}

func TestAReplayOfABrowserThatIsGoneIsAFailureAndNotAReport(t *testing.T) {
	built := newBench(t)
	folder := built.save(t, "fixture-walk", []browser.Step{{
		Number: 1, Intent: "Follow the link.", Tool: contract.ToolBrowserClick,
		Element: browser.Descriptor{Ref: "e1", Role: "link", Name: "Change the page"}, Expectation: "the page changed",
	}})
	if err := built.worker.Close(); err != nil {
		t.Fatalf("cannot close the fixture browser: %v", err)
	}
	replayer, err := browser.New(browser.Options{Browser: built.worker})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	if _, err := replayer.Replay(context.Background(), folder); err == nil {
		t.Fatal("a replay against a browser that is gone reported success")
	}
}

func TestTheReportOfAnApprovedPatchSaysTheUserApprovedIt(t *testing.T) {
	built, folder, model := benchWithARebuiltPage(t, "e9")
	replayer, err := browser.New(browser.Options{Browser: built.worker, Model: model, Ask: built.channel.ShowPreview, Skills: built.store})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("cannot replay the broken recording: %v", err)
	}
	said := report.String()
	if !strings.Contains(said, "you approved this change") {
		t.Errorf("the report does not say the user approved the change:\n%s", said)
	}
	if !strings.Contains(said, "after the model was asked") {
		t.Errorf("the report does not say the step was healed:\n%s", said)
	}
}

func TestAPatchIsNotWrittenForASkillThatIsNotOnDisk(t *testing.T) {
	built, folder, model := benchWithARebuiltPage(t, "e9")
	folder.Path = ""
	replayer, err := browser.New(browser.Options{Browser: built.worker, Model: model, Ask: built.channel.ShowPreview, Skills: built.store})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("cannot replay the broken recording: %v", err)
	}
	if report.Applied {
		t.Fatalf("a patch was written for a skill that is nowhere on disk:\n%s", report)
	}
}

func TestAModelThatCannotBeReachedHealsNothing(t *testing.T) {
	built, folder, _ := benchWithARebuiltPage(t, "e9")
	empty := testkit.NewFakeModel(testkit.Script{Name: "no-steps", ContextLength: 8000})
	replayer, err := browser.New(browser.Options{Browser: built.worker, Model: empty, Ask: built.channel.ShowPreview, Skills: built.store})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("a model that will not answer is a report, not a failure: %v", err)
	}
	if report.Met() || report.Patch != "" {
		t.Fatalf("a model that could not be reached still healed something:\n%s", report)
	}
}

func TestAVisualCheckSaysWhenAPageCannotBePhotographed(t *testing.T) {
	built := newBench(t)
	into := t.TempDir()
	if err := os.MkdirAll(filepath.Join(into, "01-open-the-app.png"), contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot put something in the way of the picture: %v", err)
	}
	checker, err := browser.NewChecker(built.worker)
	if err != nil {
		t.Fatalf("cannot build the checker: %v", err)
	}

	result, err := checker.Check(context.Background(), browser.Request{
		Steps: aWalkWithOneStateThatHoldsAndOneThatDoesNot()[:1], Into: into,
	})
	if err != nil {
		t.Fatalf("a picture that cannot be written is a report, not a failure: %v", err)
	}
	if result.Findings[0].Picture != "" {
		t.Error("the check says it wrote a picture it could not write")
	}
	if !strings.Contains(result.Findings[0].Seen, "No picture could be taken") {
		t.Errorf("the report says %q, and it should say no picture could be taken", result.Findings[0].Seen)
	}
}

func TestAVisualCheckSaysWhenTheAppWillNotOpen(t *testing.T) {
	built := newBench(t)
	checker, err := browser.NewChecker(built.worker)
	if err != nil {
		t.Fatalf("cannot build the checker: %v", err)
	}

	result, err := checker.Check(context.Background(), browser.Request{Into: t.TempDir(), Steps: []browser.Step{{
		Number: 1, Intent: "Open the app.", Tool: contract.ToolBrowserOpen,
		Address: "https://fixture.test/nowhere", Expectation: "the app is on the screen",
	}}})
	if err != nil {
		t.Fatalf("an app that will not open is a report, not a failure: %v", err)
	}
	if !strings.Contains(result.StoppedBecause, "could not open") {
		t.Errorf("the check stopped because %q, and it should say the app would not open", result.StoppedBecause)
	}
	if strings.Contains(result.Markdown(), "# visual check of \n") {
		t.Errorf("the heading of a check with no name is empty:\n%s", result.Markdown())
	}
}

func TestAVisualCheckSaysWhenThereIsNoPageToActOn(t *testing.T) {
	built := newBench(t)
	checker, err := browser.NewChecker(built.worker)
	if err != nil {
		t.Fatalf("cannot build the checker: %v", err)
	}

	result, err := checker.Check(context.Background(), browser.Request{Into: t.TempDir(), Steps: []browser.Step{{
		Number: 1, Intent: "Click before anything is open.", Tool: contract.ToolBrowserClick,
		Element: browser.Descriptor{Ref: "e1", Role: "link", Name: "Change the page"}, Expectation: "the page changed",
	}}})
	if err != nil {
		t.Fatalf("a browser with nothing open is a report, not a failure: %v", err)
	}
	if !strings.Contains(result.StoppedBecause, "could not read the page") {
		t.Errorf("the check stopped because %q, and it should say the page could not be read", result.StoppedBecause)
	}
}

func TestAVisualCheckSaysWhenItCouldNotActOnTheElement(t *testing.T) {
	built := newBench(t)
	built.worker.NextActionCannotBeRead()
	checker, err := browser.NewChecker(built.worker)
	if err != nil {
		t.Fatalf("cannot build the checker: %v", err)
	}

	result, err := checker.Check(context.Background(), browser.Request{
		Steps: aWalkWithOneStateThatHoldsAndOneThatDoesNot(), Into: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("a page that will not settle is a report, not a failure: %v", err)
	}
	if !strings.Contains(result.StoppedBecause, "could not act on") {
		t.Errorf("the check stopped because %q, and it should say it could not act", result.StoppedBecause)
	}
}

func TestAVisualCheckOfASkillThatIsNotAWalkIsRefused(t *testing.T) {
	built := newBench(t)
	checker, err := browser.NewChecker(built.worker)
	if err != nil {
		t.Fatalf("cannot build the checker: %v", err)
	}
	folder := skill.Folder{Definition: skill.Definition{Name: "shell-thing"}, HasScript: true}
	if _, err := checker.CheckFolder(context.Background(), folder, "", t.TempDir()); err == nil {
		t.Fatal("a skill carrying a script was walked as a browser check")
	}
}

func TestAStepWhoseWordsMakeNoFileNameIsStillPhotographed(t *testing.T) {
	built := newBench(t)
	into := t.TempDir()
	checker, err := browser.NewChecker(built.worker)
	if err != nil {
		t.Fatalf("cannot build the checker: %v", err)
	}

	result, err := checker.Check(context.Background(), browser.Request{Into: into, Steps: []browser.Step{
		{Number: 1, Intent: "...", Tool: contract.ToolBrowserOpen, Address: testkit.FixtureSimplePage, Expectation: "a simple page"},
		{
			Number: 2, Intent: strings.Repeat("a very long way of saying it ", 6), Tool: contract.ToolBrowserClick,
			Element: browser.Descriptor{Ref: "e1", Role: "link", Name: "Change the page"}, Expectation: "the page changed",
		},
	}})
	if err != nil {
		t.Fatalf("cannot check the fixture app: %v", err)
	}
	if name := filepath.Base(result.Findings[0].Picture); name != "01-step.png" {
		t.Errorf("the picture is called %q, and a step with no words in it is still called something", name)
	}
	if name := filepath.Base(result.Findings[1].Picture); len(name) > 50 {
		t.Errorf("the picture is called %q, and a very long step name is cut down", name)
	}
}

func TestTheQualitySkillNeedsAStoreToInstallInto(t *testing.T) {
	if err := browser.InstallQASkill(context.Background(), nil); err == nil {
		t.Fatal("the quality skill was installed into nothing")
	}
}
