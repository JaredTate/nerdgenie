package loop_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/loop"
)

// TestAFinishedJobWritesTheMapAndKeepsAHandWrittenOne holds that a job that
// finishes done leaves a generated REPO_MAP.md in its folder beside
// AGENTS.md: a legend of the top folders and a tree of the files, with the
// dependency and build folders left out, and that a map without the
// generated mark is the person's and is never touched.
func TestAFinishedJobWritesTheMapAndKeepsAHandWrittenOne(t *testing.T) {
	built, agents := aFinishingJob(t)
	folder := filepath.Dir(agents)
	for _, path := range []string{"src/engine.js", "src/hazards.js", "test/engine.test.js", "node_modules/left-pad/index.js", "dist/bundle.js", ".git/HEAD", "index.html"} {
		whole := filepath.Join(folder, path)
		if err := os.MkdirAll(filepath.Dir(whole), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(whole, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	runTheJobToTheEnd(t, built.loop, built.channel)

	written, err := os.ReadFile(filepath.Join(folder, loop.MapFile))
	if err != nil {
		t.Fatalf("the finished job wrote no %s: %v", loop.MapFile, err)
	}
	text := string(written)
	for _, want := range []string{loop.GeneratedMark, "## Roots", "`src/`", "2 files", "`test/`", "## Tree", "src/engine.js", "test/engine.test.js", "index.html"} {
		if !strings.Contains(text, want) {
			t.Errorf("the map lacks %q; it reads:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"node_modules", "dist/bundle.js", ".git/"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("the map lists %q, which is not the project's own work:\n%s", unwanted, text)
		}
	}

	again, agentsAgain := aFinishingJob(t)
	mine := filepath.Join(filepath.Dir(agentsAgain), loop.MapFile)
	if err := os.WriteFile(mine, []byte("# My map\n\nWritten by hand.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTheJobToTheEnd(t, again.loop, again.channel)
	kept, err := os.ReadFile(mine)
	if err != nil {
		t.Fatal(err)
	}
	if string(kept) != "# My map\n\nWritten by hand.\n" {
		t.Errorf("a map the person wrote was changed:\n%s", string(kept))
	}
}
