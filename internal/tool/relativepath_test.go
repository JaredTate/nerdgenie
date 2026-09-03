package tool_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/tool"
)

// aWorkArea makes a folder the file tools may work in, under a user home of its
// own, and returns the check over it with that same folder named as the one the
// agent works in, because that is how the program wires it. It hands back the
// check, the folder, and the user home.
func aWorkArea(t *testing.T) (tool.PathCheck, string, string) {
	t.Helper()
	userHome := t.TempDir()
	root := filepath.Join(userHome, contract.WorkFolderName)
	if err := os.MkdirAll(root, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the work folder %s: %v", root, err)
	}
	check := tool.NewPathCheck(tool.WorkArea{
		Roots:         []string{root},
		WorkingFolder: root,
		UserHome:      userHome,
	})
	return check, root, userHome
}

func TestAPathThatDoesNotStartAtTheRootIsTakenFromTheFolderTheAgentWorksIn(t *testing.T) {
	check, root, _ := aWorkArea(t)

	// This is the path the local model wrote on the first human trial.
	allowed, err := check("haiku.txt")
	if err != nil {
		t.Fatalf("the short path every model writes was refused: %v", err)
	}
	if wanted := filepath.Join(root, "haiku.txt"); allowed != wanted {
		t.Errorf("the check made %q of the short path, want %q", allowed, wanted)
	}
}

func TestAPathThatClimbsOutOfTheFolderTheAgentWorksInIsStillRefused(t *testing.T) {
	check, _, _ := aWorkArea(t)

	allowed, err := check(filepath.Join("..", "secrets.txt"))
	if err == nil {
		t.Fatalf("a short path that climbs out of every folder the agent may work in was allowed as %q", allowed)
	}
}

func TestTheRefusalOfAShortPathSaysWhatFolderItWasTakenFrom(t *testing.T) {
	check, root, _ := aWorkArea(t)

	_, err := check(filepath.Join("..", "secrets.txt"))
	if err == nil {
		t.Fatalf("a short path that climbs out of every folder the agent may work in was allowed")
	}
	if !strings.Contains(err.Error(), root) {
		t.Errorf("the refusal reads %q and never says the folder a short path is taken from, which is %s", err, root)
	}
}

func TestARefusedWholePathIsNotToldItWasTakenFromAnyFolder(t *testing.T) {
	check, _, userHome := aWorkArea(t)

	_, err := check(filepath.Join(userHome, "secrets.txt"))
	if err == nil {
		t.Fatalf("a whole path outside every folder the agent may work in was allowed")
	}
	if strings.Contains(err.Error(), "was taken from") {
		t.Errorf("the refusal reads %q, and a path that starts at the root is taken from nowhere", err)
	}
}

func TestAPathThatBeginsWithTheHomeMarkIsReadAsTheUsersHomeFolder(t *testing.T) {
	check, root, _ := aWorkArea(t)
	work := filepath.Base(root)

	for _, written := range []string{
		"~/" + work + "/haiku.txt",
		"$HOME/" + work + "/haiku.txt",
		"${HOME}/" + work + "/haiku.txt",
	} {
		allowed, err := check(written)
		if err != nil {
			t.Fatalf("the path %q, which a model writes whenever it has seen a shell, was refused: %v", written, err)
		}
		if wanted := filepath.Join(root, "haiku.txt"); allowed != wanted {
			t.Errorf("the check made %q of %q, want %q", allowed, written, wanted)
		}
	}
}

func TestTheRefusalOfAPathUnderTheHomeSaysTheMarkIsReadAsTheHomeFolder(t *testing.T) {
	check, _, userHome := aWorkArea(t)

	// This is the path the local model wrote on the first human trial: a folder
	// under the home that is in none of the folders the agent may work in.
	_, err := check("~/Desktop/Tater Tots Tetrisv1/src/config.js")
	if err == nil {
		t.Fatalf("a path under the home but outside every folder the agent may work in was allowed")
	}
	if !strings.Contains(err.Error(), "~") || !strings.Contains(err.Error(), userHome) {
		t.Errorf("the refusal reads %q and does not say that ~ is read as the user's home folder, %s", err, userHome)
	}
}

func TestAPathWithTheHomeMarkInTheMiddleIsLeftAlone(t *testing.T) {
	check, root, _ := aWorkArea(t)

	allowed, err := check("notes/~backup/haiku.txt")
	if err != nil {
		t.Fatalf("a path with a tilde inside a name was refused: %v", err)
	}
	if wanted := filepath.Join(root, "notes", "~backup", "haiku.txt"); allowed != wanted {
		t.Errorf("the check made %q of it, want %q, because only a tilde at the front means the home folder", allowed, wanted)
	}
}

func TestAShortPathWithNoFolderToTakeItFromSaysWhatIsMissing(t *testing.T) {
	userHome := t.TempDir()
	root := filepath.Join(userHome, contract.WorkFolderName)
	check := tool.NewPathCheck(tool.WorkArea{Roots: []string{root}, UserHome: userHome})

	_, err := check("haiku.txt")
	if err == nil {
		t.Fatalf("a short path was allowed with no folder to take it from")
	}
	if !strings.Contains(err.Error(), "folder") {
		t.Errorf("the refusal reads %q and does not say a folder is what is missing", err)
	}
}
