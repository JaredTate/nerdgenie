package browser_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/skill"
	"github.com/JaredTate/coeus/internal/skill/browser"
)

// The seeds are the shapes a recorded step really takes, plus the shapes that
// would break a reader written carelessly: no input at all, an input that is not
// an object, an input that is not JSON, numbers where text belongs, a descriptor
// with nothing in it, and text that is not valid UTF-8.
var stepSeeds = []struct {
	tool   string
	input  string
	expect string
}{
	{contract.ToolBrowserOpen, `{"url":"https://fixture.test/simple"}`, "a simple page"},
	{contract.ToolBrowserClick, `{"element":"e1","role":"link","name":"Change the page","shown":"Change the page"}`, "the page changed"},
	{contract.ToolBrowserType, `{"element":"e3","role":"textbox","name":"Note","text":"nine years"}`, "the note box holds it"},
	{contract.ToolBrowserOpen, "", ""},
	{contract.ToolBrowserClick, "{", "the page changed"},
	{contract.ToolBrowserClick, "[]", "the page changed"},
	{contract.ToolBrowserClick, `{"element":4}`, "the page changed"},
	{contract.ToolBrowserType, `{"element":"","role":"","name":"","shown":""}`, "anything"},
	{contract.ToolShell, `{"command":"ls"}`, "a listing"},
	{"", "", ""},
	{contract.ToolBrowserOpen, "{\"url\":\"\xff\xfe\"}", "\xff"},
}

// FuzzParseStep throws anything at the reader of a recorded step. Nothing it is
// given may panic, and anything it accepts has to be replayable: a tool this
// package drives, an expectation to check, and either an address or something to
// find the element by.
func FuzzParseStep(f *testing.F) {
	for _, seed := range stepSeeds {
		f.Add(seed.tool, seed.input, seed.expect)
	}
	f.Fuzz(func(t *testing.T, tool string, input string, expect string) {
		step, err := browser.ParseStep(skill.Step{Number: 1, Intent: "Do the thing.", Tool: tool, Input: input, Expect: expect})
		if err != nil {
			return
		}
		if !browser.DrivenHere(step.Tool) {
			t.Fatalf("a step that parsed calls %q, which this package cannot replay", step.Tool)
		}
		if step.Expectation == "" {
			t.Fatal("a step that parsed says nothing about what should happen, so nothing could judge it")
		}
		if step.Tool == contract.ToolBrowserOpen && step.Address == "" {
			t.Fatal("an opening step that parsed has no address to open")
		}
		if step.Tool != contract.ToolBrowserOpen && step.Element.Empty() {
			t.Fatalf("a %s step that parsed names no element, so no rung of the cascade could find one", step.Tool)
		}
		if step.Element.String() == "" {
			t.Fatal("a step that parsed cannot say what element it points at")
		}
	})
}

// FuzzStepRoundTrip throws anything at a whole steps.md and proves that a
// recording which reads and writes once reads back the same the second time, so
// that a saved skill never quietly means something else than it did.
func FuzzStepRoundTrip(f *testing.F) {
	for _, seed := range stepSeeds {
		f.Add("1. Do the thing.\n   tool: " + seed.tool + "\n   input: " + seed.input + "\n   expect: " + seed.expect + "\n")
	}
	f.Add("")
	f.Add("1. one\n2. two\n")
	f.Add("3. numbered wrong\n")
	f.Fuzz(func(t *testing.T, content string) {
		written, err := skill.ParseSteps([]byte(content))
		if err != nil {
			return
		}
		steps, err := browser.ParseStepList(written)
		if err != nil {
			return
		}

		again, err := skill.ParseSteps(browser.RenderSteps(steps))
		if err != nil {
			t.Fatalf("a recording that read once did not survive being written out: %v", err)
		}
		readBack, err := browser.ParseStepList(again)
		if err != nil {
			t.Fatalf("a recording that read once did not read back the second time: %v", err)
		}
		if len(readBack) != len(steps) {
			t.Fatalf("read %d steps and read back %d", len(steps), len(readBack))
		}
		for at, step := range readBack {
			if step.Tool != steps[at].Tool || step.Element != steps[at].Element || step.Address != steps[at].Address {
				t.Fatalf("step %d read back as %+v, want %+v", at+1, step, steps[at])
			}
			if strings.TrimSpace(step.Expectation) == "" {
				t.Fatalf("step %d read back with no expectation", at+1)
			}
		}
	})
}
