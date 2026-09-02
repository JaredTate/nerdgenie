package testkit

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// updateGoldenFiles is set by running the tests with -update, which rewrites
// every golden file instead of comparing against it. Read the difference before
// you use it: a golden file rewritten without being read is a test that proves
// nothing.
var updateGoldenFiles = flag.Bool("update", false, "rewrite the golden files under testdata instead of comparing against them")

// Golden compares bytes with the file of that name under the package's testdata
// folder, and fails the test when they differ. Running the tests with -update
// writes the file instead.
func Golden(t testing.TB, name string, actual []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)

	if *updateGoldenFiles {
		if err := WriteGolden(path, actual); err != nil {
			t.Fatalf("cannot rewrite the golden file %s: %v", path, err)
		}
		return
	}

	same, err := GoldenMatches(path, actual)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !same {
		t.Errorf("the result does not match the golden file %s.\nRead the difference, and run the tests with -update only when the new result is right.\n--- want ---\n%s\n--- got ---\n%s",
			path, mustRead(path), actual)
	}
}

// GoldenMatches says whether the bytes are what the golden file holds.
func GoldenMatches(path string, actual []byte) (bool, error) {
	wanted, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("cannot read the golden file %s, so run the tests with -update to write it: %w", path, err)
	}
	return bytes.Equal(wanted, actual), nil
}

// WriteGolden writes a golden file and the folder above it.
func WriteGolden(path string, actual []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), contract.HomeFolderMode); err != nil {
		return fmt.Errorf("cannot make the folder for the golden file %s: %w", path, err)
	}
	if err := os.WriteFile(path, actual, contract.DataFileMode); err != nil {
		return fmt.Errorf("cannot write the golden file %s: %w", path, err)
	}
	return nil
}

// mustRead reads a file for a failure message and returns a note instead of
// failing when it cannot, because a failure message must never fail itself.
func mustRead(path string) []byte {
	content, err := os.ReadFile(path)
	if err != nil {
		return []byte("(the golden file could not be read)")
	}
	return content
}
