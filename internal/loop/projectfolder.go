package loop

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The folder a task works in is the home's work folder, unless the task
// belongs to a job made from a work order whose Where named a folder. On 7
// September 2026 run sixteen built tic-tac-toe on the Desktop while the harness
// listed the work folder in the orientation, read no AGENTS.md, ran the
// person's "npm test" check in a folder with no package.json, and unproved a
// done line the model had just proved. The job record's situation carries the
// folder as one line, and every task of the job reads it from there.

// projectFolderIn reads the project folder off a job record's situation, with
// a leading tilde expanded, or answers nothing when the job names none.
func projectFolderIn(held contract.Record) string {
	for _, line := range held.Work.Situation {
		if strings.HasPrefix(line, contract.ProjectFolderLine) {
			return expandHome(strings.TrimSpace(strings.TrimPrefix(line, contract.ProjectFolderLine)))
		}
	}
	return ""
}

// folder is where this task works: the job's project folder when it has one,
// and the home's work folder otherwise.
func (running *run) folder() string {
	if running.projectFolder != "" {
		return running.projectFolder
	}
	return running.theLoop.options.WorkingDirectory
}

// inTheProjectFolder wraps a check's command so it runs in the project folder
// when the task has one, the way the model itself changes folder first.
func (running *run) inTheProjectFolder(command string) string {
	if running.projectFolder == "" {
		return command
	}
	return "cd '" + strings.ReplaceAll(running.projectFolder, "'", `'\''`) + "' && " + command
}

// folderOfTheJob is where a finished job's documents go: its project folder
// when it has one, else the home's work folder.
func (theLoop *Loop) folderOfTheJob(held contract.Record) string {
	if folder := projectFolderIn(held); folder != "" {
		return folder
	}
	return theLoop.options.WorkingDirectory
}

// theFolderExists says whether the project folder is there to work in.
func theFolderExists(folder string) bool {
	info, err := os.Stat(folder)
	return err == nil && info.IsDir()
}

// aPathUnder joins a relative path to the folder and leaves an absolute one.
func aPathUnder(folder string, path string) string {
	whole := expandHome(path)
	if filepath.IsAbs(whole) {
		return whole
	}
	return filepath.Join(folder, whole)
}
