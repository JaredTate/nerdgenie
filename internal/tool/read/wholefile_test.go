package read_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// aFileOfThisManyBytes writes a file of lines of a hundred bytes each, so that a
// test can ask for a few lines of something far bigger than the read cap.
func aFileOfThisManyBytes(t *testing.T, path string, bytes int) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, contract.DataFileMode)
	if err != nil {
		t.Fatalf("cannot write the fixture file: %v", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Fatalf("cannot close the fixture file: %v", err)
		}
	}()
	block := strings.Repeat(strings.Repeat("x", 99)+"\n", 1000)
	for written := 0; written < bytes; written += len(block) {
		if _, err := file.WriteString(block); err != nil {
			t.Fatalf("cannot write the fixture file: %v", err)
		}
	}
}

// bytesAllocatedBy is how much memory the work allocated in all, which is what
// says whether a file was held whole or read a line at a time.
func bytesAllocatedBy(work func()) uint64 {
	before := runtime.MemStats{}
	after := runtime.MemStats{}
	runtime.GC()
	runtime.ReadMemStats(&before)
	work()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

func TestAFileFarBiggerThanTheCapIsNotHeldWholeToReadFiveLinesOfIt(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	path := filepath.Join(root, "large.log")
	aFileOfThisManyBytes(t, path, 8<<20)

	text := ""
	allocated := bytesAllocatedBy(func() {
		output, err := run(t, tool, map[string]any{"path": path, "limit": 5})
		if err != nil {
			t.Fatalf("reading five lines of a large file failed: %v", err)
		}
		text = output.Text
	})

	if lines := strings.Count(text, "\n"); lines != 5 {
		t.Errorf("the read returned %d lines, want the five it was asked for", lines)
	}
	// One megabyte is more than the read cap of 256 kilobytes and a great deal
	// less than the eight megabytes the file holds, so a read that allocates
	// less than this read what it was asked for and not the whole file.
	if allocated > 1<<20 {
		t.Errorf("reading five lines of an eight-megabyte file allocated %d bytes, and the file must not be held whole", allocated)
	}
}

func TestAFileFarBiggerThanTheCapStillReadsFromAnOffset(t *testing.T) {
	tool, root := newTool(t, nil, nil)
	path := filepath.Join(root, "large.log")
	aFileOfThisManyBytes(t, path, 2<<20)

	output, err := run(t, tool, map[string]any{"path": path, "offset": 20000, "limit": 2})
	if err != nil {
		t.Fatalf("reading a window in the middle of a large file failed: %v", err)
	}
	if !strings.HasPrefix(output.Text, "20000: ") {
		t.Errorf("the read began with %q, want line twenty thousand", output.Text[:40])
	}
}
