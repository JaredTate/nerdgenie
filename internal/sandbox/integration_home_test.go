//go:build integration

package sandbox

// What a path the user says means inside the fence. The first human trial found
// the model reading $HOME/Desktop, believing the empty answer, and building a
// whole project in a scratch folder nobody could find, because the fence had
// moved the home directory somewhere else. A path has to mean the same thing on
// both sides of the fence, and the things that must stay outside have to be
// missing rather than moved.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// aRealFenceOverTwoFoldersInTheUsersHome builds a fence whose roots are two
// folders in a temporary home, the way a person's own machine is set up: some of
// the home is inside the fence and the rest of it is not.
func aRealFenceOverTwoFoldersInTheUsersHome(t *testing.T) (*Fence, string, []string) {
	t.Helper()
	userHome, _ := aTemporaryUserHome(t)

	roots := []string{filepath.Join(userHome, "coeus"), filepath.Join(userHome, "Desktop")}
	for _, root := range roots {
		if err := os.MkdirAll(root, contract.HomeFolderMode); err != nil {
			t.Fatalf("cannot make the folder %s: %v", root, err)
		}
	}
	if err := os.WriteFile(filepath.Join(roots[1], "a-note.txt"), []byte("a file on the desktop\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file on the desktop: %v", err)
	}

	thisProgram, err := os.Executable()
	if err != nil {
		t.Fatalf("cannot find this test binary on disk: %v", err)
	}
	fence, err := New(Settings{Roots: roots, UserHome: userHome, OutputCap: theToolOutputCap, HelperProgram: thisProgram})
	if err != nil {
		t.Fatalf("cannot build a fence over %v: %v", roots, err)
	}
	if err := fence.Available(); err != nil {
		t.Fatalf("this machine cannot make a fence, so none of these tests can run: %v", err)
	}
	return fence, userHome, roots
}

func TestTheHomeInsideTheFenceIsTheUsersOwnPath(t *testing.T) {
	fence, userHome, _ := aRealFenceOverTwoFoldersInTheUsersHome(t)

	said := insideTheFence(t, fence, "echo $HOME")

	if said != userHome {
		t.Errorf("the home directory inside the fence is %q and the user's own is %q; a path the user says has to mean the same thing "+
			"on both sides of the fence, or the model builds the work somewhere nobody can find it", said, userHome)
	}
}

func TestAFolderTheUserNamesInsideTheirHomeIsThereInsideTheFence(t *testing.T) {
	fence, _, _ := aRealFenceOverTwoFoldersInTheUsersHome(t)

	said := insideTheFence(t, fence, "ls ~/Desktop")

	if !strings.Contains(said, "a-note.txt") {
		t.Errorf("listing ~/Desktop inside the fence said %q, want the file that is really there", said)
	}
}

func TestTheFoldersThatStayOutsideAreMissingRatherThanEmptyInsideTheFence(t *testing.T) {
	fence, _, _ := aRealFenceOverTwoFoldersInTheUsersHome(t)

	for _, folder := range []string{"~/.ssh", "~/" + contract.HomeFolderName} {
		said := insideTheFence(t, fence, "ls "+folder)

		if !strings.Contains(said, "No such file") {
			t.Errorf("listing %s inside the fence said %q, want no such file; the rest of the home is an empty temporary folder "+
				"and the keys and the vault are simply not in it", folder, said)
		}
	}
}

func TestAWriteToAFolderTheUserNamesLandsInTheRealFolder(t *testing.T) {
	fence, _, roots := aRealFenceOverTwoFoldersInTheUsersHome(t)

	insideTheFence(t, fence, "echo 'the model wrote this' > ~/coeus/x")

	written, err := os.ReadFile(filepath.Join(roots[0], "x"))
	if err != nil {
		t.Fatalf("the file the command wrote to ~/coeus/x is not in the real folder: %v", err)
	}
	if !strings.Contains(string(written), "the model wrote this") {
		t.Errorf("the real ~/coeus/x holds %q, want what the command wrote", written)
	}
}
