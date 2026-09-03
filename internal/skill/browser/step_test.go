package browser_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/skill"
	"github.com/JaredTate/coeus/internal/skill/browser"
)

func TestARecordedStepSurvivesBeingWrittenOutAndReadBack(t *testing.T) {
	recorded := []browser.Step{
		{
			Number: 1, Intent: "Open the compose page.", Tool: contract.ToolBrowserOpen,
			Address: "https://fixture.test/simple", Expectation: "a simple page opens",
		},
		{
			Number: 2, Intent: "Write the note.", Tool: contract.ToolBrowserType, Typed: "nine years",
			Element:     browser.Descriptor{Ref: "e3", Role: "textbox", Name: "Note", Shown: "Note"},
			Expectation: "the note box holds the words",
		},
		{
			Number: 3, Intent: "Change the page.", Tool: contract.ToolBrowserClick,
			Element:     browser.Descriptor{Ref: "e1", Role: "link", Name: "Change the page", Shown: "Change the page"},
			Expectation: "the page changed",
		},
	}

	written := browser.RenderSteps(recorded)
	parsed, err := skill.ParseSteps(written)
	if err != nil {
		t.Fatalf("what the recorder wrote is not a steps file internal/skill can read: %v\n%s", err, written)
	}
	readBack, err := browser.ParseStepList(parsed)
	if err != nil {
		t.Fatalf("cannot read back the steps that were just written: %v\n%s", err, written)
	}
	if len(readBack) != len(recorded) {
		t.Fatalf("wrote %d steps and read back %d", len(recorded), len(readBack))
	}
	for at, step := range readBack {
		if step != recorded[at] {
			t.Errorf("step %d read back as %+v, want %+v", at+1, step, recorded[at])
		}
	}
}

func TestAWrittenStepIsAlsoACallTheOrdinaryRunnerCouldMake(t *testing.T) {
	written := string(browser.RenderSteps([]browser.Step{{
		Number: 1, Intent: "Change the page.", Tool: contract.ToolBrowserClick,
		Element:     browser.Descriptor{Ref: "e1", Role: "link", Name: "Change the page", Shown: "Change the page"},
		Expectation: "the page changed",
	}}))
	for _, wanted := range []string{`"intent":"Change the page."`, `"element":"e1"`, `"expectation":"the page changed"`} {
		if !strings.Contains(written, wanted) {
			t.Errorf("the written step does not carry %s, so the browser tool could not run it:\n%s", wanted, written)
		}
	}
}

func TestTheDescriptorSaysWhatItPointsAtInPlainWords(t *testing.T) {
	cases := []struct {
		descriptor browser.Descriptor
		wanted     string
	}{
		{browser.Descriptor{Ref: "e1", Role: "link", Name: "Change the page"}, `e1 (link "Change the page")`},
		{browser.Descriptor{Role: "button", Name: "Sign in"}, `the button "Sign in"`},
		{browser.Descriptor{Shown: "Sign in"}, `the element showing "Sign in"`},
		{browser.Descriptor{}, "no element at all"},
	}
	for _, one := range cases {
		if said := one.descriptor.String(); said != one.wanted {
			t.Errorf("the descriptor %+v says %q, want %q", one.descriptor, said, one.wanted)
		}
	}
}

func TestAStepThatCannotBeReplayedIsRefusedBySayingWhy(t *testing.T) {
	cases := []struct {
		name   string
		step   skill.Step
		wanted string
	}{
		{
			name:   "a tool this package does not drive",
			step:   skill.Step{Number: 1, Intent: "Run something.", Tool: contract.ToolShell, Input: `{}`, Expect: "anything"},
			wanted: "shell",
		},
		{
			name:   "input that is not JSON",
			step:   skill.Step{Number: 1, Intent: "Open it.", Tool: contract.ToolBrowserOpen, Input: `{`, Expect: "anything"},
			wanted: "cannot read",
		},
		{
			name:   "an opening step with no address",
			step:   skill.Step{Number: 1, Intent: "Open it.", Tool: contract.ToolBrowserOpen, Input: `{}`, Expect: "anything"},
			wanted: "no address",
		},
		{
			name:   "a click with nothing to find the element by",
			step:   skill.Step{Number: 2, Intent: "Click it.", Tool: contract.ToolBrowserClick, Input: `{}`, Expect: "anything"},
			wanted: "names no element",
		},
		{
			name:   "a step that says nothing about what should happen",
			step:   skill.Step{Number: 2, Intent: "Click it.", Tool: contract.ToolBrowserClick, Input: `{"element":"e1"}`},
			wanted: "what should happen",
		},
		{
			name: "a step too big to write back into a skill folder",
			step: skill.Step{
				Number: 2, Intent: "Type the book.", Tool: contract.ToolBrowserType,
				Input:  `{"element":"e3","text":"` + strings.Repeat("a", skill.MaxStepBytes) + `"}`,
				Expect: "the box holds it",
			},
			wanted: "bytes written into a skill folder",
		},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			_, err := browser.ParseStep(one.step)
			if err == nil {
				t.Fatalf("the step %+v was accepted, and it cannot be replayed", one.step)
			}
			if !strings.Contains(err.Error(), one.wanted) {
				t.Errorf("the refusal says %q, and it should mention %q", err, one.wanted)
			}
		})
	}
}

func TestTheExpectationIsTakenFromTheLineAPersonWouldEdit(t *testing.T) {
	step, err := browser.ParseStep(skill.Step{
		Number: 1, Intent: "Open it.", Tool: contract.ToolBrowserOpen,
		Input:  `{"url":"https://fixture.test/simple","expectation":"what the machine wrote"}`,
		Expect: "what the person wrote",
	})
	if err != nil {
		t.Fatalf("cannot read the step: %v", err)
	}
	if step.Expectation != "what the person wrote" {
		t.Errorf("the expectation is %q, and the expect line a person edits should win", step.Expectation)
	}
}

func TestTheExpectationFallsBackToTheInputWhenThereIsNoExpectLine(t *testing.T) {
	step, err := browser.ParseStep(skill.Step{
		Number: 1, Intent: "Open it.", Tool: contract.ToolBrowserOpen,
		Input: `{"url":"https://fixture.test/simple","expectation":"a simple page opens"}`,
	})
	if err != nil {
		t.Fatalf("cannot read the step: %v", err)
	}
	if step.Expectation != "a simple page opens" {
		t.Errorf("the expectation is %q, want the one inside the input", step.Expectation)
	}
}

func TestAStepListLongerThanTheCapIsRefused(t *testing.T) {
	steps := []skill.Step{}
	for number := 1; number <= browser.MaxRecordedSteps+1; number++ {
		steps = append(steps, skill.Step{
			Number: number, Intent: "Open it.", Tool: contract.ToolBrowserOpen,
			Input: `{"url":"https://fixture.test/simple"}`, Expect: "a simple page",
		})
	}
	if _, err := browser.ParseStepList(steps); err == nil {
		t.Fatal("a step list past the cap was accepted, and every list has a cap")
	}
}

func TestAFolderCarryingAScriptCannotBeReplayedInTheBrowser(t *testing.T) {
	folder := skill.Folder{Definition: skill.Definition{Name: "shell-thing"}, HasScript: true}
	if _, err := browser.StepsOf(folder); err == nil {
		t.Fatal("a skill that carries a script was accepted as a browser recording")
	}
}
