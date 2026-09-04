package computer_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/tool/computer"
)

func TestTheTypingCapIsTwentyThousandCharacters(t *testing.T) {
	if computer.MaxTextRunes != 20000 {
		t.Errorf("MaxTextRunes is %d, want twenty thousand: a model pasting a book into a window has lost its way",
			computer.MaxTextRunes)
	}

	tool, _ := newTool(t)
	_, err := run(t, tool, map[string]any{
		"intent": "fill the document", "action": "type",
		"text": strings.Repeat("a", 20001), "expectation": "the document holds it",
	})
	if err == nil {
		t.Errorf("twenty thousand and one characters were typed, and the cap is twenty thousand")
	}
}
