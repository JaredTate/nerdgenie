package loop

import (
	"path/filepath"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// A map written at a task's end is stale by the task's second write, and on
// run eighteen the model read the map at a task's start and found the
// entries of the files it had just written empty. So a write or an edit
// under the project folder puts that file's fresh entry into the map at
// once, when the map there is the harness's own. Nothing here fails a call.

// refreshTheMapAfter puts the written file's entry back into the folder's
// generated map, when the call was a write or an edit that succeeded and the
// file is under the folder.
func (running *run) refreshTheMapAfter(call contract.ToolCall, failed bool) {
	if failed || (call.Name != contract.ToolWrite && call.Name != contract.ToolEdit) {
		return
	}
	folder := running.folder()
	written := fieldOfCall(call, "path")
	if folder == "" || written == "" {
		return
	}
	relative, err := filepath.Rel(folder, aPathUnder(folder, written))
	if err != nil || relative == "." || strings.HasPrefix(relative, "..") {
		return
	}
	refreshTheMapEntry(folder, filepath.ToSlash(relative))
}
