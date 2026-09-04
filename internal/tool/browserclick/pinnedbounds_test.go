package browserclick_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/browserclick"
)

func TestTheExpectationCapIsThreeHundredCharacters(t *testing.T) {
	if browserclick.MaxExpectationRunes != 300 {
		t.Errorf("MaxExpectationRunes is %d, want three hundred: what a step expects is one line, and a person reads it in a preview",
			browserclick.MaxExpectationRunes)
	}

	tool, _ := newTool(t)
	_, err := run(t, tool, map[string]any{
		"intent": "follow the link", "element": testkit.FixtureChangeLinkRef,
		"expectation": strings.Repeat("a", 301),
	})
	if err == nil {
		t.Errorf("an expectation of 301 characters was taken, and the cap is three hundred")
	}
}
