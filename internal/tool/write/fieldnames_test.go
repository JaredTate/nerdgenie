package write_test

import (
	"os"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// codeFile puts a file holding a line of code in the folder the tool may write
// in, and returns its path and what it holds.
func codeFile(t *testing.T, root string) (string, string) {
	t.Helper()
	held := "func main() { work() }\n"
	path := root + "/main.go"
	if err := os.WriteFile(path, []byte(held), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the fixture file: %v", err)
	}
	return path, held
}

func TestAWriteWithNoContentIsRefusedAndTheFileIsLeftAsItWas(t *testing.T) {
	tool, root, _ := newTool(t)
	path, held := codeFile(t, root)

	output, err := run(t, tool, map[string]any{"path": path})
	if err == nil {
		t.Fatalf("a write with no content emptied the file and said %q", output.Text)
	}
	if !strings.Contains(err.Error(), "content") {
		t.Errorf("the refusal reads %q and does not name the field that is missing", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("cannot read the file back: %v", readErr)
	}
	if string(after) != held {
		t.Errorf("the file now holds %q, and a refused write must leave it as it was", after)
	}
}

func TestAWriteThatCallsTheContentSomethingElseStillWritesIt(t *testing.T) {
	names := []string{"contents", "text", "body", "file_text", "new_content"}
	for _, name := range names {
		tool, root, _ := newTool(t)
		path, _ := codeFile(t, root)

		if _, err := run(t, tool, map[string]any{"path": path, name: "the new text\n"}); err != nil {
			t.Errorf("a write with the content under %q was refused: %v", name, err)
			continue
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("cannot read the file back: %v", err)
		}
		if string(after) != "the new text\n" {
			t.Errorf("a write with the content under %q left %q in the file", name, after)
		}
	}
}

func TestAWriteThatEmptiesAFileOnPurposeIsStillAllowed(t *testing.T) {
	tool, root, _ := newTool(t)
	path, _ := codeFile(t, root)

	if _, err := run(t, tool, map[string]any{"path": path, "content": ""}); err != nil {
		t.Fatalf("a write of an empty content was refused: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the file back: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("the file holds %q, and a content written as an empty string means an empty file", after)
	}
}

func TestAWriteThatNamesThePathSomethingElseStillFindsIt(t *testing.T) {
	tool, root, _ := newTool(t)
	path, _ := codeFile(t, root)

	if _, err := run(t, tool, map[string]any{"file_path": path, "content": "the new text\n"}); err != nil {
		t.Fatalf("a write with the path under file_path was refused: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the file back: %v", err)
	}
	if string(after) != "the new text\n" {
		t.Errorf("the file holds %q after a write that named the path file_path", after)
	}
}
