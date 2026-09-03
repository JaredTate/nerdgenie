// Every number in this file is written out rather than read from the package it
// checks. A test that measures a bound against the constant which sets it passes
// whatever that constant becomes, which is how five of this package's bounds
// came to be pinned by nothing at all.

package browser_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/skill"
	"github.com/JaredTate/coeus/internal/skill/browser"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestTheBoundsOfThisPackageAreTheNumbersTheyAreWrittenDownAs(t *testing.T) {
	for _, one := range []struct {
		name  string
		is    int
		wants int
		why   string
	}{
		{"MaxRecordedSteps", browser.MaxRecordedSteps, 50, "a recording is saved as a skill step list, and a skill holds fifty steps"},
		{"HealAttempts", browser.HealAttempts, 1, "a second broken step means the page has moved further than one patch can follow"},
		{"MaxScreenshots", browser.MaxScreenshots, 50, "a visual check photographs every step, so it is also the longest walk"},
		{"MaxElementsShownToTheModel", browser.MaxElementsShownToTheModel, 100, "a page with thousands of elements must not be sent whole to answer one question"},
		{"MaxSeenRunes", browser.MaxSeenRunes, 300, "a page full of text must not fill a report"},
	} {
		if one.is != one.wants {
			t.Errorf("browser.%s is %d, want %d, because %s", one.name, one.is, one.wants, one.why)
		}
	}
}

func TestARecordingOfFiftyOneStepsIsRefused(t *testing.T) {
	steps := []skill.Step{}
	for number := 1; number <= 51; number++ {
		steps = append(steps, skill.Step{
			Number: number, Intent: "Open it.", Tool: contract.ToolBrowserOpen,
			Input: `{"url":"https://fixture.test/simple"}`, Expect: "a simple page",
		})
	}
	if _, err := browser.ParseStepList(steps); err == nil {
		t.Fatal("a recording of fifty-one steps was accepted, and fifty is the most a skill folder holds")
	}
	if _, err := browser.ParseStepList(steps[:50]); err != nil {
		t.Fatalf("a recording of fifty steps was refused, and fifty is what a skill folder holds: %v", err)
	}
}

func TestAWalkOfFiftyOneStepsIsRefusedAndOneOfFiftyIsNot(t *testing.T) {
	built := newBench(t)
	checker, err := browser.NewChecker(built.worker)
	if err != nil {
		t.Fatalf("cannot build the checker: %v", err)
	}
	steps := []browser.Step{}
	for number := 1; number <= 51; number++ {
		steps = append(steps, browser.Step{
			Number: number, Intent: "Open the app.", Tool: contract.ToolBrowserOpen,
			Address: testkit.FixtureSimplePage, Expectation: "a simple page",
		})
	}
	if _, err := checker.Check(context.Background(), browser.Request{Steps: steps, Into: t.TempDir()}); err == nil {
		t.Fatal("a walk of fifty-one steps was accepted, and fifty pictures is the most a check may write")
	}
	if _, err := checker.Check(context.Background(), browser.Request{Steps: steps[:50], Into: t.TempDir()}); err != nil {
		t.Fatalf("a walk of fifty steps was refused, and fifty pictures is what a check may write: %v", err)
	}
}

func TestTheHundredAndFirstElementOfAPageIsNotSentToTheModel(t *testing.T) {
	built := newBench(t)
	crowded := contract.Snapshot{URL: "https://fixture.test/crowded", Title: "Crowded", TabID: "t1"}
	for number := 1; number <= 105; number++ {
		crowded.Elements = append(crowded.Elements, contract.Element{
			Ref: "x" + strconv.Itoa(number), Role: "link", Name: "Thing " + strconv.Itoa(number),
		})
	}
	built.worker.AddPage(crowded)
	model := testkit.NewFakeModel(testkit.Script{Name: "healer", ContextLength: 8000, Steps: []testkit.Step{{
		Text: "none", Finish: contract.FinishEnd,
	}}})
	folder := built.save(t, "crowded-walk", []browser.Step{
		{Number: 1, Intent: "Open the page.", Tool: contract.ToolBrowserOpen, Address: crowded.URL, Expectation: "the crowded page"},
		{
			Number: 2, Intent: "Click the missing thing.", Tool: contract.ToolBrowserClick,
			Element: browser.Descriptor{Ref: "e1", Role: "link", Name: "Nowhere", Shown: "Nowhere"}, Expectation: "something happens",
		},
	})
	replayer, err := browser.New(browser.Options{Browser: built.worker, Model: model})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	if _, err := replayer.Replay(context.Background(), folder); err != nil {
		t.Fatalf("cannot replay the recording: %v", err)
	}
	requests := model.Requests()
	if len(requests) != 1 {
		t.Fatalf("the model was asked %d times, want the one question a self-heal asks", len(requests))
	}
	asked := testkit.WholeRequestText(requests[0])
	if !strings.Contains(asked, `x100 link "Thing 100"`) {
		t.Errorf("the hundredth element of the page was not sent to the model:\n%s", asked)
	}
	if strings.Contains(asked, `x101 link "Thing 101"`) {
		t.Errorf("the hundred-and-first element of the page was sent to the model:\n%s", asked)
	}
	if !strings.Contains(asked, "(5 more elements are not listed)") {
		t.Errorf("the model was not told how many elements it was not shown:\n%s", asked)
	}
}

func TestTheModelIsAskedForNoMoreThanTwoHundredTokensToNameOneElement(t *testing.T) {
	built, folder, model := benchWithARebuiltPage(t, "e9")
	replayer, err := browser.New(browser.Options{Browser: built.worker, Model: model, Ask: built.channel.ShowPreview, Skills: built.store})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}

	if _, err := replayer.Replay(context.Background(), folder); err != nil {
		t.Fatalf("cannot replay the broken recording: %v", err)
	}
	requests := model.Requests()
	if len(requests) != 1 {
		t.Fatalf("the model was asked %d times, want the one question a self-heal asks", len(requests))
	}
	if requests[0].MaxOutputTokens != 200 {
		t.Errorf("the self-heal lets the model write %d tokens, want the two hundred an answer of one reference needs",
			requests[0].MaxOutputTokens)
	}
}

func TestOneReplayHealsOnceEvenWhenTheFirstHealWorked(t *testing.T) {
	built, _, model := benchWithARebuiltPage(t, "e9")
	folder := built.save(t, "two-broken", []browser.Step{
		{Number: 1, Intent: "Open the page.", Tool: contract.ToolBrowserOpen, Address: theRebuiltPage, Expectation: "the rebuilt page"},
		{
			Number: 2, Intent: "Follow the link that changes the page.", Tool: contract.ToolBrowserClick,
			Element:     browser.Descriptor{Ref: "e1", Role: "link", Name: "Change the page", Shown: "Change the page"},
			Expectation: "the page changed",
		},
		{
			Number: 3, Intent: "Follow the second link nobody built.", Tool: contract.ToolBrowserClick,
			Element:     browser.Descriptor{Ref: "e77", Role: "link", Name: "Somewhere else", Shown: "Somewhere else"},
			Expectation: "the page changed again",
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
	if !report.Outcomes[1].Healed {
		t.Fatalf("the first heal did not work, and this test measures what happens after one that did:\n%s", report)
	}
	if report.Outcomes[2].Met || report.Outcomes[2].Healed {
		t.Errorf("the third step was healed as well, and one replay heals once:\n%s", report)
	}
	if calls := len(model.Requests()); calls != 1 {
		t.Errorf("the model was asked %d times in one replay, want the one heal a replay is allowed", calls)
	}
}
