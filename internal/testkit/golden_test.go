package testkit_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestGoldenAcceptsBytesThatMatchTheFileOnDisk(t *testing.T) {
	testkit.Golden(t, "greeting.txt", []byte("hello from the golden file\n"))
}

func TestGoldenComparisonReportsTheDifference(t *testing.T) {
	same, err := testkit.GoldenMatches(filepath.Join("testdata", "greeting.txt"), []byte("hello from the golden file\n"))
	if err != nil {
		t.Fatalf("comparing with the golden file failed: %v", err)
	}
	if !same {
		t.Error("the golden file did not match bytes that are the same as its contents")
	}

	same, err = testkit.GoldenMatches(filepath.Join("testdata", "greeting.txt"), []byte("something else\n"))
	if err != nil {
		t.Fatalf("comparing with the golden file failed: %v", err)
	}
	if same {
		t.Error("the golden file matched bytes that differ from its contents")
	}
}

func TestGoldenComparisonSaysSoWhenTheFileIsNotThere(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nowhere.txt")

	if _, err := testkit.GoldenMatches(missing, []byte("anything")); err == nil {
		t.Fatal("comparing with a golden file that is not there was reported as a success, want an error saying to run the tests with -update")
	}
}

func TestGoldenWritesTheFileWhenTheTestIsRunWithUpdate(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "written.txt")

	if err := testkit.WriteGolden(path, []byte("written by -update\n")); err != nil {
		t.Fatalf("writing the golden file failed: %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the golden file that was just written: %v", err)
	}
	if string(written) != "written by -update\n" {
		t.Errorf("the golden file holds %q, want the bytes that were written", written)
	}
}
