package browserhandoff_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/tool/browserhandoff"
)

func TestTheReasonCapIsFiveHundredCharacters(t *testing.T) {
	if browserhandoff.MaxReasonRunes != 500 {
		t.Errorf("MaxReasonRunes is %d, want five hundred: a person reading it on a phone will not read more than that",
			browserhandoff.MaxReasonRunes)
	}

	tool, _ := newTool(t, "the user said yes", nil)
	_, err := run(t, tool, map[string]any{"intent": "ask the user", "reason": strings.Repeat("a", 501)})
	if err == nil {
		t.Errorf("a reason of 501 characters was put to the user, and the cap is five hundred")
	}
}
