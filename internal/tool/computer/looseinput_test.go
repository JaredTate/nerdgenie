package computer_test

import (
	"strings"
	"testing"
)

func TestAnActionInCapitalsOrWithDashesIsTheActionItPlainlyMeans(t *testing.T) {
	spellings := map[string]string{
		"CLICK":         "click",
		"Click":         "click",
		"set-clipboard": "set_clipboard",
		"SET_CLIPBOARD": "set_clipboard",
	}
	for written, meant := range spellings {
		tool, _ := newTool(t)
		_, err := run(t, tool, map[string]any{
			"intent": "act on the editor", "action": written, "element": 1, "text": "a note",
			"expectation": "the editor takes it",
		})
		if err != nil {
			t.Errorf("the action %q, which plainly means %s, was refused: %v", written, meant, err)
		}
	}
}

func TestAnElementWrittenAsAPageReferenceIsStillTheNumberOfAControl(t *testing.T) {
	tool, desktop := newTool(t)

	if _, err := run(t, tool, map[string]any{
		"intent": "press the button", "action": "click", "element": "e1",
		"expectation": "the button goes down",
	}); err != nil {
		t.Fatalf("an element written the way a browser page names one was refused: %v", err)
	}
	done := strings.Join(desktop.Actions(), ", ")
	if !strings.Contains(done, "click 1") {
		t.Errorf("the desktop did %q, want a click on the control numbered one", done)
	}
}

func TestAScreenshotAndAClipboardReadNeedNoExpectation(t *testing.T) {
	for _, action := range []string{"screenshot", "clipboard"} {
		tool, _ := newTool(t)
		if _, err := run(t, tool, map[string]any{"intent": "see what is on the screen", "action": action}); err != nil {
			t.Errorf("a %s was refused for having no expectation, and it changes nothing to expect anything of: %v", action, err)
		}
	}
}

func TestAnExpectationLongerThanTheCapIsRefusedAndOneWrittenAsNothingIsToo(t *testing.T) {
	tool, _ := newTool(t)

	_, err := run(t, tool, map[string]any{
		"intent": "press the button", "action": "click", "element": 1,
		"expectation": strings.Repeat("a", 301),
	})
	if err == nil {
		t.Errorf("an expectation of 301 characters was taken, and the cap is three hundred")
	}

	_, err = run(t, tool, map[string]any{
		"intent": "press the button", "action": "click", "element": 1, "expectation": "   ",
	})
	if err == nil {
		t.Errorf("an expectation written as spaces was taken as saying what should happen")
	}
}

func TestAStepThatChangesTheScreenStillSaysWhatItExpectsOfTheScreen(t *testing.T) {
	tool, _ := newTool(t)

	_, err := run(t, tool, map[string]any{"intent": "press the button", "action": "click", "element": 1})
	if err == nil {
		t.Fatalf("a click with no expectation was made")
	}
	if !strings.Contains(err.Error(), "screen") {
		t.Errorf("the refusal reads %q, and this tool drives the screen rather than a page", err)
	}
}
