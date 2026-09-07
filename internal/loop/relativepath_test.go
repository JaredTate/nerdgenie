package loop

import (
	"path/filepath"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// Run 20, tasks 2 and 3: the model read REPO_MAP.md, game.js and tests/ by
// their bare names, as the map rule says to, and every read failed against
// the home's work folder, one wasted round each. A relative path in a read, a
// write, an edit or a search is the project's own when the task has a
// project folder; a label, an ask heading and an absolute path are left as
// they are.
func TestARelativePathIsReadUnderTheProjectFolder(t *testing.T) {
	folder := filepath.Join(string(filepath.Separator), "home", "user", "Desktop", "Tic Tac Toe")
	for _, shape := range []struct{ tool, path, want string }{
		{contract.ToolRead, "REPO_MAP.md", filepath.Join(folder, "REPO_MAP.md")},
		{contract.ToolRead, "tests/", filepath.Join(folder, "tests")},
		{contract.ToolRead, "./game.js", filepath.Join(folder, "game.js")},
		{contract.ToolRead, "REPO_MAP.md src/game.js", filepath.Join(folder, "REPO_MAP.md src/game.js")},
		{contract.ToolWrite, "src/new.js", filepath.Join(folder, "src", "new.js")},
		{contract.ToolEdit, "game.js", filepath.Join(folder, "game.js")},
		{contract.ToolSearch, "src", filepath.Join(folder, "src")},
		{contract.ToolRead, "/etc/hosts", "/etc/hosts"},
		{contract.ToolRead, "~/notes.md", "~/notes.md"},
		{contract.ToolRead, "r7", "r7"},
		{contract.ToolRead, "j4.2", "j4.2"},
		{contract.ToolRead, "ask Engineering", "ask Engineering"},
		{contract.ToolRead, "", ""},
		{contract.ToolShell, "game.js", "game.js"},
	} {
		if got := underTheProjectFolder(folder, shape.tool, shape.path); got != shape.want {
			t.Errorf("%s %q under the folder reads %q, want %q", shape.tool, shape.path, got, shape.want)
		}
	}
	if got := underTheProjectFolder("", contract.ToolRead, "game.js"); got != "game.js" {
		t.Errorf("a task with no project folder rewrote %q to %q", "game.js", got)
	}
}

// TestTheRewrittenCallKeepsItsOtherFields: only the path changes; the
// section, the offset and the content ride through untouched.
func TestTheRewrittenCallKeepsItsOtherFields(t *testing.T) {
	folder := t.TempDir()
	call := contract.ToolCall{ID: "c1", Name: contract.ToolRead, Input: []byte(`{"path":"REPO_MAP.md","section":"game.js","offset":3}`)}
	moved := inTheProjectFolderCall(folder, call)
	if got := fieldOfCall(moved, "path"); got != filepath.Join(folder, "REPO_MAP.md") {
		t.Errorf("the path reads %q", got)
	}
	if got := fieldOfCall(moved, "section"); got != "game.js" {
		t.Errorf("the section reads %q after the rewrite, want it kept", got)
	}
	if string(moved.Input) == string(call.Input) || moved.ID != call.ID || moved.Name != call.Name {
		t.Errorf("the rewritten call reads %+v", moved)
	}
	same := inTheProjectFolderCall(folder, contract.ToolCall{Name: contract.ToolRead, Input: []byte(`{"path":"r7"}`)})
	if string(same.Input) != `{"path":"r7"}` {
		t.Errorf("a label's call was rewritten to %s", same.Input)
	}
}
