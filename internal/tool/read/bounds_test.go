package read_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/tool/read"
)

func TestAVeryLongLineIsCutAndSaysHowMuchWasLeftOff(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	path := filepath.Join(root, "one-long-line.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", read.MaxLineRunes+40)+"\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the fixture file: %v", err)
	}

	output, err := run(t, tool, map[string]any{"path": path})
	if err != nil {
		t.Fatalf("reading a file with one very long line failed: %v", err)
	}
	if !strings.Contains(output.Text, "40 more characters") {
		t.Errorf("the line was cut without saying how much was left off: %q", output.Text[len(output.Text)-60:])
	}
}

func TestAFileOfManyLinesStopsAtTheLineCapAndSaysHowToReadOn(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	path := filepath.Join(root, "many.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("a line\n", read.MaxLines+10)), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the fixture file: %v", err)
	}

	output, err := run(t, tool, map[string]any{"path": path})
	if err != nil {
		t.Fatalf("reading a long file failed: %v", err)
	}
	if !strings.Contains(output.Text, fmt.Sprintf("offset of %d", read.MaxLines+1)) {
		t.Errorf("the read stopped at the line cap without saying where to read on from")
	}
}

func TestAFileOfManyBytesStopsAtTheByteCap(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	path := filepath.Join(root, "wide.txt")
	line := strings.Repeat("y", 1000) + "\n"
	if err := os.WriteFile(path, []byte(strings.Repeat(line, 400)), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the fixture file: %v", err)
	}

	output, err := run(t, tool, map[string]any{"path": path})
	if err != nil {
		t.Fatalf("reading a wide file failed: %v", err)
	}
	if len(output.Text) > read.MaxBytes+200 {
		t.Errorf("the read returned %d bytes, and the cap is %d", len(output.Text), read.MaxBytes)
	}
	if !strings.Contains(output.Text, "bytes") {
		t.Errorf("the read stopped at the byte cap without saying so")
	}
}

func TestAFolderOfManyEntriesStopsAtTheEntryCap(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	crowded := filepath.Join(root, "crowded")
	if err := os.MkdirAll(crowded, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the crowded folder: %v", err)
	}
	for at := range read.MaxEntries + 5 {
		name := filepath.Join(crowded, fmt.Sprintf("file-%05d", at))
		if err := os.WriteFile(name, []byte("x"), contract.DataFileMode); err != nil {
			t.Fatalf("cannot write %s: %v", name, err)
		}
	}

	output, err := run(t, tool, map[string]any{"path": crowded})
	if err != nil {
		t.Fatalf("listing a crowded folder failed: %v", err)
	}
	if !strings.Contains(output.Text, "5 more entries") {
		t.Errorf("the listing stopped without saying how many entries it left out")
	}
}

func TestAToolWithNoFoldersToReadSaysSo(t *testing.T) {
	tool := read.New(read.Settings{})

	_, err := run(t, tool, map[string]any{"path": "/tmp/anything"})
	if err == nil {
		t.Fatalf("a file was read by a tool with no folders wired into it")
	}
	if !strings.Contains(err.Error(), "sandbox roots") {
		t.Errorf("the refusal reads %q and does not say what is missing", err)
	}
}

func TestAFileThatIsNotThereSaysSo(t *testing.T) {
	tool, root := newTool(t, nil, nil)

	if _, err := run(t, tool, map[string]any{"path": filepath.Join(root, "nothing.md")}); err == nil {
		t.Errorf("a file that is not there was read")
	}
}
