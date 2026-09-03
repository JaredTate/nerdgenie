package browser_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/skill"
	"github.com/JaredTate/coeus/internal/skill/browser"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theRenumberedPage is the fixture page after somebody rebuilt the site: the
// same link, under a reference the recording has never heard of.
const theRenumberedPage = "https://fixture.test/renumbered"

// addTheRenumberedPage puts that page on the fixture browser and says where its
// link goes.
func addTheRenumberedPage(built *bench, link contract.Element) {
	built.worker.AddPage(contract.Snapshot{
		URL: theRenumberedPage, Title: "The renumbered page", TabID: "t1",
		Elements: []contract.Element{link, {Ref: "e8", Role: "textbox", Name: "Note"}},
	})
	built.worker.LinkGoesTo(link.Ref, testkit.FixtureChangedPage)
}

// replayerWithAModelNobodyMayCall builds a replayer whose model would fail the
// test if it were ever asked anything.
func replayerWithAModelNobodyMayCall(t *testing.T, built *bench) (*browser.Replayer, *testkit.FakeModel) {
	t.Helper()
	model := testkit.NewFakeModel(testkit.Script{Name: "nobody-calls-me", ContextLength: 8000})
	replayer, err := browser.New(browser.Options{Browser: built.worker, Model: model, Ask: built.channel.ShowPreview, Skills: built.store})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}
	return replayer, model
}

func TestAThreeStepRecordingReplaysWithTheModelNeverCalled(t *testing.T) {
	built := newBench(t)
	folder := built.save(t, "fixture-walk", theThreeStepFlow())
	replayer, model := replayerWithAModelNobodyMayCall(t, built)

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("cannot replay the recording: %v", err)
	}
	if !report.Met() {
		t.Fatalf("the replay did not do what was recorded:\n%s", report)
	}
	if len(report.Outcomes) != 3 {
		t.Fatalf("the replay reports %d steps, want three:\n%s", len(report.Outcomes), report)
	}
	if calls := len(model.Requests()); calls != 0 {
		t.Errorf("the model was called %d times, and a replay calls it none", calls)
	}
	if written := built.worker.TypedInto("e3"); len(written) != 1 || written[0] != "nine years" {
		t.Errorf("the replay typed %v, want the words the recording typed", written)
	}
}

func TestAStaleReferenceIsFoundAgainByTheRoleAndTheName(t *testing.T) {
	built := newBench(t)
	addTheRenumberedPage(built, contract.Element{Ref: "e7", Role: "link", Name: "Change the page"})
	folder := built.save(t, "fixture-walk", []browser.Step{
		{Number: 1, Intent: "Open the page.", Tool: contract.ToolBrowserOpen, Address: theRenumberedPage, Expectation: "the renumbered page"},
		{
			Number: 2, Intent: "Follow the link.", Tool: contract.ToolBrowserClick,
			Element:     browser.Descriptor{Ref: "e1", Role: "link", Name: "Change the page", Shown: "Change the page"},
			Expectation: "the page changed",
		},
	})
	replayer, model := replayerWithAModelNobodyMayCall(t, built)

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("cannot replay the recording: %v", err)
	}
	if !report.Met() {
		t.Fatalf("the replay did not find the link again:\n%s", report)
	}
	if found := report.Outcomes[1].FoundBy; found != "its role and name" {
		t.Errorf("the link was found by %q, want its role and name", found)
	}
	if calls := len(model.Requests()); calls != 0 {
		t.Errorf("the model was called %d times, and finding an element again needs no model", calls)
	}
}

func TestAnElementWhoseRoleChangedIsFoundAgainByTheTextItShowed(t *testing.T) {
	built := newBench(t)
	addTheRenumberedPage(built, contract.Element{Ref: "e7", Role: "menuitem", Name: "Change the page now"})
	folder := built.save(t, "fixture-walk", []browser.Step{
		{Number: 1, Intent: "Open the page.", Tool: contract.ToolBrowserOpen, Address: theRenumberedPage, Expectation: "the renumbered page"},
		{
			Number: 2, Intent: "Follow the link.", Tool: contract.ToolBrowserClick,
			Element:     browser.Descriptor{Ref: "e1", Role: "link", Name: "Change the page", Shown: "Change the page"},
			Expectation: "the page changed",
		},
	})
	replayer, _ := replayerWithAModelNobodyMayCall(t, built)

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("cannot replay the recording: %v", err)
	}
	if !report.Met() {
		t.Fatalf("the replay did not find the link again:\n%s", report)
	}
	if found := report.Outcomes[1].FoundBy; found != "the text it showed" {
		t.Errorf("the link was found by %q, want the text it showed", found)
	}
}

func TestAReplayStopsAtTheFirstUnmetExpectationAndSaysWhatWasSeen(t *testing.T) {
	built := newBench(t)
	steps := theThreeStepFlow()
	steps[1].Expectation = "the invoice was paid in full"
	folder := built.save(t, "fixture-walk", steps)
	replayer, err := browser.New(browser.Options{Browser: built.worker})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("cannot replay the recording: %v", err)
	}
	if report.Met() {
		t.Fatalf("the replay says every step worked, and the second one cannot have:\n%s", report)
	}
	if len(report.Outcomes) != 2 {
		t.Fatalf("the replay ran %d steps, and it should stop at the second:\n%s", len(report.Outcomes), report)
	}
	if seen := report.Outcomes[1].Seen; !strings.Contains(seen, "invoice") {
		t.Errorf("the report says %q was seen, and it should say what was expected and what happened", seen)
	}
	if !strings.Contains(report.String(), "not met") {
		t.Errorf("the report does not say the step was not met:\n%s", report)
	}
}

func TestAnElementNoRungOfTheCascadeFindsStopsTheReplay(t *testing.T) {
	built := newBench(t)
	addTheRenumberedPage(built, contract.Element{Ref: "e7", Role: "link", Name: "Something else entirely"})
	folder := built.save(t, "fixture-walk", []browser.Step{
		{Number: 1, Intent: "Open the page.", Tool: contract.ToolBrowserOpen, Address: theRenumberedPage, Expectation: "the renumbered page"},
		{
			Number: 2, Intent: "Follow the link.", Tool: contract.ToolBrowserClick,
			Element:     browser.Descriptor{Ref: "e1", Role: "link", Name: "Change the page", Shown: "Change the page"},
			Expectation: "the page changed",
		},
	})
	replayer, err := browser.New(browser.Options{Browser: built.worker})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("cannot replay the recording: %v", err)
	}
	if report.Met() {
		t.Fatalf("the replay says it found an element that is not on the page:\n%s", report)
	}
	if seen := report.Outcomes[1].Seen; !strings.Contains(seen, "no element") {
		t.Errorf("the report says %q was seen, and it should say the element was not found", seen)
	}
}

func TestAStepThatCannotBeUndoneIsShownToTheUserBeforeItRuns(t *testing.T) {
	built := newBench(t)
	folder := built.save(t, "fixture-walk", theThreeStepFlow(), 3)
	replayer, err := browser.New(browser.Options{Browser: built.worker, Ask: built.channel.ShowPreview})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("cannot replay the recording: %v", err)
	}
	if !report.Met() {
		t.Fatalf("the user said yes and the replay still did not finish:\n%s", report)
	}
	previews := built.channel.Previews()
	if len(previews) != 1 {
		t.Fatalf("the user was shown %d previews, want the one step that cannot be undone", len(previews))
	}
	if !strings.Contains(previews[0].Title, "cannot be undone") {
		t.Errorf("the preview says %q, and it should say why it is being asked", previews[0].Title)
	}
}

// aModelThatWouldNameTheLink answers the one question a self-heal asks with the
// reference of the link on the fixture page, so that a replay which heals a
// refused step really does follow it.
func aModelThatWouldNameTheLink() *testkit.FakeModel {
	return testkit.NewFakeModel(testkit.Script{Name: "healer", ContextLength: 8000, Steps: []testkit.Step{{
		Text: "e1", Finish: contract.FinishEnd,
	}}})
}

func TestAStepThatCannotBeUndoneIsNotRunWhenTheUserSaysNo(t *testing.T) {
	built := newBench(t)
	built.channel.AnswerPreviewsWith(contract.AnswerReject)
	folder := built.save(t, "fixture-walk", theThreeStepFlow(), 3)
	model := aModelThatWouldNameTheLink()
	replayer, err := browser.New(browser.Options{Browser: built.worker, Model: model, Ask: built.channel.ShowPreview, Skills: built.store})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("a refusal is an answer, not a failure of the replay: %v", err)
	}
	if report.Met() {
		t.Fatalf("the replay ran a step the user refused:\n%s", report)
	}
	if report.Outcomes[2].Met || report.Outcomes[2].Healed {
		t.Errorf("the refused step is reported as %+v, and a step the user refused was never taken", report.Outcomes[2])
	}
	if calls := len(model.Requests()); calls != 0 {
		t.Errorf("the model was asked %d times about a step the user refused, and a refused step is never healed", calls)
	}
	page, err := built.worker.Read(context.Background(), contract.ReadOptions{})
	if err != nil {
		t.Fatalf("cannot read the page the browser is on: %v", err)
	}
	if page.URL == testkit.FixtureChangedPage {
		t.Error("the browser followed the link the user refused")
	}
}

func TestAStepThatCannotBeUndoneStopsAReplayWithNoScreenToAskOn(t *testing.T) {
	built := newBench(t)
	folder := built.save(t, "fixture-walk", theThreeStepFlow(), 3)
	model := aModelThatWouldNameTheLink()
	replayer, err := browser.New(browser.Options{Browser: built.worker, Model: model, Skills: built.store})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("an unattended replay stops, it does not fail: %v", err)
	}
	if report.Met() {
		t.Fatalf("an unattended replay took a step that cannot be undone:\n%s", report)
	}
	if seen := report.Outcomes[2].Seen; !strings.Contains(seen, "no screen") {
		t.Errorf("the report says %q, and it should say there was nobody to ask", seen)
	}
	if calls := len(model.Requests()); calls != 0 {
		t.Errorf("the model was asked %d times about a step nobody could be asked about, and such a step is never healed", calls)
	}
	page, err := built.worker.Read(context.Background(), contract.ReadOptions{})
	if err != nil {
		t.Fatalf("cannot read the page the browser is on: %v", err)
	}
	if page.URL == testkit.FixtureChangedPage {
		t.Error("an unattended replay followed the link it stopped before")
	}
}

func TestAReplayerWithNoBrowserBehindItIsRefused(t *testing.T) {
	if _, err := browser.New(browser.Options{}); err == nil {
		t.Fatal("a replayer was built with no browser behind it")
	}
}

func TestASkillThatIsNotABrowserRecordingIsRefusedByTheReplayer(t *testing.T) {
	built := newBench(t)
	replayer, err := browser.New(browser.Options{Browser: built.worker})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}
	folder := skill.Folder{Definition: skill.Definition{Name: "shell-thing"}, HasScript: true}
	if _, err := replayer.Replay(context.Background(), folder); err == nil {
		t.Fatal("a skill carrying a script was replayed as a browser recording")
	}
}
