package loop

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/codemap"
)

// A finished job leaves a map of its folder beside its standing order, so the
// next task and the next job start knowing where everything is without
// listing it: a legend of the top folders and a tree of the files. The map is
// generated, says so on its second line, and is written again by every job
// that finishes there; a map without that mark is the person's own and is
// never touched.

const (
	// MapFile is the map's name in the project folder.
	MapFile = "REPO_MAP.md"
	// GeneratedMark is the line that says the harness wrote the map, and is
	// what lets it write the map again.
	GeneratedMark = codemap.GeneratedMark
	// MaxMapEntries bounds the tree, because a map of ten thousand lines tells
	// nobody anything.
	MaxMapEntries = 2000
)

// foldersLeftOut are the folders no map lists: what a package manager or a
// build wrote, and what version control keeps.
var foldersLeftOut = map[string]bool{"node_modules": true, ".git": true, "dist": true, "build": true, "coverage": true, ".cache": true, "vendor": true, "target": true, "__pycache__": true, ".venv": true, "venv": true}

// writeTheMap writes the folder's map when there is none or the one there is
// generated. Nothing here fails the job.
func writeTheMap(folder string) {
	path := filepath.Join(folder, MapFile)
	if held, err := os.ReadFile(path); err == nil && !strings.Contains(string(held), GeneratedMark) {
		return
	}
	paths, err := theFilesOf(folder)
	if err != nil || len(paths) == 0 {
		return
	}
	files := make([]codemap.File, 0, len(paths))
	for _, relative := range paths {
		source, _ := os.ReadFile(filepath.Join(folder, relative))
		files = append(files, codemap.Read(relative, source))
	}
	_ = os.WriteFile(path, []byte(codemap.Print(filepath.Base(folder), files)), 0o644)
}

// theFilesOf walks the folder and returns every file's relative path, sorted,
// with the left-out folders skipped and the count bounded.
func theFilesOf(folder string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(folder, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			if path != folder && foldersLeftOut[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if len(files) >= MaxMapEntries {
			return filepath.SkipAll
		}
		relative, err := filepath.Rel(folder, path)
		if err != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	sort.Strings(files)
	return files, err
}
