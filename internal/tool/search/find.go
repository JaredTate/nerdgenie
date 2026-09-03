package search

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"unicode/utf8"
)

// ripgrepProgram is the name looked for on the PATH when the settings name no
// path of their own.
const ripgrepProgram = "rg"

// findRipgrep returns the path of ripgrep to use, and an empty string when there
// is none on the machine, which is what makes the tool search on its own.
func findRipgrep(asked string) string {
	if asked != "" {
		if _, err := os.Stat(asked); err != nil {
			return ""
		}
		return asked
	}
	found, err := exec.LookPath(ripgrepProgram)
	if err != nil {
		return ""
	}
	return found
}

// filesUnder lists the files under a path, through ripgrep when it is on the
// machine and by walking the folder when it is not.
func (tool *Tool) filesUnder(ctx context.Context, path string) ([]string, error) {
	if about, err := os.Stat(path); err == nil && !about.IsDir() {
		return []string{path}, nil
	}
	if tool.ripgrep != "" {
		return tool.filesThroughRipgrep(ctx, path)
	}
	return filesByWalking(path)
}

// filesThroughRipgrep asks ripgrep for the files it would search, which is the
// same list this package would walk to and a great deal faster to come by.
func (tool *Tool) filesThroughRipgrep(ctx context.Context, path string) ([]string, error) {
	written, err := runQuietly(ctx, tool.ripgrep, "--files", "--", path)
	if err != nil {
		return nil, err
	}
	return sortedLines(written, path), nil
}

// filesByWalking walks a folder for the files under it, skipping the folders
// nothing in a search belongs in and stopping at the cap.
func filesByWalking(path string) ([]string, error) {
	found := []string{}
	err := filepath.WalkDir(path, func(at string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if skippedFolder(entry.Name()) && at != path {
				return filepath.SkipDir
			}
			return nil
		}
		if len(found) >= MaxFilesWalked {
			return filepath.SkipAll
		}
		found = append(found, at)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("cannot walk %s to search it: %w", path, err)
	}
	sort.Strings(found)
	return found, nil
}

// skippedFolder says whether a folder holds anything worth searching.
func skippedFolder(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	return name == "node_modules" || name == "vendor" || name == "bin" || name == "dist"
}

// matchingLines returns one row per matching line, through ripgrep when it is on
// the machine and by reading the files when it is not.
func (tool *Tool) matchingLines(ctx context.Context, path string, pattern string, expression *regexp.Regexp, files []string) ([]string, error) {
	if tool.ripgrep != "" {
		return tool.linesThroughRipgrep(ctx, pattern, path)
	}
	return linesByReading(expression, files), nil
}

// linesThroughRipgrep asks ripgrep for the matching lines. Ripgrep reports no
// match by quitting with a code of one, which is not a failure of the search.
func (tool *Tool) linesThroughRipgrep(ctx context.Context, pattern string, path string) ([]string, error) {
	written, err := runQuietly(ctx, tool.ripgrep,
		"--line-number", "--no-heading", "--color", "never", "--max-count", fmt.Sprint(MaxRows), "-e", pattern, "--", path)
	if err != nil {
		return nil, err
	}
	rows := []string{}
	for _, line := range sortedLines(written, "") {
		rows = append(rows, tidyRow(line))
	}
	sort.Strings(rows)
	return rows, nil
}

// linesByReading reads every file and keeps the lines the expression matches,
// which is the slow half of the tool and gives the same answers.
func linesByReading(expression *regexp.Regexp, files []string) []string {
	rows := []string{}
	for _, path := range files {
		if len(rows) > MaxRows {
			break
		}
		rows = append(rows, matchesInFile(expression, path)...)
	}
	sort.Strings(rows)
	return rows
}

// matchesInFile returns the matching lines of one file, and nothing at all for a
// file too big to read or one that is not text.
func matchesInFile(expression *regexp.Regexp, path string) []string {
	about, err := os.Stat(path)
	if err != nil || about.Size() > MaxFileBytes {
		return nil
	}
	held, err := os.ReadFile(path)
	if err != nil || bytes.IndexByte(held, 0) >= 0 || !utf8.Valid(held) {
		return nil
	}

	rows := []string{}
	scanner := bufio.NewScanner(bytes.NewReader(held))
	scanner.Buffer(make([]byte, 0, 64<<10), MaxFileBytes)
	for number := 1; scanner.Scan() && len(rows) <= MaxRows; number++ {
		if expression.MatchString(scanner.Text()) {
			rows = append(rows, tidyRow(fmt.Sprintf("%s:%d:%s", path, number, scanner.Text())))
		}
	}
	return rows
}

// nameMatches says whether a file's name matches, by the regular expression when
// the pattern is one and by the name pattern when it is not.
func nameMatches(path string, pattern string, expression *regexp.Regexp) bool {
	name := filepath.Base(path)
	if expression == nil {
		matched, err := filepath.Match(pattern, name)
		return err == nil && matched
	}
	return expression.MatchString(name)
}

// tidyRow puts one row on a single line and cuts it, so that a file written as
// one enormous line cannot fill the result on its own.
func tidyRow(row string) string {
	flattened := strings.ReplaceAll(strings.ReplaceAll(row, "\r", " "), "\t", " ")
	letters := []rune(flattened)
	if len(letters) <= MaxRowRunes {
		return flattened
	}
	return string(letters[:MaxRowRunes]) + " ..."
}

// sortedLines splits what a program wrote into lines, drops the empty ones, and
// puts them in order. The path is dropped when it is one of the lines, because
// ripgrep lists the folder itself when it is handed one file.
func sortedLines(written []byte, path string) []string {
	found := []string{}
	for _, line := range strings.Split(string(written), "\n") {
		if line = strings.TrimRight(line, "\r"); line != "" && line != path {
			found = append(found, line)
		}
	}
	sort.Strings(found)
	return found
}

// runQuietly runs a program in its own process group and returns what it wrote,
// treating "nothing matched", which ripgrep reports by quitting with a one, as
// an answer rather than as a failure.
func runQuietly(ctx context.Context, program string, arguments ...string) ([]byte, error) {
	running := exec.CommandContext(ctx, program, arguments...)
	running.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	written := &bytes.Buffer{}
	complaint := &bytes.Buffer{}
	running.Stdout = written
	running.Stderr = complaint

	err := running.Run()
	if err == nil {
		return written.Bytes(), nil
	}
	if quit, isQuit := err.(*exec.ExitError); isQuit && quit.ExitCode() == 1 {
		return written.Bytes(), nil
	}
	return nil, fmt.Errorf("the search program %s did not finish, and it said %q: %w",
		program, strings.TrimSpace(complaint.String()), err)
}
