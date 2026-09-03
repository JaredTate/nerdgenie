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

// TestTheShippedConfigurationSaysTheSandboxIsOffAndWhatTheFenceWouldDo pins the
// one setting a person is most likely to want to change. The line is written out
// rather than left to the default, so that whoever opens the file can see which
// way their machine is running and turn it the other way in one edit.
func TestTheShippedConfigurationSaysTheSandboxIsOffAndWhatTheFenceWouldDo(t *testing.T) {
	chosen := modelChoice{name: contract.LocalModelAlias, detected: true, alias: contract.DefaultConfig().Models[0]}
	written := configurationText(chosen, []modelChoice{chosen}, []string{"/home/someone/coeus"})

	lines := strings.Split(written, "\n")
	at := -1
	for offset, line := range lines {
		if strings.HasPrefix(line, "sandbox = ") {
			at = offset
		}
	}
	if at < 0 {
		t.Fatalf("the shipped configuration has no sandbox line at all:\n%s", written)
	}
	if lines[at] != `sandbox = "off"` {
		t.Errorf("the shipped configuration writes %q, want the default written out as %q", lines[at], `sandbox = "off"`)
	}
	if at < 2 || !strings.HasPrefix(lines[at-1], "#") || !strings.HasPrefix(lines[at-2], "#") {
		t.Fatalf("the sandbox line has no two-line comment above it:\n%s", strings.Join(lines[max(at-2, 0):at+1], "\n"))
	}
	comment := lines[at-2] + " " + lines[at-1]
	for _, wanted := range []string{"fence", "sandbox roots"} {
		if !strings.Contains(comment, wanted) {
			t.Errorf("the comment above the sandbox line is %q and does not say %q", comment, wanted)
		}
	}
}
