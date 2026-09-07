package loop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The folder a task works in is the home's work folder, unless the task
// belongs to a job made from a work order whose Where named a folder. On 7
// September 2026 run sixteen built tic-tac-toe on the Desktop while the harness
// listed the work folder in the orientation, read no NERDGENIE.md, ran the
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

// learnTheFolder gives a task whose job named no folder the folder its own
// files share, so that the documents it writes at its end land with the work.
func (running *run) learnTheFolder() {
	if running.projectFolder != "" || running.task.FromJob == nil {
		return
	}
	running.projectFolder = theFolderTheFilesShare(running.filesChanged)
}

// keepTheLearnedFolder writes the folder a task's files share into the job
// record when the job has none yet, and returns the record as it is then.
func (theLoop *Loop) keepTheLearnedFolder(ctx context.Context, jobID string, held contract.Record, files []string) (contract.Record, error) {
	if projectFolderIn(held) != "" {
		return held, nil
	}
	folder := theFolderTheFilesShare(files)
	if folder == "" {
		return held, nil
	}
	if err := theLoop.options.Jobs.SetProjectFolder(ctx, jobID, folder); err != nil {
		return held, fmt.Errorf("cannot keep the folder job %s works in: %w", jobID, err)
	}
	again, err := theLoop.options.Jobs.Load(ctx, jobID)
	if err != nil {
		return held, fmt.Errorf("cannot read job %s after its folder was kept: %w", jobID, err)
	}
	return again, nil
}

// theFolderTheFilesShare is the deepest folder every absolute path is under,
// or the one file's own folder, and nothing when the paths share only the
// root or a home folder, which is no project.
func theFolderTheFilesShare(files []string) string {
	var shared []string
	for _, file := range files {
		whole := expandHome(file)
		if !filepath.IsAbs(whole) {
			continue
		}
		parts := strings.Split(filepath.Dir(whole), string(filepath.Separator))
		if shared == nil {
			shared = parts
			continue
		}
		keep := 0
		for keep < len(shared) && keep < len(parts) && shared[keep] == parts[keep] {
			keep++
		}
		shared = shared[:keep]
	}
	if len(shared) == 0 {
		return ""
	}
	folder := strings.Join(shared, string(filepath.Separator))
	if folder == "" || folder == string(filepath.Separator) || folder == expandHome("~") {
		return ""
	}
	return folder
}

// writeTheProjectDocuments leaves the standing order and the map in the
// project folder at the end of a job's task, so the next task starts from
// them instead of from the files.
func (theLoop *Loop) writeTheProjectDocuments(held contract.Record) {
	folder := theLoop.folderOfTheJob(held)
	if folder == "" || !theFolderExists(folder) {
		return
	}
	writeTheMap(folder)
	theLoop.writeTheStandingOrder(held)
}
