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

// theRebuiltPage is the fixture page after somebody renamed the link on it, so
// that no rung of the cascade can find it and only the model can. The link is
// still a link, because the self-heal never moves a step onto an element of
// another kind.
const theRebuiltPage = "https://fixture.test/rebuilt"

// brokenRecording is a walk whose second step points at an element the rebuilt
// page no longer has under any of the three names it was recorded with.
func brokenRecording() []browser.Step {
	return []browser.Step{
		{Number: 1, Intent: "Open the page.", Tool: contract.ToolBrowserOpen, Address: theRebuiltPage, Expectation: "the rebuilt page"},
		{
			Number: 2, Intent: "Follow the link that changes the page.", Tool: contract.ToolBrowserClick,
			Element:     browser.Descriptor{Ref: "e1", Role: "link", Name: "Change the page", Shown: "Change the page"},
			Expectation: "the page changed",
		},
	}
}

// benchWithARebuiltPage sets up the store, the rebuilt page, and a model that
// answers the one question a self-heal asks.
func benchWithARebuiltPage(t *testing.T, answer string) (*bench, skill.Folder, *testkit.FakeModel) {
	t.Helper()
	built := newBench(t)
	built.worker.AddPage(contract.Snapshot{
		URL: theRebuiltPage, Title: "Onwards", TabID: "t1",
		Elements: []contract.Element{{Ref: "e9", Role: "link", Name: "Onwards"}},
	})
	built.worker.LinkGoesTo("e9", testkit.FixtureChangedPage)

	model := testkit.NewFakeModel(testkit.Script{Name: "healer", ContextLength: 8000, Steps: []testkit.Step{{
		Expect: []string{"Follow the link that changes the page.", "the page changed", `e9 link "Onwards"`},
		Text:   answer,
		Finish: contract.FinishEnd,
	}}})
	return built, built.save(t, "fixture-walk", brokenRecording()), model
}

func TestABrokenStepHasAPatchProposedAndTheSkillIsLeftAlone(t *testing.T) {
	built, folder, model := benchWithARebuiltPage(t, "The one that matches is e9.")
	built.channel.AnswerPreviewsWith(contract.AnswerReject)
	replayer, err := browser.New(browser.Options{Browser: built.worker, Model: model, Ask: built.channel.ShowPreview, Skills: built.store})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("cannot replay the broken recording: %v", err)
	}
	if report.Patch == "" {
		t.Fatalf("no patch was proposed for a step that could not find its element:\n%s", report)
	}
	if report.Applied {
		t.Fatalf("the patch was written down and the user said no:\n%s", report)
	}
	previews := built.channel.Previews()
	if len(previews) != 1 {
		t.Fatalf("the user was shown %d previews, want the one patch", len(previews))
	}
	if !strings.Contains(previews[0].Body, "e9") || !strings.Contains(previews[0].Body, "e1") {
		t.Errorf("the preview says %q, and it should say which element it moves from and to", previews[0].Body)
	}

	steps, err := browser.StepsOf(built.load(t, "fixture-walk"))
	if err != nil {
		t.Fatalf("cannot read the skill back: %v", err)
	}
	if steps[1].Element.Ref != "e1" {
		t.Errorf("the saved step now points at %s, and a refused patch changes nothing", steps[1].Element)
	}
}

func TestAnApprovedPatchIsWrittenIntoTheSkillAndItsChangelog(t *testing.T) {
	built, folder, model := benchWithARebuiltPage(t, "e9")
	replayer, err := browser.New(browser.Options{Browser: built.worker, Model: model, Ask: built.channel.ShowPreview, Skills: built.store})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("cannot replay the broken recording: %v", err)
	}
	if !report.Applied {
		t.Fatalf("the user said yes and the patch was not written down:\n%s", report)
	}
	if !report.Met() {
		t.Fatalf("the healed replay says a step did not work:\n%s", report)
	}
	if !report.Outcomes[1].Healed {
		t.Errorf("the second step is not marked as healed: %+v", report.Outcomes[1])
	}

	steps, err := browser.StepsOf(built.load(t, "fixture-walk"))
	if err != nil {
		t.Fatalf("cannot read the patched skill back: %v", err)
	}
	wanted := browser.Descriptor{Ref: "e9", Role: "link", Name: "Onwards", Shown: "Onwards"}
	if steps[1].Element != wanted {
		t.Errorf("the patched step points at %+v, want %+v", steps[1].Element, wanted)
	}
	if steps[0].Address != theRebuiltPage || steps[1].Expectation != "the page changed" {
		t.Errorf("the patch changed more than the one descriptor: %+v", steps)
	}

	changelog, err := built.store.Changelog("fixture-walk")
	if err != nil {
		t.Fatalf("cannot read the changelog: %v", err)
	}
	for _, wantedLine := range []string{"self-heal", "e9", "keeping the copy it replaced"} {
		if !strings.Contains(changelog, wantedLine) {
			t.Errorf("the changelog does not mention %q:\n%s", wantedLine, changelog)
		}
	}
}

func TestAPatchIsProposedAndNotWrittenWhenThereIsNoScreenToAskOn(t *testing.T) {
	built, folder, model := benchWithARebuiltPage(t, "e9")
	replayer, err := browser.New(browser.Options{Browser: built.worker, Model: model, Skills: built.store})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("cannot replay the broken recording: %v", err)
	}
	if report.Patch == "" || report.Applied {
		t.Fatalf("an unattended replay either proposed nothing or wrote it down anyway:\n%s", report)
	}
	if !strings.Contains(report.String(), "proposed and not made") {
		t.Errorf("the report does not say the change was not made:\n%s", report)
	}
}

func TestAModelThatNamesNothingOnThePageHealsNothing(t *testing.T) {
	built, folder, model := benchWithARebuiltPage(t, "none of them matches")
	replayer, err := browser.New(browser.Options{Browser: built.worker, Model: model, Ask: built.channel.ShowPreview, Skills: built.store})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("cannot replay the broken recording: %v", err)
	}
	if report.Met() || report.Patch != "" {
		t.Fatalf("a model that named no element still healed something:\n%s", report)
	}
	if seen := report.Outcomes[1].Seen; !strings.Contains(seen, "named none") {
		t.Errorf("the report says %q, and it should say the model named nothing on the page", seen)
	}
}

func TestAModelWhoseAnswerDoesNotWorkHealsNothing(t *testing.T) {
	built, folder, model := benchWithARebuiltPage(t, "e9")
	built.worker.NextActionChangesNothing()
	replayer, err := browser.New(browser.Options{Browser: built.worker, Model: model, Ask: built.channel.ShowPreview, Skills: built.store})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("cannot replay the broken recording: %v", err)
	}
	if report.Met() || report.Applied {
		t.Fatalf("an answer that did not work was taken as a heal:\n%s", report)
	}
	if seen := report.Outcomes[1].Seen; !strings.Contains(seen, "did not do what the step should") {
		t.Errorf("the report says %q, and it should say acting on the answer did not work", seen)
	}
}

func TestOnlyOneStepOfAReplayIsEverPutToTheModel(t *testing.T) {
	built, _, model := benchWithARebuiltPage(t, "nothing here")
	folder := built.save(t, "two-broken", []browser.Step{
		{Number: 1, Intent: "Open the page.", Tool: contract.ToolBrowserOpen, Address: theRebuiltPage, Expectation: "the rebuilt page"},
		{
			Number: 2, Intent: "Follow the link that changes the page.", Tool: contract.ToolBrowserClick,
			Element:     browser.Descriptor{Ref: "e1", Role: "link", Name: "Change the page", Shown: "Change the page"},
			Expectation: "the page changed",
		},
		{
			Number: 3, Intent: "Follow it again.", Tool: contract.ToolBrowserClick,
			Element:     browser.Descriptor{Ref: "e2", Role: "link", Name: "Change the page", Shown: "Change the page"},
			Expectation: "the page changed",
		},
	})
	replayer, err := browser.New(browser.Options{Browser: built.worker, Model: model, Ask: built.channel.ShowPreview, Skills: built.store})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("cannot replay the broken recording: %v", err)
	}
	if len(report.Outcomes) != 2 {
		t.Fatalf("the replay ran %d steps, and it should stop at the one heal it is allowed:\n%s", len(report.Outcomes), report)
	}
	if calls := len(model.Requests()); calls != browser.HealAttempts {
		t.Errorf("the model was asked %d times, and one replay allows %d", calls, browser.HealAttempts)
	}
}

func TestAnOpeningStepThatFailedIsNotPutToTheModel(t *testing.T) {
	built := newBench(t)
	model := testkit.NewFakeModel(testkit.Script{Name: "nobody-calls-me", ContextLength: 8000})
	folder := built.save(t, "fixture-walk", []browser.Step{{
		Number: 1, Intent: "Open the page.", Tool: contract.ToolBrowserOpen,
		Address: testkit.FixtureSimplePage, Expectation: "the shopping basket is on the screen",
	}})
	replayer, err := browser.New(browser.Options{Browser: built.worker, Model: model, Ask: built.channel.ShowPreview, Skills: built.store})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	report, err := replayer.Replay(context.Background(), folder)
	if err != nil {
		t.Fatalf("cannot replay the recording: %v", err)
	}
	if calls := len(model.Requests()); calls != 0 {
		t.Errorf("the model was asked %d times about an opening step, and it has no element to look for", calls)
	}
	if seen := report.Outcomes[0].Seen; !strings.Contains(seen, "opening step has no element") {
		t.Errorf("the report says %q, and it should say why an opening step cannot be healed", seen)
	}
}
