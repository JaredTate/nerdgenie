package tool_test

// What the wave 6 security review reached through the file tools. Both tests
// work under a temporary home; neither goes anywhere near the user's own
// ~/.coeus or ~/.ssh.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/tool"
)

// aHomeWithAWorkFolder makes a user's home directory holding the agent's own
// folder, an SSH folder with a fixture key in it, and one work folder, and
// returns the three paths.
func aHomeWithAWorkFolder(t *testing.T) (string, string, string) {
	t.Helper()
	userHome := t.TempDir()
	agentHome := filepath.Join(userHome, contract.HomeFolderName)
	work := filepath.Join(userHome, contract.WorkFolderName)
	for _, folder := range []string{agentHome, filepath.Join(userHome, ".ssh"), work} {
		if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
			t.Fatalf("cannot make the folder %s: %v", folder, err)
		}
	}
	return userHome, agentHome, work
}

// TestTheCheckHandsBackThePathWithItsLinksFollowed holds the promise the
// PathCheck doc comment makes. A check that judges one path and hands back
// another leaves the window between the two open, and a sandboxed command may
// make and remake a link inside a root as often as it likes, because making a
// link only writes a string.
func TestTheCheckHandsBackThePathWithItsLinksFollowed(t *testing.T) {
	userHome, agentHome, work := aHomeWithAWorkFolder(t)
	inside := filepath.Join(work, "notes.md")
	if err := os.WriteFile(inside, []byte("ordinary work\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file inside the work folder: %v", err)
	}
	link := filepath.Join(work, "notes-link.md")
	if err := os.Symlink(inside, link); err != nil {
		t.Fatalf("cannot make the link: %v", err)
	}

	check := tool.NewPathCheck([]string{work}, userHome, agentHome)
	allowed, err := check(link)
	if err != nil {
		t.Fatalf("the check refused a link that leads inside the work folder: %v", err)
	}
	if allowed != inside {
		t.Errorf("the check was given %s and handed back %s, want %s; a caller that writes to the name rather than to the folder"+
			" the links lead to writes wherever the link points when the write happens, not where it pointed when the check ran",
			link, allowed, inside)
	}
}

// TestTheConfiguredBrowserProfileStaysOutsideTheFence holds the rule from design
// section 11 that the browser profile is always outside the sandbox. The paths
// that must stay outside used to be worked out from the default browser folder
// alone, so a browser_profile_path the user had moved into a sandbox root was
// inside the fence, and its cookies are the agent's logins. Both the sandbox
// check and the file tools' check are now told where the configured profile is,
// and both refuse a root that holds it.
func TestTheConfiguredBrowserProfileStaysOutsideTheFence(t *testing.T) {
	userHome, agentHome, work := aHomeWithAWorkFolder(t)
	profile := filepath.Join(work, "chrome-profile")
	cookies := filepath.Join(profile, "Default", "Cookies")
	if err := os.MkdirAll(filepath.Dir(cookies), contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the profile folder: %v", err)
	}
	if err := os.WriteFile(cookies, []byte("the agent's logins"), contract.SecretFileMode); err != nil {
		t.Fatalf("cannot write the fixture cookies: %v", err)
	}

	if err := contract.CheckSandboxRoot(work, userHome, agentHome, profile); err == nil {
		t.Errorf("the sandbox root %s holds the configured browser profile %s and was allowed, so a sandboxed command can read the cookies"+
			" that are the agent's logins; the configured profile has to join the paths that must stay outside the fence", work, profile)
	}
	if _, err := tool.NewPathCheck([]string{work}, userHome, agentHome, profile)(cookies); err == nil {
		t.Errorf("the file tools may read %s, and the model never reads the cookies that are the agent's logins", cookies)
	}
}
