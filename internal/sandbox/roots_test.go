package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// tempUserHome makes a home directory with the agent's own folder and an SSH
// folder inside it, which is the shape every root rule is written against.
func tempUserHome(t *testing.T) string {
	t.Helper()
	userHome := t.TempDir()
	for _, folder := range []string{
		filepath.Join(userHome, contract.HomeFolderName),
		filepath.Join(userHome, contract.HomeFolderName, "browser"),
		filepath.Join(userHome, ".ssh"),
		filepath.Join(userHome, "work"),
	} {
		if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
			t.Fatalf("cannot make the folder %s for the test: %v", folder, err)
		}
	}
	return userHome
}

func TestCheckRootsAcceptsAFolderBesideTheAgentsOwnHome(t *testing.T) {
	userHome := tempUserHome(t)
	work := filepath.Join(userHome, "work")

	checked, err := checkRoots([]string{work + "/"}, userHome, "")
	if err != nil {
		t.Fatalf("a folder beside the agent's home was refused: %v", err)
	}
	if len(checked) != 1 || checked[0] != work {
		t.Errorf("the checked roots are %v, want the cleaned path %q", checked, work)
	}
}

func TestCheckRootsRefusesEveryPathThatMustStayOutside(t *testing.T) {
	userHome := tempUserHome(t)

	for _, forbidden := range contract.ExcludedFromSandbox(userHome, "") {
		if _, err := checkRoots([]string{forbidden}, userHome, ""); err == nil {
			t.Errorf("the root %q was accepted, and it must stay outside the fence", forbidden)
		}
	}
}

func TestCheckRootsRefusesAFolderInsideThePathsThatMustStayOutside(t *testing.T) {
	userHome := tempUserHome(t)
	inside := filepath.Join(userHome, contract.HomeFolderName, "browser", "default")

	_, err := checkRoots([]string{inside}, userHome, "")
	if err == nil {
		t.Fatalf("the root %q was accepted, and it sits inside the browser profile", inside)
	}
	if !strings.Contains(err.Error(), "browser") {
		t.Errorf("the refusal says %q, and it must name the folder that must stay outside", err)
	}
}

func TestCheckRootsRefusesAParentOfTheAgentsHome(t *testing.T) {
	userHome := tempUserHome(t)

	_, err := checkRoots([]string{userHome}, userHome, "")
	if err == nil {
		t.Fatal("the user's whole home directory was accepted as a root, and it holds the agent's own home")
	}
	if !strings.Contains(err.Error(), contract.HomeFolderName) {
		t.Errorf("the refusal says %q, and it must name the folder it would have put inside the fence", err)
	}
}

func TestCheckRootsRefusesTheWholeFilesystem(t *testing.T) {
	userHome := tempUserHome(t)

	if _, err := checkRoots([]string{"/"}, userHome, ""); err == nil {
		t.Fatal("the whole filesystem was accepted as a root, and it holds everything that must stay outside")
	}
}

func TestCheckRootsRefusesAPathThatIsNotAFullPath(t *testing.T) {
	userHome := tempUserHome(t)

	if _, err := checkRoots([]string{"work"}, userHome, ""); err == nil {
		t.Fatal("a relative path was accepted as a root, and a root must be a full path")
	}
}

func TestCheckRootsRefusesAListWithNoRootsInIt(t *testing.T) {
	userHome := tempUserHome(t)

	if _, err := checkRoots(nil, userHome, ""); err == nil {
		t.Fatal("an empty list of roots was accepted, and a fence with no root can reach nothing")
	}
}

func TestCheckRootsRefusesMoreRootsThanTheCap(t *testing.T) {
	userHome := tempUserHome(t)

	roots := []string{}
	for index := 0; index <= MaxRoots; index++ {
		folder := filepath.Join(userHome, "work", "root", string(rune('a'+index)))
		if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
			t.Fatalf("cannot make the folder %s for the test: %v", folder, err)
		}
		roots = append(roots, folder)
	}

	if _, err := checkRoots(roots, userHome, ""); err == nil {
		t.Fatalf("%d roots were accepted, and the cap is %d", len(roots), MaxRoots)
	}
}

func TestCheckRootsRefusesARootThatIsNotThere(t *testing.T) {
	userHome := tempUserHome(t)
	missing := filepath.Join(userHome, "work", "nowhere")

	if _, err := checkRoots([]string{missing}, userHome, ""); err == nil {
		t.Fatalf("the root %q was accepted, and there is no such folder", missing)
	}
}

func TestCheckRootsRefusesARootThatIsAFileRatherThanAFolder(t *testing.T) {
	userHome := tempUserHome(t)
	note := filepath.Join(userHome, "work", "note.txt")
	if err := os.WriteFile(note, []byte("hello"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file for the test: %v", err)
	}

	if _, err := checkRoots([]string{note}, userHome, ""); err == nil {
		t.Fatalf("the root %q was accepted, and it is a file rather than a folder", note)
	}
}

func TestCheckRootsRefusesAnEmptyUserHome(t *testing.T) {
	if _, err := checkRoots([]string{"/tmp"}, "", ""); err == nil {
		t.Fatal("the roots were checked with no home directory to check them against")
	}
}

func TestCheckRootsRefusesARootThatHoldsTheAgentsHomeWhereCoeusHomeMovedIt(t *testing.T) {
	userHome := tempUserHome(t)
	work := filepath.Join(userHome, "work")
	agentHome := filepath.Join(work, "agenthome")
	if err := os.MkdirAll(agentHome, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the moved agent home for the test: %v", err)
	}

	_, err := checkRoots([]string{work}, userHome, agentHome)
	if err == nil {
		t.Fatalf("the root %q was accepted, and COEUS_HOME put the agent's home, its vault, and its browser profile inside it", work)
	}
	if !strings.Contains(err.Error(), agentHome) {
		t.Errorf("the refusal says %q, and it must name the folder it would have put inside the fence", err)
	}
}

func TestCheckRootsAcceptsARootBesideTheAgentsHomeWhereCoeusHomeMovedIt(t *testing.T) {
	userHome := tempUserHome(t)
	work := filepath.Join(userHome, "work")
	agentHome := filepath.Join(userHome, "somewhere-else")
	if err := os.MkdirAll(agentHome, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the moved agent home for the test: %v", err)
	}

	if _, err := checkRoots([]string{work}, userHome, agentHome); err != nil {
		t.Fatalf("a folder beside the moved agent home was refused: %v", err)
	}
}

func TestCheckRootsKeepsWhereALinkLeadsRatherThanTheLinkItself(t *testing.T) {
	userHome := tempUserHome(t)
	work := filepath.Join(userHome, "work")
	link := filepath.Join(userHome, "link-to-work")
	if err := os.Symlink(work, link); err != nil {
		t.Fatalf("cannot make the link for the test: %v", err)
	}
	wanted, err := filepath.EvalSymlinks(work)
	if err != nil {
		t.Fatalf("cannot work out where the link leads: %v", err)
	}

	checked, err := checkRoots([]string{link}, userHome, "")
	if err != nil {
		t.Fatalf("a link to a folder beside the agent's home was refused: %v", err)
	}
	if len(checked) != 1 || checked[0] != wanted {
		t.Errorf("the checked roots are %v, want where the link leads, %q, because bwrap binds the folder a link leads to and Landlock hangs its rule there",
			checked, wanted)
	}
}

func TestCheckRootsRefusesALinkThatLeadsSomewhereThatMustStayOutside(t *testing.T) {
	userHome := tempUserHome(t)
	targets := map[string]string{
		"the-home-directory": userHome,
		"the-ssh-folder":     filepath.Join(userHome, ".ssh"),
		"the-agents-home":    filepath.Join(userHome, contract.HomeFolderName),
	}

	for what, target := range targets {
		link := filepath.Join(userHome, "work", "link-to-"+what)
		if err := os.Symlink(target, link); err != nil {
			t.Fatalf("cannot make the link for the test: %v", err)
		}
		if _, err := checkRoots([]string{link}, userHome, ""); err == nil {
			t.Errorf("the root %q was accepted, and it is a link leading to %q, which must stay outside the fence", link, target)
		}
	}
}

func TestCheckRootsRefusesAPathTheCallerNamedAsOneToKeepOutside(t *testing.T) {
	userHome := tempUserHome(t)
	profile := filepath.Join(userHome, "chrome-profile")
	if err := os.MkdirAll(profile, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the folder for the test: %v", err)
	}

	if _, err := checkRoots([]string{profile}, userHome, "", profile); err == nil {
		t.Error("the browser profile the configuration moved was accepted as a sandbox root, and the cookies in it are the agent's own logins")
	}
}

func TestCheckRootsRefusesARootThatHoldsAPathTheCallerNamedAsOneToKeepOutside(t *testing.T) {
	userHome := tempUserHome(t)
	work := filepath.Join(userHome, "work")
	backups := filepath.Join(work, "backups")
	if err := os.MkdirAll(backups, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the folder for the test: %v", err)
	}

	_, err := checkRoots([]string{work}, userHome, "", backups)
	if err == nil {
		t.Fatalf("the root %q was accepted and it holds the backup folder %q, which the caller said must stay outside", work, backups)
	}
	if !strings.Contains(err.Error(), backups) {
		t.Errorf("the refusal says %q, and it must name the path it was refused for", err)
	}
}

func TestCheckRootsStillAcceptsARootBesideThePathTheCallerNamed(t *testing.T) {
	userHome := tempUserHome(t)
	work := filepath.Join(userHome, "work")
	profile := filepath.Join(userHome, "chrome-profile")

	if _, err := checkRoots([]string{work}, userHome, "", profile); err != nil {
		t.Errorf("the root %q was refused for the sake of %q, which is beside it rather than inside it: %v", work, profile, err)
	}
}
