package command

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// TestTheShippedSoulNamesNerdGenieAndHisVoice pins the persona a fresh install
// starts with: the agent is Nerd Genie, and when he talks to the person he
// talks like Doc Brown from Back to the Future; what he writes into the record
// and into tool calls stays plain, because those are for the harness.
func TestTheShippedSoulNamesNerdGenieAndHisVoice(t *testing.T) {
	home := contract.NewHome(t.TempDir())
	soul := ""
	for _, file := range personaFiles(home) {
		if file.path == home.SoulFile() {
			soul = file.text
		}
	}
	for _, words := range []string{"Nerd Genie", "Doc Brown", "Back to the Future", "Great Scott", "record", "plain"} {
		if !strings.Contains(soul, words) {
			t.Errorf("the shipped soul says nothing about %q", words)
		}
	}
	if len(soul) > 4000 {
		t.Errorf("the shipped soul is %d bytes, and the context builder reads at most 4000", len(soul))
	}
}
