package read_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// tenLines puts a file of ten numbered lines in the folder the tool may read,
// and returns its path.
func tenLines(t *testing.T, root string) string {
	t.Helper()
	lines := ""
	for at := 1; at <= 10; at++ {
		lines += "line " + string(rune('0'+at%10)) + "\n"
	}
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte(lines), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the fixture file: %v", err)
	}
	return path
}

func TestAReadThatNamesThePathSomethingElseStillFindsIt(t *testing.T) {
	names := []string{"file_path", "filepath", "file", "filename"}
	for _, name := range names {
		tool, root := newTool(t, nil, nil)
		path := tenLines(t, root)

		output, err := run(t, tool, map[string]any{name: path})
		if err != nil {
			t.Errorf("a read with the path under %q was refused: %v", name, err)
			continue
		}
		if !strings.Contains(output.Text, "line 1") {
			t.Errorf("a read with the path under %q returned %q", name, output.Text)
		}
	}
}

func TestAnOffsetAndALimitWrittenInQuotesAreStillNumbers(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	path := tenLines(t, root)

	output, err := run(t, tool, map[string]any{"path": path, "offset": "3", "limit": "2"})
	if err != nil {
		t.Fatalf("an offset and a limit written in quotes were refused: %v", err)
	}
	if !strings.HasPrefix(output.Text, "3: ") {
		t.Errorf("the read began with %q, want the third line", output.Text)
	}
	if lines := strings.Count(output.Text, "\n"); lines != 2 {
		t.Errorf("the read returned %d lines, want the two the limit asked for: %q", lines, output.Text)
	}
}

func TestAnOffsetThatIsNoNumberIsRefusedByNameAndNamesTheField(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	path := tenLines(t, root)

	_, err := run(t, tool, map[string]any{"path": path, "offset": "the third line"})
	if err == nil {
		t.Fatalf("an offset that is not a number was read as one")
	}
	if !strings.Contains(err.Error(), "offset") {
		t.Errorf("the refusal reads %q and does not name the field that is wrong", err)
	}
	if strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("the refusal reads %q, which is Go's words and not a person's", err)
	}
}

func TestAReadWithNothingToReadNamesTheFieldToWrite(t *testing.T) {
	tool, _ := newTool(t, nil, nil)

	_, err := run(t, tool, map[string]any{"offset": 2})
	if err == nil {
		t.Fatalf("a read with nothing to read was run")
	}
	if !strings.HasSuffix(err.Error(), `"path"`) {
		t.Errorf("the refusal reads %q and does not end with the name of the field to write", err)
	}
}
