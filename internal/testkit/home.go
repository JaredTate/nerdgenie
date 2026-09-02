package testkit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// NewTempHome makes the whole ~/.coeus layout under a folder the test framework
// removes afterwards, with the modes the layout calls for, and points the HOME
// variable at it so that anything asking for the default home finds this one.
func NewTempHome(t testing.TB) contract.Home {
	t.Helper()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	home := contract.NewHome(filepath.Join(userHome, contract.HomeFolderName))
	for _, folder := range home.Folders() {
		if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
			t.Fatalf("cannot make the folder %s for the temporary home: %v", folder, err)
		}
		if err := os.Chmod(folder, contract.HomeFolderMode); err != nil {
			t.Fatalf("cannot set the mode on %s: %v", folder, err)
		}
	}
	return home
}
