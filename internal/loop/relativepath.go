package loop

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// A task of a job works in the job's project folder, and the model names the
// project's files the way the map and the standing order do, by their bare
// names: "read REPO_MAP.md", "read game.js". The tools resolve a relative path
// against the home's work folder, which is somewhere else, so on run twenty
// every such read failed and cost a round. A relative path in a call to one
// of the file tools is put under the project folder before the guard, the
// permission function and the tool see it; a label, an ask heading, a home
// path and an absolute path are left alone.

// theToolsThatTakeAPath are the tools whose "path" is a file or folder.
var theToolsThatTakeAPath = map[string]bool{contract.ToolRead: true, contract.ToolWrite: true, contract.ToolEdit: true, contract.ToolSearch: true}

// aResultLabel is what the read tool takes as a past result, r7, or a task's
// report, j4.2.
var aResultLabel = regexp.MustCompile(`^[rj]\d+(\.\d+)*$`)

// underTheProjectFolder is the path a tool is given: the bare path under the
// folder when it is relative and names a file, and the path as written
// otherwise.
func underTheProjectFolder(folder string, tool string, path string) string {
	if folder == "" || path == "" || !theToolsThatTakeAPath[tool] {
		return path
	}
	if filepath.IsAbs(path) || strings.HasPrefix(path, "~") {
		return path
	}
	if tool == contract.ToolRead && (aResultLabel.MatchString(path) || strings.HasPrefix(path, "ask ")) {
		return path
	}
	return filepath.Join(folder, path)
}

// inTheProjectFolderCall is the call with its path put under the folder, or
// the call as it came when there is nothing to change.
func inTheProjectFolderCall(folder string, call contract.ToolCall) contract.ToolCall {
	path := fieldOfCall(call, "path")
	moved := underTheProjectFolder(folder, call.Name, path)
	if moved == path {
		return call
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(call.Input, &fields); err != nil {
		return call
	}
	written, err := json.Marshal(moved)
	if err != nil {
		return call
	}
	fields["path"] = written
	input, err := json.Marshal(fields)
	if err != nil {
		return call
	}
	call.Input = input
	return call
}
