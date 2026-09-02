package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// The header and the roots table the map always carries. They change only when
// the shape of the repository changes, which is why they live here rather than
// in the file the generator overwrites.
const mapHeader = `# Repository Map

<!-- generated: scripts/repomap -->
<!-- repo-map-contract: v1 -->

This file is generated from the repository tree. Regenerate it with ` + "`make repo-map`" + `.
The generator omits dependency folders, build outputs, test artifacts, and version-control internals.

## Roots

- ` + "`./`" + ` - The three living documents, the plain-words explanation COEUS.md, the license, the third-party notes, the Makefile.
- ` + "`cmd/coeus/`" + ` - The one binary and its subcommands.
- ` + "`internal/`" + ` - The Go packages, one job each; see ARCHITECTURE.md for the list.
- ` + "`worker/`" + ` - The TypeScript browser and desktop workers.
- ` + "`test/`" + ` - Functional tests and fixtures.
- ` + "`scripts/`" + ` - The installer, the repo-map generator, CI helpers.
- ` + "`docs/`" + ` - The design, the work plan, the comparison, the research, the references, the briefs.

## Tree

`

// The folders that hold nothing a fresh clone would carry: dependencies, build
// outputs, test artifacts, and version-control internals.
var excludedFolders = []string{".git", "node_modules", "bin", "dist", "coverage"}

// The file endings that mark something written by a build rather than by a
// person.
var excludedEndings = []string{".log", ".tsbuildinfo"}

// generate returns the whole of REPO_MAP.md for the repository rooted at root.
func generate(root string) (string, error) {
	if _, err := os.Stat(root); err != nil {
		return "", fmt.Errorf("cannot map the folder %s, because it is not there: %w", root, err)
	}

	paths, err := listedFiles(root)
	if err != nil {
		return "", err
	}
	slices.Sort(paths)

	var built strings.Builder
	built.WriteString(mapHeader)
	built.WriteString("```text\n.\n")
	for _, path := range paths {
		built.WriteString(path)
		built.WriteString("\n")
	}
	built.WriteString("```\n")
	return built.String(), nil
}

// listedFiles returns the paths the map lists, with the excluded ones taken out.
// Inside a git work tree that is the tracked files, which is exactly what a
// fresh clone holds; outside one it is every file in the tree, which is what the
// fixture trees in the tests are.
func listedFiles(root string) ([]string, error) {
	paths, err := trackedFiles(root)
	if err != nil {
		paths, err = walkedFiles(root)
		if err != nil {
			return nil, err
		}
	}

	kept := []string{}
	for _, path := range paths {
		if !excludedPath(path) {
			kept = append(kept, path)
		}
	}
	return kept, nil
}

// trackedFiles asks git which files it is tracking, and fails when the folder is
// not a git work tree.
//
// The child runs with every GIT_ variable taken out of its environment. Git
// exports GIT_DIR, GIT_INDEX_FILE, and GIT_WORK_TREE to the programs it runs,
// and GIT_DIR outranks the -C flag, so a generator that inherited them would
// list some other repository's files and say nothing about it. HomeRecon wrote
// the same warning at the top of
// ~/Code/homerecon/scripts/repo-map/repo-map.test.cjs.
func trackedFiles(root string) ([]string, error) {
	command := exec.Command("git", "-C", root, "ls-files", "-z")
	command.Env = environmentWithoutGitVariables()
	command.Stderr = nil
	listing, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("cannot ask git which files %s tracks: %w", root, err)
	}
	paths := []string{}
	for _, path := range strings.Split(string(listing), "\x00") {
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

// environmentWithoutGitVariables is this program's environment with every GIT_
// setting removed, so that a git child obeys the folder it was given and nothing
// else.
func environmentWithoutGitVariables() []string {
	kept := []string{}
	for _, setting := range os.Environ() {
		if strings.HasPrefix(setting, "GIT_") {
			continue
		}
		kept = append(kept, setting)
	}
	return kept
}

// readCapped reads everything the reader has, up to the cap, and refuses to go
// past it rather than holding an unbounded amount in memory.
func readCapped(reader io.Reader, cap int) ([]byte, error) {
	read, err := io.ReadAll(io.LimitReader(reader, int64(cap)+1))
	if err != nil {
		return nil, fmt.Errorf("cannot read the answer: %w", err)
	}
	if len(read) > cap {
		return nil, fmt.Errorf("the answer is longer than the %d byte cap, so something is wrong with the folder being mapped", cap)
	}
	return read, nil
}

// walkedFiles lists every file under the root, for a tree that is not a git work
// tree.
func walkedFiles(root string) ([]string, error) {
	paths := []string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, relativeErr := filepath.Rel(root, path)
		if relativeErr != nil {
			return relativeErr
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if relative != "." && excludedPath(relative) {
				return filepath.SkipDir
			}
			return nil
		}
		paths = append(paths, relative)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("cannot walk the folder %s to map it: %w", root, err)
	}
	return paths, nil
}

// excludedPath says whether a path is one the map leaves out, either because a
// part of it names an excluded folder or because the file was written by a
// build.
func excludedPath(path string) bool {
	for _, part := range strings.Split(path, "/") {
		if slices.Contains(excludedFolders, part) {
			return true
		}
	}
	for _, ending := range excludedEndings {
		if strings.HasSuffix(path, ending) {
			return true
		}
	}
	return false
}
