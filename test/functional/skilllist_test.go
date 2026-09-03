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

// The skill a person wrote is one skill in the home's skills folder, named and
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

	first := theFirstModelCall(t, agent)
	wanted := theSkillAPersonWroteName + ": " + theSkillAPersonWroteDescription
	if !strings.Contains(first, wanted) {
		t.Errorf("the model's first call does not carry the line %q, so the model cannot know the skill is there:\n%s", wanted, first)
	}
	if !strings.Contains(first, "`skill` tool") {
		t.Error("the model's first call names the skill and never says how to load it")
	}
}

// theFirstModelCall is everything the model read on the first call the agent
// made to it. The agent asks the server about itself before it ever sends a
// prompt, and that probe is recorded too, so the first call is the first request
// to a path that speaks a wire protocol.
func theFirstModelCall(t *testing.T, agent runningAgent) string {
	t.Helper()
	for _, asked := range agent.model.Requests() {
		if asked.Path == testkit.OpenAIPath || asked.Path == testkit.AnthropicPath {
			return testkit.WholeRequestBodyText(asked.Body)
		}
	}
	t.Fatal("the model was never called, so there is no first call to look at")
	return ""
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
