package write_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool"
	"github.com/JaredTate/nerdgenie/internal/tool/write"
)

// newTool builds the write tool over a folder it may write in, and returns the
// tool, the folder, and the log behind it.
func newTool(t *testing.T) (*write.Tool, string, *testkit.FakeStore) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(root, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the folder the agent may work in: %v", err)
	}
	store := testkit.NewFakeStore()
	allowed := tool.MadeWhole(tool.NewPathCheck([]string{root}, filepath.Dir(root), ""), root, filepath.Dir(root))
	tool := write.New(write.Settings{
		Allowed: allowed,
		Log:     store,
		TaskID:  "17",
		Clock:   testkit.NewFakeClock(time.Unix(1700000000, 0).UTC()),
	})
	return tool, root, store
}

// run calls the tool with the fields written as JSON.
func run(t *testing.T, tool *write.Tool, fields map[string]any) (contract.ToolOutput, error) {
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
		if event.TaskID != "17" {
			t.Errorf("the file-change event belongs to task %q, want the task the tool was built for", event.TaskID)
		}
		bodies = append(bodies, body)
	}
	return bodies
}

func TestTheDescriptionFitsInTheCapAndTakesTheFixedFieldNames(t *testing.T) {
	tool, _, _ := newTool(t)
	spec := tool.Spec()

	if spec.Name != contract.ToolWrite {
		t.Errorf("the tool calls itself %q, want %q", spec.Name, contract.ToolWrite)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		t.Errorf("the description is %d words, and the cap is %d", words, contract.MaxToolDescriptionWords)
	}
	names := []string{}
	for _, field := range spec.Fields {
		names = append(names, field.Name)
	}
	if strings.Join(names, ",") != "path,content" {
		t.Errorf("the tool takes the fields %v, and the permission function reduces a write by path and content", names)
	}
}

func TestANewFileIsCreatedAndTheLogSaysItWasNotThereBefore(t *testing.T) {
	tool, root, store := newTool(t)
	path := filepath.Join(root, "notes", "today.md")

	output, err := run(t, tool, map[string]any{"path": path, "content": "the first line\n"})
	if err != nil {
		t.Fatalf("writing a new file failed: %v", err)
	}
	testkit.Golden(t, "a_new_file.txt", []byte(strings.ReplaceAll(output.Text, root, "/root")))

	held, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the file the tool said it wrote is not there: %v", err)
	}
	if string(held) != "the first line\n" {
		t.Errorf("the file holds %q, want what the tool was given", string(held))
	}

	changes := fileChanges(t, store)
	if len(changes) != 1 {
		t.Fatalf("the log holds %d file-change events, want one", len(changes))
	}
	if changes[0].Existed {
		t.Errorf("the log says the file was there before, and it was not")
	}
	if changes[0].Path != path {
		t.Errorf("the log names %q, want the file that was written", changes[0].Path)
	}
}

func TestOverwritingAFileSavesWhatItHeldBefore(t *testing.T) {
	tool, root, store := newTool(t)
	path := filepath.Join(root, "notes.md")
	if err := os.WriteFile(path, []byte("what it held before\n"), 0o640); err != nil {
		t.Fatalf("cannot write the file to overwrite: %v", err)
	}

	if _, err := run(t, tool, map[string]any{"path": path, "content": "what it holds now\n"}); err != nil {
		t.Fatalf("overwriting a file failed: %v", err)
	}

	changes := fileChanges(t, store)
	if len(changes) != 1 {
		t.Fatalf("the log holds %d file-change events, want one", len(changes))
	}
	if !changes[0].Existed {
		t.Errorf("the log says the file was not there before, and it was")
	}
	if string(changes[0].PriorContents) != "what it held before\n" {
		t.Errorf("the log kept %q, want what the file held before the write", string(changes[0].PriorContents))
	}
	if changes[0].Mode != 0o640 {
		t.Errorf("the log kept the mode %o, want the mode the file had", changes[0].Mode)
	}
	held, _ := os.ReadFile(path)
	if string(held) != "what it holds now\n" {
		t.Errorf("the file holds %q after the write", string(held))
	}
}

func TestAFileKeepsItsOwnModeWhenItIsOverwritten(t *testing.T) {
	tool, root, _ := newTool(t)
	path := filepath.Join(root, "script.sh")
	if err := os.WriteFile(path, []byte("old\n"), 0o755); err != nil {
		t.Fatalf("cannot write the file to overwrite: %v", err)
	}

	if _, err := run(t, tool, map[string]any{"path": path, "content": "new\n"}); err != nil {
		t.Fatalf("overwriting a file failed: %v", err)
	}
	about, err := os.Stat(path)
	if err != nil {
		t.Fatalf("cannot look at the file after the write: %v", err)
	}
	if about.Mode().Perm() != 0o755 {
		t.Errorf("the file's mode is now %o, want the %o it had", about.Mode().Perm(), 0o755)
	}
}

func TestAWriteOutsideTheRootsIsRefusedAndNothingIsLogged(t *testing.T) {
	tool, _, store := newTool(t)

	_, err := run(t, tool, map[string]any{"path": "/etc/passwd", "content": "no"})
	if err == nil {
		t.Fatalf("a file outside every root was written")
	}
	if len(fileChanges(t, store)) != 0 {
		t.Errorf("a refused write left a file-change event in the log")
	}
}

func TestNothingIsWrittenWhenTheLogRefusesTheChange(t *testing.T) {
	_, root, _ := newTool(t)
	path := filepath.Join(root, "unwritten.md")
	broken := write.New(write.Settings{
		Allowed: func(asked string) (string, error) { return asked, nil },
		Log:     refusingStore{},
		TaskID:  "17",
		Clock:   testkit.NewFakeClock(time.Unix(1700000000, 0).UTC()),
	})

	if _, err := run(t, broken, map[string]any{"path": path, "content": "no"}); err == nil {
		t.Fatalf("a write went ahead although the log refused to record what the file held")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the file was written although the log refused the change first")
	}
}

// refusingStore is a log that refuses every write, which is how a test proves
// nothing is written until the change is recorded.
type refusingStore struct{ *testkit.FakeStore }

// Append refuses, and says why.
func (refusingStore) Append(_ context.Context, _ contract.Event) (int64, error) {
	return 0, fmt.Errorf("this log is full, so nothing more can be recorded in it")
}

func TestBadInputIsRefusedWithALineTheModelCanActOn(t *testing.T) {
	tool, root, _ := newTool(t)

	if _, err := tool.Run(context.Background(), json.RawMessage("not json")); err == nil {
		t.Errorf("input that is not JSON was treated as a call")
	}
	if _, err := run(t, tool, map[string]any{"content": "no path"}); err == nil {
		t.Errorf("a call with no path was treated as a write")
	}
	if _, err := run(t, tool, map[string]any{"path": filepath.Join(root, "big.txt"), "content": strings.Repeat("x", write.MaxContentBytes+1)}); err == nil {
		t.Errorf("a file bigger than the cap was written")
	}
}
