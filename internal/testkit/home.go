package testkit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// NewTempHome makes the whole ~/.nerdgenie layout under a folder it removes
// afterwards, with the modes the layout calls for, and points the HOME variable
// at it so that anything asking for the default home finds this one.
//
// The base folder is a short os.MkdirTemp path rather than t.TempDir(), because
// t.TempDir() embeds the test's full name, and a long-named test that binds
// <home>/run/agent.sock would push the socket path past the 108-byte limit a
// Unix socket path has.
func NewTempHome(t testing.TB) contract.Home {
	t.Helper()
	userHome, err := os.MkdirTemp("", "ng")
	if err != nil {
		t.Fatalf("cannot make a temporary home: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(userHome) })
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
