// The whole-program test for the skill that ships with the program: a fresh home
// folder has it, so "/skills" lists it the first time a person looks. A skill
// nobody installed is a skill nobody can use.
package functional

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	browserskill "github.com/JaredTate/nerdgenie/internal/skill/browser"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestTheShippedQualitySkillIsListedOnAFreshHome(t *testing.T) {
	agent := startTheAgent(t, testkit.Script{Name: "local", ContextLength: 32768})

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "skills"})

	listed := screen.waitFor(t, contract.SocketReply, 30*time.Second)
	if !strings.Contains(listed.Text, browserskill.QASkillName) {
		t.Errorf("the skills are %q, and the one that ships with the program is not among them", listed.Text)
	}
}

func TestTheShippedSkillIsNotWrittenOverAPersonsOwnCopy(t *testing.T) {
	agent := startTheAgentWorkingIn(t, func(string) testkit.Script {
		return testkit.Script{Name: "local", ContextLength: 32768}
	}, func(home contract.Home, _ string) {
		writeASkillOfTheirOwn(t, home, browserskill.QASkillName, "A person wrote this one")
	})

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "skills show " + browserskill.QASkillName})

	shown := screen.waitFor(t, contract.SocketReply, 30*time.Second)
	if !strings.Contains(shown.Text, "A person wrote this one") {
		t.Errorf("the skill shown is %q, and it is not the copy the person had already written", shown.Text)
	}
}
