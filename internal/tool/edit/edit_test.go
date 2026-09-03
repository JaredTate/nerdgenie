package edit_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/tool"
	"github.com/JaredTate/coeus/internal/tool/edit"
)

// newTool builds the edit tool over a folder it may work in, and returns the
// tool, the folder, and the log behind it.
func newTool(t *testing.T) (*edit.Tool, string, *testkit.FakeStore) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(root, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the folder the agent may work in: %v", err)
	}
	store := testkit.NewFakeStore()
	allowed := tool.NewPathCheck([]string{root}, filepath.Dir(root), "")
	tool := edit.New(edit.Settings{
		Allowed: allowed,
		Log:     store,
		TaskID:  "17",
		Clock:   testkit.NewFakeClock(time.Unix(1700000000, 0).UTC()),
	})
	return tool, root, store
}

// run calls the tool with the fields written as JSON.
func run(t *testing.T, tool *edit.Tool, fields map[string]any) (contract.ToolOutput, error) {
	t.Helper()
	written, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("cannot write the tool's input as JSON: %v", err)
	}
	return tool.Run(context.Background(), written)
}

// fileChanges returns every file-change event the log holds, in order.
func fileChanges(t *testing.T, store *testkit.FakeStore) []contract.FileChangeBody {
	t.Helper()
	events, err := store.ByKind(context.Background(), contract.EventFileChange)
	if err != nil {
		t.Fatalf("cannot read the file-change events out of the log: %v", err)
	}
	bodies := make([]contract.FileChangeBody, 0, len(events))
	for _, event := range events {
		body := contract.FileChangeBody{}
		if err := json.Unmarshal(event.Body, &body); err != nil {
			t.Fatalf("cannot read a file-change event's body: %v", err)
		}
		bodies = append(bodies, body)
	}
	return bodies
}

func TestTheDescriptionFitsInTheCapAndTakesTheFixedFieldNames(t *testing.T) {
	tool, _, _ := newTool(t)
	spec := tool.Spec()

	if spec.Name != contract.ToolEdit {
		t.Errorf("the tool calls itself %q, want %q", spec.Name, contract.ToolEdit)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		t.Errorf("the description is %d words, and the cap is %d", words, contract.MaxToolDescriptionWords)
	}
	names := []string{}
	for _, field := range spec.Fields {
		names = append(names, field.Name)
	}
	if strings.Join(names, ",") != "path,old,new" {
		t.Errorf("the tool takes the fields %v, and the permission function reduces an edit by path, old, and new", names)
	}
}

func TestOneSpanIsReplacedAndWhatTheFileHeldIsInTheLog(t *testing.T) {
	tool, root, store := newTool(t)
	path := filepath.Join(root, "code.go")
	before := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	if err := os.WriteFile(path, []byte(before), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file to edit: %v", err)
	}

	output, err := run(t, tool, map[string]any{"path": path, "old": "println(\"hello\")", "new": "println(\"goodbye\")"})
	if err != nil {
		t.Fatalf("editing a file failed: %v", err)
	}
	testkit.Golden(t, "one_span.txt", []byte(strings.ReplaceAll(output.Text, root, "/root")))

	held, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the file after the edit: %v", err)
	}
	if !strings.Contains(string(held), "goodbye") || strings.Contains(string(held), "hello") {
		t.Errorf("the file reads %q after the edit", string(held))
	}

	changes := fileChanges(t, store)
	if len(changes) != 1 {
		t.Fatalf("the log holds %d file-change events, want one", len(changes))
	}
	if string(changes[0].PriorContents) != before {
		t.Errorf("the log kept %q, want what the file held before the edit", string(changes[0].PriorContents))
	}
}

func TestAnEditOutsideTheRootsIsRefusedAndNothingIsLogged(t *testing.T) {
	tool, _, store := newTool(t)

	if _, err := run(t, tool, map[string]any{"path": "/etc/passwd", "old": "root", "new": "nobody"}); err == nil {
		t.Fatalf("a file outside every root was edited")
	}
	if len(fileChanges(t, store)) != 0 {
		t.Errorf("a refused edit left a file-change event in the log")
	}
}

func TestAnEditThatFindsNothingLeavesTheFileAndTheLogAlone(t *testing.T) {
	tool, root, store := newTool(t)
	path := filepath.Join(root, "code.go")
	before := "package main\n"
	if err := os.WriteFile(path, []byte(before), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file to edit: %v", err)
	}

	if _, err := run(t, tool, map[string]any{"path": path, "old": "nothing like this", "new": "x"}); err == nil {
		t.Fatalf("an edit that found nothing was treated as a change")
	}
	held, _ := os.ReadFile(path)
	if string(held) != before {
		t.Errorf("the file reads %q after a refused edit", string(held))
	}
	if len(fileChanges(t, store)) != 0 {
		t.Errorf("a refused edit left a file-change event in the log")
	}
}

func TestAFileThatIsNotThereIsRefused(t *testing.T) {
	tool, root, _ := newTool(t)

	if _, err := run(t, tool, map[string]any{"path": filepath.Join(root, "gone.go"), "old": "a", "new": "b"}); err == nil {
		t.Errorf("an edit was made to a file that is not there")
	}
}

func TestBadInputIsRefusedWithALineTheModelCanActOn(t *testing.T) {
	tool, _, _ := newTool(t)

	if _, err := tool.Run(context.Background(), json.RawMessage("not json")); err == nil {
		t.Errorf("input that is not JSON was treated as a call")
	}
	if _, err := run(t, tool, map[string]any{"old": "a", "new": "b"}); err == nil {
		t.Errorf("a call with no path was treated as an edit")
	}
}
