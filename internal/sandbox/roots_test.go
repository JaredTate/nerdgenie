package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
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
