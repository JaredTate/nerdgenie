package tool_test

// What the file tools may touch when the sandbox is off. The fenced check is
// proved in roots_test.go; this file proves the other half of the switch: every
// path on the machine is allowed except the handful that stay outside whichever
// way the setting is turned.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/tool"
)

// anOpenCheck makes the check the file tools are given when the sandbox is off,
// over a user's home directory of its own with the agent's home inside it, and
// returns the check and those two paths.
func anOpenCheck(t *testing.T) (tool.PathCheck, string, string) {
	t.Helper()
	userHome := t.TempDir()
	agentHome := filepath.Join(userHome, contract.HomeFolderName)
	for _, folder := range []string{agentHome, filepath.Join(userHome, ".ssh")} {
		if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
			t.Fatalf("cannot make the folder %s: %v", folder, err)
		}
	}
	return tool.NewOpenPathCheck(userHome, agentHome), userHome, agentHome
}

func TestWithTheSandboxOffAPathAnywhereOnTheMachineIsAllowed(t *testing.T) {
	check, userHome, _ := anOpenCheck(t)
	elsewhere := filepath.Join(t.TempDir(), "somebody-elses-project", "main.go")

	for _, wanted := range []string{
		filepath.Join(userHome, "notes.md"),
		filepath.Join(userHome, "Code", "coeus", "README.md"),
		elsewhere,
		"/etc/hostname",
	} {
		allowed, err := check(wanted)
		if err != nil {
			t.Errorf("the path %s was refused with the sandbox off: %v", wanted, err)
			continue
		}
		if allowed != wanted {
			t.Errorf("the check was given %s and handed back %s, want the path itself", wanted, allowed)
		}
	}
}

func TestWithTheSandboxOffEveryPathThatMustStayOutsideIsStillRefused(t *testing.T) {
	userHome := t.TempDir()
	agentHome := filepath.Join(userHome, contract.HomeFolderName)
	profile := filepath.Join(userHome, "chrome-profile")
	backups := filepath.Join(userHome, "coeus-backups")
	check := tool.NewOpenPathCheck(userHome, agentHome, profile, backups)

	for _, forbidden := range contract.ExcludedFromSandbox(userHome, agentHome, profile, backups) {
		wanted := filepath.Join(forbidden, "anything")
		if _, err := check(wanted); err == nil {
			t.Errorf("the path %s was allowed, and it must stay outside the file tools however the sandbox is set", wanted)
		} else if !strings.Contains(err.Error(), forbidden) {
			t.Errorf("the refusal of %s reads %q and does not name what it is inside", wanted, err)
		}
	}
}

func TestWithTheSandboxOffALinkIntoAPathThatMustStayOutsideIsRefused(t *testing.T) {
	check, _, agentHome := anOpenCheck(t)
	link := filepath.Join(t.TempDir(), "shortcut")
	if err := os.Symlink(agentHome, link); err != nil {
		t.Fatalf("cannot make the link into the agent's home folder: %v", err)
	}

	if _, err := check(filepath.Join(link, "vault.age")); err == nil {
		t.Errorf("a link leading into %s was followed and allowed, and the vault is behind it", agentHome)
	}
}

func TestWithTheSandboxOffAHardLinkToTheVaultIsRefused(t *testing.T) {
	check, _, agentHome := anOpenCheck(t)
	vault := contract.NewHome(agentHome).VaultFile()
	if err := os.WriteFile(vault, []byte("the agent's secrets\n"), contract.SecretFileMode); err != nil {
		t.Fatalf("cannot write the fixture vault: %v", err)
	}
	innocent := filepath.Join(t.TempDir(), "innocent.txt")
	if err := os.Link(vault, innocent); err != nil {
		t.Fatalf("cannot make the hard link to the vault: %v", err)
	}

	if _, err := check(innocent); err == nil {
		t.Errorf("the file %s was allowed, and its other name is the vault, which has no link to follow", innocent)
	}
}

func TestWithTheSandboxOffAPathThatIsNotAWholePathIsStillRefused(t *testing.T) {
	check, _, _ := anOpenCheck(t)

	for _, written := range []string{"", "notes/today.md", "   "} {
		if _, err := check(written); err == nil {
			t.Errorf("the path %q was allowed, and a tool has no working directory to read it against", written)
		}
	}
}

func TestTheReadToolFollowsTheSandboxSettingWhicheverWayItIsTurned(t *testing.T) {
	settings, home := wholeSettings(t)
	outside := filepath.Join(filepath.Dir(home.Root), "to-the-bank.txt")
	if err := os.WriteFile(outside, []byte("dear sir\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file outside every sandbox root: %v", err)
	}

	settings.Configuration.Sandbox = contract.SandboxOff
	if _, err := readOneFileThrough(t, settings, outside); err != nil {
		t.Errorf("with the sandbox off the read tool refused %s, which is an ordinary file in the user's home: %v", outside, err)
	}
	settings.Configuration.Sandbox = contract.SandboxFence
	if _, err := readOneFileThrough(t, settings, outside); err == nil {
		t.Errorf("with the fence on the read tool allowed %s, which is outside every sandbox root", outside)
	}
}

// readOneFileThrough builds the registry these settings describe and asks its
// read tool for one file, which is how a test sees the check the file tools were
// really given.
func readOneFileThrough(t *testing.T, settings tool.Settings, path string) (contract.ToolOutput, error) {
	t.Helper()
	registry, err := tool.New(context.Background(), settings)
	if err != nil {
		t.Fatalf("building the registry failed: %v", err)
	}
	found, held := registry.Lookup(contract.ToolRead)
	if !held {
		t.Fatalf("the registry holds no tool called %q", contract.ToolRead)
	}
	return found.Run(context.Background(), json.RawMessage(fmt.Sprintf(`{"path":%q}`, path)))
}
