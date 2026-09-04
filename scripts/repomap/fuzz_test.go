package main

import (
	"strings"
	"testing"
)

// The generator reads two things from outside itself: the NUL-separated listing
// git prints, and the paths in it. Both have a fuzz target, because anything
// that parses bytes from outside the program does.

func FuzzPathsInListing(f *testing.F) {
	for _, seed := range []string{
		"",
		"README.md\x00",
		"README.md\x00internal/contract/doc.go\x00",
		"\x00\x00\x00",
		"a path with spaces.md\x00",
		"weird\nname\x00another",
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, listing []byte) {
		paths := pathsInListing(listing)

		for _, path := range paths {
			if path == "" {
				t.Fatalf("the listing %q produced an empty path", listing)
			}
			if strings.Contains(path, "\x00") {
				t.Fatalf("the path %q still holds a separator, so the listing was not split", path)
			}
		}
		if rejoined := strings.Join(paths, "\x00"); len(rejoined) > len(listing) {
			t.Fatalf("the listing %q of %d bytes produced %d bytes of paths", listing, len(listing), len(rejoined))
		}
	})
}

func FuzzExcludedPath(f *testing.F) {
	for _, seed := range []string{
		"", "README.md", ".git/config", "node_modules/library/index.js",
		"bin/nerdgenie", "dist/nerdgenie", "coverage/report.txt", "notes.log",
		"build.tsbuildinfo", "a/b/c/d/e/f", "/", "//", "..",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, path string) {
		excluded := excludedPath(path)

		if excluded != excludedPath(path) {
			t.Fatalf("the path %q was excluded and then not, and the answer must be the same every time", path)
		}
		if !excluded {
			return
		}

		for _, folder := range excludedFolders {
			if strings.Contains(path, folder) {
				return
			}
		}
		for _, ending := range excludedEndings {
			if strings.HasSuffix(path, ending) {
				return
			}
		}
		t.Fatalf("the path %q was excluded and holds no excluded folder and no excluded ending", path)
	})
}
