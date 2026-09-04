package browsertype_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestTypingWithNoTextAtAllIsRefusedAndNamesTheField(t *testing.T) {
	tool, worker := newTool(t)

	_, err := run(t, tool, map[string]any{
		"intent": "fill the box", "element": testkit.FixtureUsernameRef, "expectation": "the box holds the name",
	})
	if err == nil {
		t.Fatalf("a call with no text typed an empty string and reported success")
	}
	if !strings.HasSuffix(err.Error(), `"text"`) {
		t.Errorf("the refusal reads %q and does not end with the name of the field to write", err)
	}
	if len(worker.TypedInto(testkit.FixtureUsernameRef)) != 0 {
		t.Errorf("the worker was asked to type %v, and nothing should have been typed", worker.TypedInto(testkit.FixtureUsernameRef))
	}
}

func TestTypingNothingOnPurposeStillClearsTheBox(t *testing.T) {
	tool, _ := newTool(t)

	if _, err := run(t, tool, map[string]any{
		"intent": "clear the box", "element": testkit.FixtureUsernameRef, "text": "",
		"expectation": "the box is empty",
	}); err != nil {
		t.Fatalf("typing an empty text on purpose was refused: %v", err)
	}
}

func TestTypingThatCallsTheTextSomethingElseStillTypesIt(t *testing.T) {
	names := []string{"value", "content", "input"}
	for _, name := range names {
		tool, worker := newTool(t)
		_, err := run(t, tool, map[string]any{
			"intent": "fill the box", "element": testkit.FixtureUsernameRef, name: "Ada",
			"expectation": "the box holds the name",
		})
		if err != nil {
			t.Errorf("typing with the text under %q was refused: %v", name, err)
			continue
		}
		if len(worker.TypedInto(testkit.FixtureUsernameRef)) != 1 || !strings.Contains(worker.TypedInto(testkit.FixtureUsernameRef)[0], "Ada") {
			t.Errorf("typing with the text under %q typed %v", name, worker.TypedInto(testkit.FixtureUsernameRef))
		}
	}
}
