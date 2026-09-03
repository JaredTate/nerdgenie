package tool_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/tool"
)

// FuzzThePathCheckNeverLandsOutsideTheRoots throws whatever a model might write
// as a path at the check and asks one thing of every answer: a path the check
// allowed is inside the one folder the agent may work in. The check now makes a
// path whole before it judges it, taking a short path from the folder the agent
// works in and reading ~ as the user's home, so it turns text from outside into a
// place on the disk, and nothing it turns that text into may be outside the
// fence.
func FuzzThePathCheckNeverLandsOutsideTheRoots(f *testing.F) {
	userHome := f.TempDir()
	root := filepath.Join(userHome, contract.WorkFolderName)
	if err := os.MkdirAll(root, contract.HomeFolderMode); err != nil {
		f.Fatalf("cannot make the work folder %s: %v", root, err)
	}
	inside, err := filepath.EvalSymlinks(root)
	if err != nil {
		f.Fatalf("cannot follow the links along %s: %v", root, err)
	}
	check := tool.NewPathCheck(tool.WorkArea{
		Roots:         []string{root},
		WorkingFolder: root,
		UserHome:      userHome,
	})

	for _, seed := range []string{
		"haiku.txt", "notes/today.md", "../secrets.txt", "~", "~/haiku.txt",
		"$HOME/.ssh/id_rsa", "${HOME}/..//work", "/etc/passwd", "", "   ",
		"~backup", "./../../..", string([]byte{0}), "~/" + contract.WorkFolderName,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, written string) {
		allowed, err := check(written)
		if err != nil {
			return
		}
		if allowed != inside && !strings.HasPrefix(allowed, inside+string(filepath.Separator)) {
			t.Errorf("the check turned %q into %q, which is outside %s, the one folder the agent may work in", written, allowed, inside)
		}
	})
}
