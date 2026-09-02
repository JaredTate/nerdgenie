package contract_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

func TestASandboxRootThatIsALinkToAnExcludedPathIsRefused(t *testing.T) {
	userHome := t.TempDir()
	agentHome := filepath.Join(userHome, ".coeus")
	for _, folder := range []string{agentHome, filepath.Join(userHome, ".ssh"), filepath.Join(userHome, "work")} {
		if err := os.MkdirAll(folder, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(userHome, "work", "everything")
	if err := os.Symlink(userHome, link); err != nil {
		t.Fatal(err)
	}
	if err := contract.CheckSandboxRoot(link, userHome, agentHome); err == nil {
		t.Error("a root that is a link to the user's home directory was accepted, and it would put the keys inside the fence")
	}
	keysLink := filepath.Join(userHome, "work", "keys")
	if err := os.Symlink(filepath.Join(userHome, ".ssh"), keysLink); err != nil {
		t.Fatal(err)
	}
	if err := contract.CheckSandboxRoot(keysLink, userHome, agentHome); err == nil {
		t.Error("a root that is a link to the SSH keys was accepted")
	}
	if err := contract.CheckSandboxRoot(filepath.Join(userHome, "work"), userHome, agentHome); err != nil {
		t.Errorf("an ordinary work folder was refused: %v", err)
	}
}

func TestTheExcludedPathsFollowTheAgentHomeWhereverItIs(t *testing.T) {
	userHome := filepath.Join("/home", "someone")
	movedHome := filepath.Join("/srv", "agent")
	excluded := contract.ExcludedFromSandbox(userHome, movedHome)
	found := false
	for _, path := range excluded {
		if path == movedHome {
			found = true
		}
	}
	if !found {
		t.Errorf("the excluded paths %v do not include the moved agent home %s", excluded, movedHome)
	}
	if err := contract.CheckSandboxRoot(filepath.Join(movedHome, "browser"), userHome, movedHome); err == nil {
		t.Error("a root inside the moved agent home was accepted")
	}
}

func TestTheOutputCapPerCallHasADefault(t *testing.T) {
	if got := contract.DefaultConfig().Caps.OutputTokensPerCall; got != 8192 {
		t.Errorf("the default output cap per call is %d, want 8192", got)
	}
}

func TestAnActStepCanScroll(t *testing.T) {
	step := contract.ActStep{Method: "scroll", Direction: contract.ScrollDown, Amount: 3, Expectation: "more posts appear"}
	if step.Direction != contract.ScrollDown || step.Amount != 3 {
		t.Errorf("an act step did not hold its scroll fields: %+v", step)
	}
}

func TestTheSocketCanAskForAMaskedAnswerAndSayAlways(t *testing.T) {
	ask := contract.SocketEnvelope{Type: contract.SocketAsk, MaskInput: true, Text: "API key for anthropic"}
	if !ask.MaskInput {
		t.Error("an ask envelope cannot say to hide what is typed")
	}
	if contract.ApproveAlwaysText != "always" {
		t.Errorf("the always word is %q, want \"always\"", contract.ApproveAlwaysText)
	}
}
