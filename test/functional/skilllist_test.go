// The whole-program test for the skill list: a home with a skill in its skills
// folder makes the model's very first call carry that skill's name and one-line
// description, so the model can load it by name with the `skill` tool. The
// first human trial found the list was never sent, and a skill the model has
// never heard of is a skill it cannot use.
package functional

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theSkillAPersonWrote is one skill in the home's skills folder, named and
// described in words no other part of the prompt uses.
const (
	theSkillAPersonWroteName        = "tidy-notes"
	theSkillAPersonWroteDescription = "Keeps the notes folder tidy and says what it moved."
)

func TestAHomeWithASkillTellsTheModelAboutItOnTheFirstCall(t *testing.T) {
	agent := startTheAgentWorkingIn(t, func(string) testkit.Script {
		return testkit.Script{Name: "local", ContextLength: 32768, Steps: []testkit.Step{{
			Text:   theReplyTheModelIsScriptedToGive,
			Finish: contract.FinishEnd,
			Usage:  contract.Usage{InputTokens: 300, OutputTokens: 8},
		}}}
	}, func(home contract.Home, _ string) {
		writeASkillWithOnlyItsDescription(t, home, theSkillAPersonWroteName, theSkillAPersonWroteDescription)
	})

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: "Say hello in five words."})
	screen.waitFor(t, contract.SocketReply, 30*time.Second)

	requests := agent.model.Requests()
	if len(requests) == 0 {
		t.Fatal("the model was never called, so there is no first call to look at")
	}
	first := testkit.WholeRequestBodyText(requests[0].Body)
	wanted := theSkillAPersonWroteName + ": " + theSkillAPersonWroteDescription
	if !strings.Contains(first, wanted) {
		t.Errorf("the model's first call does not carry the line %q, so the model cannot know the skill is there:\n%s", wanted, first)
	}
	if !strings.Contains(first, "`skill` tool") {
		t.Error("the model's first call names the skill and never says how to load it")
	}
}

// writeASkillWithOnlyItsDescription puts the smallest folder the store will list
// into the home: a SKILL.md with the name and the one line the prompt carries.
func writeASkillWithOnlyItsDescription(t *testing.T, home contract.Home, name string, description string) {
	t.Helper()
	folder := home.SkillFolder(name)
	if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
		t.Fatalf("making the skill folder failed: %v", err)
	}
	written := "# " + name + "\n\n" + description + "\n"
	if err := os.WriteFile(filepath.Join(folder, "SKILL.md"), []byte(written), contract.DataFileMode); err != nil {
		t.Fatalf("writing SKILL.md failed: %v", err)
	}
}
