package browser_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/skill"
	"github.com/JaredTate/coeus/internal/skill/browser"
	"github.com/JaredTate/coeus/internal/testkit"
)

// aWalkWithOneStateThatHoldsAndOneThatDoesNot is the check every test in this
// file starts from: the page really is a simple page with a form, and no invoice
// was ever paid on it.
func aWalkWithOneStateThatHoldsAndOneThatDoesNot() []browser.Step {
	return []browser.Step{
		{
			Number: 1, Intent: "Open the app.", Tool: contract.ToolBrowserOpen,
			Address: testkit.FixtureSimplePage, Expectation: "a simple page with a form on it",
		},
		{
			Number: 2, Intent: "Follow the link.", Tool: contract.ToolBrowserClick,
			Element:     browser.Descriptor{Ref: "e1", Role: "link", Name: "Change the page", Shown: "Change the page"},
			Expectation: "the invoice was paid in full",
		},
	}
}

func TestAVisualCheckReportsOneStateMetAndOneNotMet(t *testing.T) {
	built := newBench(t)
	into := t.TempDir()
	checker, err := browser.NewChecker(built.worker)
	if err != nil {
		t.Fatalf("cannot build the checker: %v", err)
	}

	result, err := checker.Check(context.Background(), browser.Request{
		Name: "the fixture app", Steps: aWalkWithOneStateThatHoldsAndOneThatDoesNot(), Into: into,
	})
	if err != nil {
		t.Fatalf("cannot check the fixture app: %v", err)
	}
	if len(result.Findings) != 2 {
		t.Fatalf("the check reports %d steps, want the two it walked:\n%s", len(result.Findings), result.Markdown())
	}
	if !result.Findings[0].Met {
		t.Errorf("the first state is reported as not met: %+v", result.Findings[0])
	}
	if result.Findings[1].Met {
		t.Errorf("the second state is reported as met, and no invoice was paid: %+v", result.Findings[1])
	}
	if result.Met() {
		t.Error("the whole check is reported as met, and one of its states was not")
	}
}

func TestAVisualCheckWritesALabelledPictureForEveryStep(t *testing.T) {
	built := newBench(t)
	into := t.TempDir()
	checker, err := browser.NewChecker(built.worker)
	if err != nil {
		t.Fatalf("cannot build the checker: %v", err)
	}

	result, err := checker.Check(context.Background(), browser.Request{
		Steps: aWalkWithOneStateThatHoldsAndOneThatDoesNot(), Into: into,
	})
	if err != nil {
		t.Fatalf("cannot check the fixture app: %v", err)
	}
	for _, finding := range result.Findings {
		if finding.Picture == "" {
			t.Fatalf("step %d has no picture: %+v", finding.Number, finding)
		}
		if filepath.Dir(finding.Picture) != into {
			t.Errorf("the picture for step %d is at %s, and it should be under %s", finding.Number, finding.Picture, into)
		}
		content, err := os.ReadFile(finding.Picture)
		if err != nil {
			t.Fatalf("cannot read the picture the check wrote: %v", err)
		}
		if !strings.HasPrefix(string(content), "\x89PNG") {
			t.Errorf("the picture for step %d is not a PNG, and it should be the page as it stood", finding.Number)
		}
	}
	if first := filepath.Base(result.Findings[0].Picture); first != "01-open-the-app.png" {
		t.Errorf("the first picture is called %q, and it should be named after the step it shows", first)
	}
}

func TestTheReportIsMarkdownThatSaysWhatWasSeenAndWhereThePicturesAre(t *testing.T) {
	built := newBench(t)
	into := t.TempDir()
	checker, err := browser.NewChecker(built.worker)
	if err != nil {
		t.Fatalf("cannot build the checker: %v", err)
	}

	result, err := checker.Check(context.Background(), browser.Request{
		Name: "the fixture app", Steps: aWalkWithOneStateThatHoldsAndOneThatDoesNot(), Into: into,
	})
	if err != nil {
		t.Fatalf("cannot check the fixture app: %v", err)
	}
	report := result.Markdown()
	for _, wanted := range []string{
		"# the fixture app",
		"- step 1 Open the app.: met.",
		"- step 2 Follow the link.: not met.",
		"a simple page with a form on it",
		"the invoice was paid in full",
		result.Findings[0].Picture,
		result.Findings[1].Picture,
	} {
		if !strings.Contains(report, wanted) {
			t.Errorf("the report does not carry %q:\n%s", wanted, report)
		}
	}
}

func TestAVisualCheckNeverTakesAStepThatCannotBeUndone(t *testing.T) {
	built := newBench(t)
	into := t.TempDir()
	folder := built.save(t, "fixture-walk", theThreeStepFlow(), 3)
	checker, err := browser.NewChecker(built.worker)
	if err != nil {
		t.Fatalf("cannot build the checker: %v", err)
	}

	result, err := checker.CheckFolder(context.Background(), folder, "", into)
	if err != nil {
		t.Fatalf("cannot check the fixture app: %v", err)
	}
	if len(result.Findings) != 2 {
		t.Fatalf("the check walked %d steps, and it should stop before the third:\n%s", len(result.Findings), result.Markdown())
	}
	if !strings.Contains(result.StoppedBecause, "cannot be undone") {
		t.Errorf("the check stopped because %q, and it should say the step cannot be undone", result.StoppedBecause)
	}
	if !strings.Contains(result.Markdown(), "cannot be undone") {
		t.Errorf("the report does not say why the walk stopped:\n%s", result.Markdown())
	}
}

func TestACheckOfAFolderStartsAtTheAddressItIsGiven(t *testing.T) {
	built := newBench(t)
	into := t.TempDir()
	folder := built.save(t, "fixture-walk", []browser.Step{{
		Number: 1, Intent: "Open the app.", Tool: contract.ToolBrowserOpen,
		Address: "{{arguments}}", Expectation: "the page changed",
	}})
	checker, err := browser.NewChecker(built.worker)
	if err != nil {
		t.Fatalf("cannot build the checker: %v", err)
	}

	result, err := checker.CheckFolder(context.Background(), folder, testkit.FixtureChangedPage, into)
	if err != nil {
		t.Fatalf("cannot check the fixture app: %v", err)
	}
	if result.Address != testkit.FixtureChangedPage {
		t.Errorf("the check says it walked %q, want the address it was given", result.Address)
	}
	if !result.Met() {
		t.Errorf("the check did not open the address it was given:\n%s", result.Markdown())
	}
}

func TestAWalkStopsWhenAnElementIsNowhereOnThePage(t *testing.T) {
	built := newBench(t)
	into := t.TempDir()
	checker, err := browser.NewChecker(built.worker)
	if err != nil {
		t.Fatalf("cannot build the checker: %v", err)
	}

	result, err := checker.Check(context.Background(), browser.Request{Into: into, Steps: []browser.Step{
		{Number: 1, Intent: "Open the app.", Tool: contract.ToolBrowserOpen, Address: testkit.FixtureSimplePage, Expectation: "a simple page"},
		{
			Number: 2, Intent: "Click the button nobody built.", Tool: contract.ToolBrowserClick,
			Element:     browser.Descriptor{Ref: "e77", Role: "button", Name: "Buy it now", Shown: "Buy it now"},
			Expectation: "the basket has one thing in it",
		},
		{Number: 3, Intent: "Open it again.", Tool: contract.ToolBrowserOpen, Address: testkit.FixtureSimplePage, Expectation: "a simple page"},
	}})
	if err != nil {
		t.Fatalf("cannot check the fixture app: %v", err)
	}
	if len(result.Findings) != 2 {
		t.Fatalf("the check walked %d steps, and it cannot go on past an element it cannot find:\n%s", len(result.Findings), result.Markdown())
	}
	if !strings.Contains(result.StoppedBecause, "no element") {
		t.Errorf("the check stopped because %q, and it should say the element was not found", result.StoppedBecause)
	}
}

func TestAWalkLongerThanThePicturesItMayTakeIsRefused(t *testing.T) {
	built := newBench(t)
	checker, err := browser.NewChecker(built.worker)
	if err != nil {
		t.Fatalf("cannot build the checker: %v", err)
	}
	steps := []browser.Step{}
	for number := 1; number <= browser.MaxScreenshots+1; number++ {
		steps = append(steps, browser.Step{
			Number: number, Intent: "Open the app.", Tool: contract.ToolBrowserOpen,
			Address: testkit.FixtureSimplePage, Expectation: "a simple page",
		})
	}
	if _, err := checker.Check(context.Background(), browser.Request{Steps: steps, Into: t.TempDir()}); err == nil {
		t.Fatal("a walk past the picture cap was accepted, and every list has a cap")
	}
}

func TestAWalkWithNoStepsInItIsRefused(t *testing.T) {
	built := newBench(t)
	checker, err := browser.NewChecker(built.worker)
	if err != nil {
		t.Fatalf("cannot build the checker: %v", err)
	}
	if _, err := checker.Check(context.Background(), browser.Request{Into: t.TempDir()}); err == nil {
		t.Fatal("a walk with no steps was accepted, and it would check nothing")
	}
}

func TestACheckerWithNoBrowserBehindItIsRefused(t *testing.T) {
	if _, err := browser.NewChecker(nil); err == nil {
		t.Fatal("a checker was built with no browser behind it")
	}
}

func TestACheckWithNowhereToPutThePicturesIsRefused(t *testing.T) {
	built := newBench(t)
	checker, err := browser.NewChecker(built.worker)
	if err != nil {
		t.Fatalf("cannot build the checker: %v", err)
	}
	_, err = checker.Check(context.Background(), browser.Request{Steps: aWalkWithOneStateThatHoldsAndOneThatDoesNot()})
	if err == nil {
		t.Fatal("a check was run with nowhere to write its pictures")
	}
}

func TestTheShippedQualitySkillIsTheOneInTheSkillsFolder(t *testing.T) {
	shipped := browser.ShippedQASkill()
	folder := filepath.Join("..", "..", "..", "skills", "qa")
	for name, wanted := range shipped {
		onDisk, err := os.ReadFile(filepath.Join(folder, name))
		if err != nil {
			t.Fatalf("cannot read the shipped %s, and it is what the program installs: %v", name, err)
		}
		if string(onDisk) != string(wanted) {
			t.Errorf("skills/qa/%s is not what the program installs. Rewrite it from browser.ShippedQASkill().\non disk:\n%s\nwanted:\n%s",
				name, onDisk, wanted)
		}
	}
}

func TestTheShippedQualitySkillInstallsAndReadsBackAsAWalk(t *testing.T) {
	built := newBench(t)
	if err := browser.InstallQASkill(context.Background(), built.store); err != nil {
		t.Fatalf("cannot install the quality skill: %v", err)
	}

	folder := built.load(t, browser.QASkillName)
	steps, err := browser.StepsOf(folder)
	if err != nil {
		t.Fatalf("the shipped quality skill does not read back as a walk: %v", err)
	}
	if len(steps) < 2 {
		t.Fatalf("the shipped walk has %d steps, and it should show how a walk is written", len(steps))
	}
	if steps[0].Tool != contract.ToolBrowserOpen {
		t.Errorf("the shipped walk starts with %s, and a walk starts by opening the app", steps[0].Tool)
	}
	if len(folder.Definition.Permissions.IrreversibleSteps) == 0 {
		t.Error("the shipped walk marks no step as one that cannot be undone, and its last step files a report")
	}
	if err := skill.CheckName(folder.Definition.Name); err != nil {
		t.Errorf("the shipped skill is named %q, which is not a folder name: %v", folder.Definition.Name, err)
	}
}
