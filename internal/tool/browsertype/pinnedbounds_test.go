package browsertype_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/browsertype"
)

func TestTheTypingCapIsTwentyThousandCharacters(t *testing.T) {
	if browsertype.MaxTextRunes != 20000 {
		t.Errorf("MaxTextRunes is %d, want twenty thousand: a model pasting a book into a text field has lost its way",
			browsertype.MaxTextRunes)
	}

	tool, _ := newTool(t)
	_, err := run(t, tool, map[string]any{
		"intent": "fill the box", "element": testkit.FixtureUsernameRef,
		"text": strings.Repeat("a", 20001), "expectation": "the box holds it",
	})
	if err == nil {
		t.Errorf("twenty thousand and one characters were typed, and the cap is twenty thousand")
	}
}
