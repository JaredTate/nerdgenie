package search_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/tool/search"
)

func TestOneFileCanBeSearchedOnItsOwn(t *testing.T) {
	bothWays(t, func(t *testing.T, tool *search.Tool, root string) {
		output, err := run(t, tool, map[string]any{"pattern": "return", "path": filepath.Join(root, "alpha.go")})
		if err != nil {
			t.Fatalf("searching one file failed: %v", err)
		}
		if !strings.Contains(output.Text, "alpha.go:4") {
			t.Errorf("searching one file found %q, want the line inside it", output.Text)
		}
	})
}

func TestTheFoldersNothingIsSearchedInAreSkipped(t *testing.T) {
	tool, root := newTool(t, filepath.Join(t.TempDir(), "no-ripgrep-here"))
	for _, folder := range []string{".git", "node_modules"} {
		path := filepath.Join(root, folder, "hidden.go")
		if err := os.MkdirAll(filepath.Dir(path), contract.HomeFolderMode); err != nil {
			t.Fatalf("cannot make %s: %v", folder, err)
		}
		if err := os.WriteFile(path, []byte("func alpha() {}\n"), contract.DataFileMode); err != nil {
			t.Fatalf("cannot write into %s: %v", folder, err)
		}
	}

	output, err := run(t, tool, map[string]any{"pattern": "alpha", "path": root})
	if err != nil {
		t.Fatalf("searching failed: %v", err)
	}
	if strings.Contains(output.Text, "hidden.go") {
		t.Errorf("the search looked inside a folder nothing is searched in: %q", output.Text)
	}
}

func TestAFileThatIsNotTextIsPassedOver(t *testing.T) {
	tool, root := newTool(t, filepath.Join(t.TempDir(), "no-ripgrep-here"))
	path := filepath.Join(root, "picture.bin")
	if err := os.WriteFile(path, []byte{'a', 'l', 'p', 'h', 'a', 0x00, 0x01}, contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the fixture file: %v", err)
	}

	output, err := run(t, tool, map[string]any{"pattern": "alpha", "path": root})
	if err != nil {
		t.Fatalf("searching failed: %v", err)
	}
	if strings.Contains(output.Text, "picture.bin:") {
		t.Errorf("a file that is not text had its lines searched: %q", output.Text)
	}
}

func TestAVeryLongMatchingLineIsCut(t *testing.T) {
	tool, root := newTool(t, filepath.Join(t.TempDir(), "no-ripgrep-here"))
	path := filepath.Join(root, "wide.txt")
	if err := os.WriteFile(path, []byte("alpha"+strings.Repeat("z", search.MaxRowRunes+50)+"\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the fixture file: %v", err)
	}

	output, err := run(t, tool, map[string]any{"pattern": "alpha", "path": path})
	if err != nil {
		t.Fatalf("searching failed: %v", err)
	}
	for _, row := range strings.Split(strings.TrimSpace(output.Text), "\n") {
		if len([]rune(row)) > search.MaxRowRunes+10 {
			t.Errorf("a row is %d characters, and the cap is %d", len([]rune(row)), search.MaxRowRunes)
		}
	}
}

func TestAFileTooBigToReadIsPassedOver(t *testing.T) {
	tool, root := newTool(t, filepath.Join(t.TempDir(), "no-ripgrep-here"))
	path := filepath.Join(root, "enormous.txt")
	held := append([]byte("alpha\n"), make([]byte, search.MaxFileBytes)...)
	if err := os.WriteFile(path, held, contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the enormous file: %v", err)
	}

	output, err := run(t, tool, map[string]any{"pattern": "alpha", "path": root})
	if err != nil {
		t.Fatalf("searching failed: %v", err)
	}
	if strings.Contains(output.Text, "enormous.txt:") {
		t.Errorf("a file over the size cap had its lines read: %q", output.Text)
	}
}

func TestASearchProgramThatFailsIsReportedRatherThanIgnored(t *testing.T) {
	root := t.TempDir()
	broken := filepath.Join(root, "broken-ripgrep")
	if err := os.WriteFile(broken, []byte("#!/bin/sh\necho 'this program is broken' >&2\nexit 2\n"), 0o700); err != nil {
		t.Fatalf("cannot write the broken search program: %v", err)
	}
	tool := search.New(search.Settings{
		Allowed: func(asked string) (string, error) { return asked, nil },
		Ripgrep: broken,
	})

	_, err := run(t, tool, map[string]any{"pattern": "alpha", "path": root})
	if err == nil {
		t.Fatalf("a search program that failed was treated as a search that found nothing")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("the failure reads %q and does not say what the program complained about", err)
	}
}

func TestAToolWithNoFoldersToSearchSaysSo(t *testing.T) {
	tool := search.New(search.Settings{})

	_, err := run(t, tool, map[string]any{"pattern": "alpha", "path": "/tmp"})
	if err == nil {
		t.Fatalf("a search was run by a tool with no folders wired into it")
	}
	if !strings.Contains(err.Error(), "sandbox roots") {
		t.Errorf("the refusal reads %q and does not say what is missing", err)
	}
}

func TestAFolderThatIsNotThereSaysSo(t *testing.T) {
	tool, root := newTool(t, "")

	if _, err := run(t, tool, map[string]any{"pattern": "alpha", "path": filepath.Join(root, "nowhere")}); err == nil {
		t.Errorf("a folder that is not there was searched")
	}
}
