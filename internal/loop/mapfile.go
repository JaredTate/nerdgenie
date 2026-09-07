package loop

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/codemap"
	"github.com/JaredTate/nerdgenie/internal/markdown"
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
	// MaxFilesForAMapRewrite is the biggest folder whose whole map is written
	// again in the middle of a task, when a file the map has no entry for is
	// written; a bigger folder's map waits for the task's end.
	MaxFilesForAMapRewrite = 200
)

// refreshTheMapEntry puts one file's fresh entry into the folder's generated
// map, or writes the whole map again when the file has no entry yet and the
// folder is small enough. A hand-written map, and a folder with no map, are
// left alone.
func refreshTheMapEntry(folder string, relative string) {
	path := filepath.Join(folder, MapFile)
	held, err := os.ReadFile(path)
	if err != nil || !codemap.IsGenerated(string(held)) {
		return
	}
	if foldersLeftOut[strings.SplitN(relative, "/", 2)[0]] {
		return
	}
	source, err := os.ReadFile(filepath.Join(folder, relative))
	if err != nil {
		return
	}
	if _, found := markdown.Find(string(held), relative); !found {
		if paths, err := theFilesOf(folder); err == nil && len(paths) <= MaxFilesForAMapRewrite {
			writeTheMap(folder)
		}
		return
	}
	written, _ := markdown.ReplaceSection(string(held), relative, codemap.EntryBody(codemap.Read(relative, source)))
	_ = os.WriteFile(path, []byte(written), 0o644)
}

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
