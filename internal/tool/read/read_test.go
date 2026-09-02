package read_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/tool/read"
)

// storedText answers with the text a test put under each label, standing in for
// the record keeper that keeps every result in the event log.
type storedText map[string]string

// Read brings back the text under one label, or says there is none.
func (stored storedText) Read(_ context.Context, id string) (string, error) {
	text, held := stored[id]
	if !held {
		return "", fmt.Errorf("no result with the label %q was written by this record, so check the result list", id)
	}
	return text, nil
}

// newTool builds the read tool over a folder it may read, and returns the tool
// and the folder.
func newTool(t *testing.T, results storedText, reports storedText) (*read.Tool, string) {
	t.Helper()
	root := t.TempDir()
	allowed := func(path string) (string, error) {
		if !strings.HasPrefix(filepath.Clean(path), root) {
			return "", fmt.Errorf("the path %s is outside the folder the agent may work in, which is %s", path, root)
		}
		return filepath.Clean(path), nil
	}
	return read.New(read.Settings{Allowed: allowed, Results: results, Reports: reports}), root
}

// run calls the tool with the fields written as JSON.
func run(t *testing.T, tool *read.Tool, fields map[string]any) (contract.ToolOutput, error) {
	t.Helper()
	written, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("cannot write the tool's input as JSON: %v", err)
	}
	return tool.Run(context.Background(), written)
}

func TestTheDescriptionFitsInTheCapAndSaysWhenNotToUseTheTool(t *testing.T) {
	tool, _ := newTool(t, nil, nil)
	spec := tool.Spec()

	if spec.Name != contract.ToolRead {
		t.Errorf("the tool calls itself %q, want %q", spec.Name, contract.ToolRead)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		t.Errorf("the description is %d words, and the cap is %d", words, contract.MaxToolDescriptionWords)
	}
	for _, field := range []string{"path", "offset", "limit"} {
		if !hasField(spec, field) {
			t.Errorf("the tool has no field named %q, and the permission function reduces calls by that name", field)
		}
	}
}

// hasField says whether the specification names an input field.
func hasField(spec contract.ToolSpec, name string) bool {
	for _, field := range spec.Fields {
		if field.Name == name {
			return true
		}
	}
	return false
}

func TestAFileComesBackWithItsLineNumbers(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	path := filepath.Join(root, "notes.md")
	if err := os.WriteFile(path, []byte("first line\nsecond line\nthird line\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the fixture file: %v", err)
	}

	output, err := run(t, tool, map[string]any{"path": path})
	if err != nil {
		t.Fatalf("reading a file failed: %v", err)
	}
	testkit.Golden(t, "a_file.txt", []byte(output.Text))
}

func TestAnOffsetAndALimitReadOneWindowOfAFile(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	path := filepath.Join(root, "long.txt")
	lines := make([]string, 0, 20)
	for at := 1; at <= 20; at++ {
		lines = append(lines, fmt.Sprintf("line %d", at))
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the fixture file: %v", err)
	}

	output, err := run(t, tool, map[string]any{"path": path, "offset": 5, "limit": 3})
	if err != nil {
		t.Fatalf("reading a window of a file failed: %v", err)
	}
	if output.Text != "5: line 5\n6: line 6\n7: line 7\n" {
		t.Errorf("the window reads %q, want lines five to seven with their numbers", output.Text)
	}
}

func TestAnOffsetPastTheEndSaysHowLongTheFileIs(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	path := filepath.Join(root, "short.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the fixture file: %v", err)
	}

	_, err := run(t, tool, map[string]any{"path": path, "offset": 40})
	if err == nil {
		t.Fatalf("an offset past the end of the file was treated as a read")
	}
	if !strings.Contains(err.Error(), "2") {
		t.Errorf("the refusal reads %q and does not say how many lines the file has", err)
	}
}

func TestAFolderComesBackAsAListing(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	if err := os.MkdirAll(filepath.Join(root, "drafts"), contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the folder inside the root: %v", err)
	}
	for _, name := range []string{"one.md", "two.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x\n"), contract.DataFileMode); err != nil {
			t.Fatalf("cannot write %s: %v", name, err)
		}
	}

	output, err := run(t, tool, map[string]any{"path": root})
	if err != nil {
		t.Fatalf("reading a folder failed: %v", err)
	}
	testkit.Golden(t, "a_folder.txt", []byte(output.Text))
}

func TestABinaryFileIsRefusedWithANote(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	path := filepath.Join(root, "picture.png")
	if err := os.WriteFile(path, []byte{0x89, 'P', 'N', 'G', 0x00, 0x1a, 0x0a}, contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the fixture file: %v", err)
	}

	_, err := run(t, tool, map[string]any{"path": path})
	if err == nil {
		t.Fatalf("a binary file was read as text")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the refusal reads %q and does not name the file", err)
	}
}

func TestAPastResultComesBackWholeByItsLabel(t *testing.T) {
	tool, _ := newTool(t, storedText{"r7": "the whole of the seventh result"}, storedText{"j4.2": "the report of the second task of job four"})

	result, err := run(t, tool, map[string]any{"path": "r7"})
	if err != nil {
		t.Fatalf("reading a past result failed: %v", err)
	}
	if result.Text != "the whole of the seventh result" {
		t.Errorf("the result reads %q, want the whole of what was stored", result.Text)
	}

	report, err := run(t, tool, map[string]any{"path": "j4.2"})
	if err != nil {
		t.Fatalf("reading a job report failed: %v", err)
	}
	if report.Text != "the report of the second task of job four" {
		t.Errorf("the report reads %q, want the whole of what was stored", report.Text)
	}
}

func TestALabelWithNothingBehindItSaysSo(t *testing.T) {
	tool, _ := newTool(t, storedText{}, storedText{})

	if _, err := run(t, tool, map[string]any{"path": "r7"}); err == nil {
		t.Errorf("a label with no result behind it was treated as a read")
	}
}

func TestALabelWithNoRecordWiredInSaysSo(t *testing.T) {
	tool, _ := newTool(t, nil, nil)

	_, err := run(t, tool, map[string]any{"path": "r7"})
	if err == nil {
		t.Fatalf("a past result was read with no record behind the tool")
	}
	if !strings.Contains(err.Error(), "r7") {
		t.Errorf("the refusal reads %q and does not name the label asked for", err)
	}
}

func TestAPathOutsideTheRootsIsRefused(t *testing.T) {
	tool, _ := newTool(t, nil, nil)

	if _, err := run(t, tool, map[string]any{"path": "/etc/passwd"}); err == nil {
		t.Errorf("a file outside every root was read")
	}
}

func TestBadInputIsRefusedWithALineTheModelCanActOn(t *testing.T) {
	tool, _ := newTool(t, nil, nil)

	if _, err := tool.Run(context.Background(), json.RawMessage(`not json at all`)); err == nil {
		t.Errorf("input that is not JSON was treated as a call")
	}
	if _, err := run(t, tool, map[string]any{}); err == nil {
		t.Errorf("a call with no path was treated as a read")
	}
	if _, err := run(t, tool, map[string]any{"path": "/x", "limit": -3}); err == nil {
		t.Errorf("a limit below zero was treated as a read")
	}
}
