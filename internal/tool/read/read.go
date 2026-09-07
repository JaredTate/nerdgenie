// Reading a file back with one line number in front of each line, so that the
// edit tool and the model are talking about the same lines, is OpenCode's, at
// ~/Code/opencode/packages/opencode/src/tool/read.ts and read.txt. The Go here
// is written fresh, and the past-result label is Nerd Genie's own.

package read

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/tool/loose"
)

// The bounds on one read. A model that asks for a whole file gets the beginning
// of it and a line saying how to ask for the rest, because a result that fills
// the window leaves no room to think.
const (
	// MaxLines is the most lines one read returns: four hundred is about sixteen
	// kilobytes of ordinary code, the byte cap's worth, so the two caps meet on
	// a real file and a longer read is paged with an offset.
	MaxLines = 400
	// MaxBytes is the most bytes one read returns. Sixteen kilobytes is about
	// four thousand tokens, eight seconds of the local daemon's prompt
	// processing at five hundred tokens a second: on the fifth game build the
	// model read its whole thirty-thousand-character script twenty times to
	// find one function each time, fifteen seconds of every such round, and
	// the rest is read on with an offset when it is wanted.
	MaxBytes = 16 << 10
	// MaxLineRunes is how much of one very long line is shown.
	MaxLineRunes = 2000
	// MaxEntries is the most entries one folder listing shows.
	MaxEntries = 1000
	// binarySampleBytes is how much of a file is looked at to decide whether it
	// is text.
	binarySampleBytes = 8000
)

// Stored is where the whole text of a past result or a finished task's report is
// kept, which is what the record keeper does through the event log.
type Stored interface {
	// Read brings back the whole text of one result by its label.
	Read(ctx context.Context, id string) (string, error)
}

// Settings is what the read tool needs to do its work.
type Settings struct {
	// Allowed says whether a path may be read.
	Allowed func(path string) (string, error)
	// Results is the record of the task running now, which is where a label such
	// as r7 is read from.
	Results Stored
	// Reports is the record of the job the task belongs to, which is where a
	// label such as j4.2 is read from.
	Reports Stored
}

// input is what the model writes when it calls this tool.
type input struct {
	// Path is the file, the folder, or the label of a past result.
	Path string `json:"path"`
	// Offset is the line to start at, counting from one.
	Offset int `json:"offset"`
	// Limit is how many lines to read.
	Limit int `json:"limit"`
	// Section is the heading of the one section of a Markdown file to read.
	Section string `json:"section"`
}

// Tool is the read tool.
type Tool struct {
	settings Settings
}

// New returns the read tool.
func New(settings Settings) *Tool {
	return &Tool{settings: settings}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolRead,
		Description: "Reads a file with line numbers, a folder, a past result such as r7 or j4.2, or the ask. " +
			"A Markdown file and a section heading return that section only. " +
			"Use search when you do not know which file.",
		Fields: []contract.ToolField{
			{Name: "path", Type: "string", Description: "A file or folder, taken from the folder the agent works in unless it starts at the root or at ~; " +
				"or a past result by its id such as r7; or ask for the whole of the user's original ask.", Required: true},
			{Name: "offset", Type: "integer", Description: "The line to start at, counting from one. Leave it out for the start."},
			{Name: "limit", Type: "integer", Description: "How many lines to read. Leave it out for as many as fit."},
			{Name: "section", Type: "string", Description: "For a Markdown file, the heading of the one section to read, such as hazards."},
		},
		Classes: []contract.PermissionClass{contract.ClassRead},
	}
}

// Run reads whatever the path names: a past result, a folder, or a file.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked, err := readInput(written)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if label, kind, isLabel := resultLabel(asked.Path); isLabel {
		text, err := tool.readStored(ctx, label, kind)
		if err != nil {
			return contract.ToolOutput{}, err
		}
		if asked.Offset < 1 && asked.Limit < 1 {
			return contract.ToolOutput{Text: text}, nil
		}
		from, count, wanted := window(asked)
		text, err = numbered(bufio.NewReader(strings.NewReader(text)), label, from, count, wanted)
		return contract.ToolOutput{Text: text}, err
	}
	if tool.settings.Allowed == nil {
		return contract.ToolOutput{}, errors.New("this tool has no list of folders it may read, so wire the sandbox roots in before using it")
	}
	path, err := tool.settings.Allowed(asked.Path)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	about, err := os.Stat(path)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot open %s, so check the path and try again: %w", path, err)
	}
	if about.IsDir() {
		text, err := listFolder(path)
		return contract.ToolOutput{Text: text}, err
	}
	if isAPicture(path) {
		return readPicture(path)
	}
	if asked.Section != "" {
		text, err := readSection(path, asked)
		return contract.ToolOutput{Text: text}, err
	}
	text, err := readFile(path, asked)
	return contract.ToolOutput{Text: text}, err
}

// The names a model writes for the three fields this tool takes. The first of
// each is the one the specification asks for, and the rest are the names the
// other agents use or a model half-remembers.
var (
	pathNames    = []string{"path", "file_path", "filepath", "file", "filename", "target"}
	offsetNames  = []string{"offset", "start", "start_line", "from", "from_line"}
	limitNames   = []string{"limit", "count", "lines", "line_count", "max_lines"}
	sectionNames = []string{"section", "heading", "part"}
)

// readInput reads the model's arguments and refuses anything this tool could not
// act on.
func readInput(written json.RawMessage) (input, error) {
	fields, err := loose.Read(written, "a path")
	if err != nil {
		return input{}, err
	}
	path, wrotePath := fields.Text(pathNames...)
	offset, _ := fields.Number(offsetNames...)
	limit, _ := fields.Number(limitNames...)
	section, _ := fields.Text(sectionNames...)
	if err := fields.Wrong(); err != nil {
		return input{}, err
	}
	if !wrotePath {
		return input{}, fields.Missing("path", "the path of a file or folder, a result label such as r7, or ask for the whole of the ask,")
	}
	if strings.TrimSpace(path) == "" {
		return input{}, errors.New("this call names nothing to read, so give a path or a result label such as r7")
	}
	if offset < 0 || limit < 0 {
		return input{}, fmt.Errorf("the offset is %d and the limit is %d, and neither may be below zero", offset, limit)
	}
	return input{Path: path, Offset: offset, Limit: limit, Section: strings.TrimSpace(section)}, nil
}

// resultLabel says whether what the model wrote is the label of a past result
// rather than a path, and which record it belongs to. The label "ask" is one of
// them: the record shows the model the start of a very long ask and a line
// saying to read this label for the whole of it, and the record answers it.
func resultLabel(path string) (string, contract.RecordKind, bool) {
	if path == record.AskLabel {
		return path, contract.RecordTask, true
	}
	if _, isResult := contract.ParseResultID(path); isResult {
		return path, contract.RecordTask, true
	}
	if _, _, isReport := contract.ParseReportID(path); isReport {
		return path, contract.RecordJob, true
	}
	return "", "", false
}

// readStored brings back the whole text of a past result or a job's report.
func (tool *Tool) readStored(ctx context.Context, label string, kind contract.RecordKind) (string, error) {
	stored := tool.settings.Results
	if kind == contract.RecordJob {
		stored = tool.settings.Reports
	}
	if stored == nil {
		return "", fmt.Errorf("there is no %s record behind this tool to read %s from, so read a file instead", kind, label)
	}
	return stored.Read(ctx, label)
}
