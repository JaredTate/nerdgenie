package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestTheRepositoryMapIsNotStale is the drift test make check runs. It
// regenerates the map for this repository and compares it byte for byte with
// REPO_MAP.md, so that a file added without running "make repo-map" fails the
// build rather than quietly leaving the map wrong.
func TestTheRepositoryMapIsNotStale(t *testing.T) {
	root := repositoryRoot(t)

	generated, err := generate(root)
	if err != nil {
		t.Fatalf("generating the map for this repository failed: %v", err)
	}
	onDisk, err := os.ReadFile(filepath.Join(root, "REPO_MAP.md"))
	if err != nil {
		t.Fatalf("cannot read REPO_MAP.md: %v", err)
	}

	if !bytes.Equal([]byte(generated), onDisk) {
		t.Errorf("REPO_MAP.md is stale. Run \"make repo-map\" and commit the result.\n"+
			"The generated map is %d bytes and the one on disk is %d bytes.", len(generated), len(onDisk))
	}
}

// TestTheDriftTestFailsWhenAFileIsAddedWithoutRegenerating proves the drift test
// does its job, by doing to a fixture repository exactly what a forgetful worker
// does to this one.
func TestTheDriftTestFailsWhenAFileIsAddedWithoutRegenerating(t *testing.T) {
	root := newFixtureRepository(t, "README.md")

	before, err := generate(root)
	if err != nil {
		t.Fatalf("generating the first map failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "REPO_MAP.md"), []byte(before), 0o644); err != nil {
		t.Fatalf("cannot write the fixture's map: %v", err)
	}
	runGit(t, root, "add", "--", "REPO_MAP.md")

	// Regenerate now that REPO_MAP.md itself is tracked, and save that, so the
	// fixture starts from a map that is up to date rather than one that is
	// already stale.
	settled, err := generate(root)
	if err != nil {
		t.Fatalf("generating the settled map failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "REPO_MAP.md"), []byte(settled), 0o644); err != nil {
		t.Fatalf("cannot write the settled map: %v", err)
	}
	if again, _ := generate(root); again != settled {
		t.Fatalf("the fixture's map is not settled, so the rest of this test would prove nothing")
	}

	// Now add a file the way a worker does, and do not regenerate.
	if err := os.WriteFile(filepath.Join(root, "internal", "brand", "new.go"), nil, 0o644); err != nil {
		if err := os.MkdirAll(filepath.Join(root, "internal", "brand"), 0o755); err != nil {
			t.Fatalf("cannot make the folder for the new file: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "internal", "brand", "new.go"), []byte("package brand\n"), 0o644); err != nil {
			t.Fatalf("cannot write the new file: %v", err)
		}
	}
	runGit(t, root, "add", "--", "internal/brand/new.go")

	stale, err := os.ReadFile(filepath.Join(root, "REPO_MAP.md"))
	if err != nil {
		t.Fatalf("cannot read the fixture's map: %v", err)
	}
	regenerated, err := generate(root)
	if err != nil {
		t.Fatalf("regenerating the map failed: %v", err)
	}
	if string(stale) == regenerated {
		t.Error("a file was added and the map did not change, so the drift test would never catch a stale map")
	}
}

// repositoryRoot walks up from this test's folder until it finds the go.mod
// file, which is the root of this module. The walk is bounded, because every
// loop in Coeus is bounded.
func repositoryRoot(t *testing.T) string {
	t.Helper()
	here, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot find the working directory: %v", err)
	}
	for range 20 {
		if _, err := os.Stat(filepath.Join(here, "go.mod")); err == nil {
			return here
		}
		parent := filepath.Dir(here)
		if parent == here {
			break
		}
		here = parent
	}
	t.Fatal("cannot find go.mod above this test, so the repository root is unknown")
	return ""
}
