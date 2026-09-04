package testkit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestTheTemporaryHomeHasEveryFolderTheLayoutNames(t *testing.T) {
	home := testkit.NewTempHome(t)

	for _, folder := range home.Folders() {
		info, err := os.Stat(folder)
		if err != nil {
			t.Errorf("the folder %s was not made: %v", folder, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a folder", folder)
		}
		if info.Mode().Perm() != contract.HomeFolderMode {
			t.Errorf("the folder %s has mode %#o, want %#o", folder, info.Mode().Perm(), contract.HomeFolderMode)
		}
	}
}

func TestTheTemporaryHomeSetsTheHomeVariableForTheTest(t *testing.T) {
	home := testkit.NewTempHome(t)

	found, err := contract.DefaultHome()
	if err != nil {
		t.Fatalf("finding the default home folder failed: %v", err)
	}
	if found.Root != home.Root {
		t.Errorf("the default home folder is %q, want the temporary one at %q", found.Root, home.Root)
	}
}

func TestTheTemporaryHomeIsSomewhereTemporaryAndFreshEveryTime(t *testing.T) {
	home := testkit.NewTempHome(t)

	if !strings.HasSuffix(home.Root, string(filepath.Separator)+contract.HomeFolderName) {
		t.Errorf("the temporary home is at %q, want it to end in %q", home.Root, contract.HomeFolderName)
	}
	if !strings.HasPrefix(home.Root, os.TempDir()) {
		t.Errorf("the temporary home is at %q, and it must be under the temporary folder %q so nothing outlives the test",
			home.Root, os.TempDir())
	}

	another := testkit.NewTempHome(t)

	if another.Root == home.Root {
		t.Errorf("two temporary homes are both at %q, and each test needs one nobody else is writing into", home.Root)
	}
	if entries, err := os.ReadDir(another.SkillsFolder()); err != nil || len(entries) != 0 {
		t.Errorf("the second temporary home's skills folder holds %v (error %v), want a fresh empty one", entries, err)
	}
}
