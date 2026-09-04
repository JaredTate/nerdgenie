// Searching a tree for a pattern and returning "path:line: text" rows, and
// listing files by a name pattern, are OpenCode's grep and glob tools, at
// ~/Code/opencode/packages/opencode/src/tool/grep.ts and glob.ts; running the
// two from one call is Hermes' search_files, at
// ~/Code/hermes-agent/tools/file_tools.py. The Go here is written fresh.

package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/tool/loose"
)

// The bounds on one search. A search is a question, and a question with a
// thousand answers has not been asked properly.
const (
	// MaxRows is the most rows one search returns.
	MaxRows = 50
	// MaxPatternRunes is the longest pattern the tool will look for.
	MaxPatternRunes = 500
	// MaxFilesWalked is the most files one search looks at.
	MaxFilesWalked = 20000
	// MaxFileBytes is the biggest file whose lines are read.
	MaxFileBytes = 2 << 20
	// MaxRowRunes is how much of one matching line is shown.
	MaxRowRunes = 300
)

// Settings is what the search tool needs to do its work.
type Settings struct {
	// Allowed says whether a path may be searched.
	Allowed func(path string) (string, error)
	// Ripgrep is the path of the ripgrep program. Empty means look for it on the
	// PATH. When it is not on the machine, the tool searches on its own, more
	// slowly and with the same answers.
	Ripgrep string
	// DefaultFolder is where a search with no folder in it looks, which is the
	// first folder the agent may work in. A model that means everywhere writes a
	// pattern and nothing else.
	DefaultFolder string
}

// input is what the model writes when it calls this tool.
type input struct {
	// Pattern is the regular expression, or the name pattern when it is not one.
	Pattern string `json:"pattern"`
	// Path is the folder or file to search.
	Path string `json:"path"`
}

// Tool is the search tool.
type Tool struct {
	settings Settings
	ripgrep  string
}

// New returns the search tool, having looked for ripgrep once.
func New(settings Settings) *Tool {
	return &Tool{settings: settings, ripgrep: findRipgrep(settings.Ripgrep)}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolSearch,
		Description: "Finds the files whose names match a pattern and the lines that match it, under a folder. " +
			"Give a regular expression, or a name pattern such as *.md. Use read for a file you can already name.",
		Fields: []contract.ToolField{
			{Name: "pattern", Type: "string", Description: "A regular expression, or a name pattern such as *.md.", Required: true},
			{Name: "path", Type: "string", Description: "The path of the folder or file to search, taken from the folder the agent works in unless it starts at the root or at ~.", Required: true},
		},
		Classes: []contract.PermissionClass{contract.ClassRead},
	}
}

// Run searches for the pattern under the path and returns the rows it found.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked, err := readInput(written)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if tool.settings.Allowed == nil {
		return contract.ToolOutput{}, errors.New("this tool has no list of folders it may search, so wire the sandbox roots in before using it")
	}
	wanted, err := tool.folderToSearch(asked)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	path, err := tool.settings.Allowed(wanted)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if _, err := os.Stat(path); err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot search %s, so check the path and try again: %w", path, err)
	}

	expression, asName := readPattern(asked.Pattern)
	files, err := tool.filesUnder(ctx, path)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	rows := matchingNames(files, asked.Pattern, expression)
	if expression != nil {
		lines, err := tool.matchingLines(ctx, path, asked.Pattern, expression, files)
		if err != nil {
			return contract.ToolOutput{}, err
		}
		rows = append(rows, lines...)
	}
	return contract.ToolOutput{Text: rowsAsText(rows, asName)}, nil
}

// folderToSearch is where this call looks: the folder the model named, or the
// folder the agent works in when it named none, because a model that means
// everywhere writes a pattern and nothing else.
func (tool *Tool) folderToSearch(asked input) (string, error) {
	if strings.TrimSpace(asked.Path) != "" {
		return asked.Path, nil
	}
	if strings.TrimSpace(tool.settings.DefaultFolder) != "" {
		return tool.settings.DefaultFolder, nil
	}
	return "", errors.New("this call names no folder to search and this tool has no folder to fall back on, so give the path of the folder or the file")
}

// The names a model writes for the two fields this tool needs. The first of each
// is the one the specification asks for, and the rest are the names the other
// agents use or a model half-remembers.
var (
	patternNames = []string{"pattern", "query", "regex", "regexp", "text", "search", "glob"}
	pathNames    = []string{"path", "folder", "directory", "dir", "file_path", "root", "in"}
)

// readInput reads the model's arguments and refuses anything this tool could not
// act on.
func readInput(written json.RawMessage) (input, error) {
	fields, err := loose.Read(written, "a pattern and a path")
	if err != nil {
		return input{}, err
	}
	pattern, wrotePattern := fields.Text(patternNames...)
	path, _ := fields.Text(pathNames...)
	if err := fields.Wrong(); err != nil {
		return input{}, err
	}
	if !wrotePattern {
		return input{}, fields.Missing("pattern", "a regular expression, or a name pattern such as *.md,")
	}
	if strings.TrimSpace(pattern) == "" {
		return input{}, errors.New("this call has nothing to look for, so give a regular expression or a name pattern such as *.md")
	}
	if len([]rune(pattern)) > MaxPatternRunes {
		return input{}, fmt.Errorf("the pattern is %d characters and the cap is %d, so look for something shorter",
			len([]rune(pattern)), MaxPatternRunes)
	}
	return input{Pattern: pattern, Path: path}, nil
}

// readPattern compiles the pattern as a regular expression, and says so when it
// is not one, in which case only the names of files are looked at.
func readPattern(pattern string) (*regexp.Regexp, bool) {
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return nil, true
	}
	return expression, false
}

// matchingNames returns one row per file whose name matches, by the regular
// expression when the pattern is one and by the name pattern when it is not.
func matchingNames(files []string, pattern string, expression *regexp.Regexp) []string {
	rows := []string{}
	for _, path := range files {
		if nameMatches(path, pattern, expression) {
			rows = append(rows, path)
		}
	}
	sort.Strings(rows)
	return rows
}

// rowsAsText is the text the model reads back: the rows, in order, with a line
// saying how the pattern was read and a line saying when the rows were cut.
func rowsAsText(rows []string, asName bool) string {
	written := &strings.Builder{}
	if asName {
		written.WriteString("this pattern is not a regular expression, so it was read as a name pattern\n")
	}
	if len(rows) == 0 {
		written.WriteString("nothing matched\n")
		return written.String()
	}
	for at, row := range rows {
		if at >= MaxRows {
			fmt.Fprintf(written, "... more than %d rows matched; narrow the pattern or the path\n", MaxRows)
			break
		}
		written.WriteString(row + "\n")
	}
	return written.String()
}
