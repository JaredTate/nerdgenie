package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureEnvironment is the environment every fixture git command runs in, with
// every GIT_ variable taken out.
//
// This matters more than it looks. Git exports GIT_DIR, GIT_INDEX_FILE, and
// GIT_WORK_TREE to the hooks it runs, and those variables outrank the working
// directory. A fixture that inherits them runs its "git init" and "git add"
// against the real repository instead of the temporary one, which both damages
// the real repository and makes these tests pass for the wrong reason, because
// the generator would then be listing the real repository's files. HomeRecon
// learned this the hard way and wrote it down at the top of
// ~/Code/homerecon/scripts/repo-map/repo-map.test.cjs.
func fixtureEnvironment() []string {
	kept := []string{}
	for _, setting := range os.Environ() {
		if strings.HasPrefix(setting, "GIT_") {
			continue
		}
		kept = append(kept, setting)
	}
	return kept
}

// newFixtureRepository makes a temporary git repository holding the files given
// as paths relative to its root, and stages every one of them.
func newFixtureRepository(t *testing.T, paths ...string) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init")
	for _, path := range paths {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("cannot make the folder for %s: %v", path, err)
		}
		if err := os.WriteFile(full, []byte("fixture\n"), 0o644); err != nil {
			t.Fatalf("cannot write %s: %v", path, err)
		}
	}
	runGit(t, root, append([]string{"add", "--"}, paths...)...)
	return root
}

// runGit runs one git command inside the fixture repository.
func runGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	command.Env = fixtureEnvironment()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed in the fixture: %v\n%s", strings.Join(arguments, " "), err, output)
	}
}

func TestGenerateProducesTheSameMapEveryTime(t *testing.T) {
	root := newFixtureRepository(t, "README.md", "internal/contract/doc.go", "cmd/nerdgenie/main.go")

	first, err := generate(root)
	if err != nil {
		t.Fatalf("generating the map failed: %v", err)
	}
	second, err := generate(root)
	if err != nil {
		t.Fatalf("generating the map a second time failed: %v", err)
	}

	if first != second {
		t.Error("the generator produced two different maps for the same tree, and it must be deterministic")
	}
	for _, wanted := range []string{
		"# Repository Map",
		"<!-- generated: scripts/repomap -->",
		"<!-- repo-map-contract: v1 -->",
		"## Roots",
		"## Tree",
		"README.md",
		"cmd/nerdgenie/main.go",
		"internal/contract/doc.go",
	} {
		if !strings.Contains(first, wanted) {
			t.Errorf("the map is missing %q:\n%s", wanted, first)
		}
	}
}

func TestGenerateSortsThePathsSoTheMapNeverShufflesItself(t *testing.T) {
	root := newFixtureRepository(t, "zebra.md", "apple.md", "middle.md")

	map1, err := generate(root)
	if err != nil {
		t.Fatalf("generating the map failed: %v", err)
	}

	order := []string{"apple.md", "middle.md", "zebra.md"}
	previous := -1
	for _, path := range order {
		at := strings.Index(map1, path)
		if at < 0 {
			t.Fatalf("the map does not list %s:\n%s", path, map1)
		}
		if at < previous {
			t.Errorf("the map lists %s out of order:\n%s", path, map1)
		}
		previous = at
	}
}

func TestExcludedFoldersAndFilesNeverAppear(t *testing.T) {
	root := newFixtureRepository(t,
		"README.md",
		"node_modules/library/index.js",
		"bin/nerdgenie",
		"dist/nerdgenie-linux-amd64",
		"coverage/report.txt",
		"notes.log",
		"build.tsbuildinfo",
	)

	generated, err := generate(root)
	if err != nil {
		t.Fatalf("generating the map failed: %v", err)
	}

	for _, unwanted := range []string{"node_modules", "bin/nerdgenie", "dist/", "coverage/", "notes.log", "build.tsbuildinfo"} {
		if strings.Contains(generated, unwanted) {
			t.Errorf("the map lists %q, and the generator is supposed to leave it out:\n%s", unwanted, generated)
		}
	}
	if !strings.Contains(generated, "README.md") {
		t.Error("the map left out README.md, and only the excluded things should be missing")
	}
}

func TestUntrackedFilesNeverAppearInsideAGitWorkTree(t *testing.T) {
	root := newFixtureRepository(t, "README.md")
	if err := os.WriteFile(filepath.Join(root, "scratch.txt"), []byte("not staged\n"), 0o644); err != nil {
		t.Fatalf("cannot write the untracked file: %v", err)
	}

	generated, err := generate(root)
	if err != nil {
		t.Fatalf("generating the map failed: %v", err)
	}

	if strings.Contains(generated, "scratch.txt") {
		t.Errorf("the map lists an untracked file, and it documents what a fresh clone holds:\n%s", generated)
	}
}

func TestGenerateWalksTheTreeWhenThereIsNoGitRepository(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatalf("cannot write the fixture file: %v", err)
	}

	generated, err := generate(root)
	if err != nil {
		t.Fatalf("generating the map outside a git work tree failed: %v", err)
	}
	if !strings.Contains(generated, "README.md") {
		t.Errorf("the map left out the only file in the tree:\n%s", generated)
	}
}

func TestWalkingATreeWithNoGitStillSkipsTheExcludedFolders(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"README.md", "node_modules/library/index.js", "bin/nerdgenie", "notes.log"} {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("cannot make the folder for %s: %v", path, err)
		}
		if err := os.WriteFile(full, []byte("fixture\n"), 0o644); err != nil {
			t.Fatalf("cannot write %s: %v", path, err)
		}
	}

	generated, err := generate(root)
	if err != nil {
		t.Fatalf("generating the map outside a git work tree failed: %v", err)
	}

	for _, unwanted := range []string{"node_modules", "bin/nerdgenie", "notes.log"} {
		if strings.Contains(generated, unwanted) {
			t.Errorf("the map lists %q from a tree with no git in it:\n%s", unwanted, generated)
		}
	}
	if !strings.Contains(generated, "README.md") {
		t.Errorf("the map left out README.md:\n%s", generated)
	}
}

// theRealGitDirectory is where this repository keeps its git folder, asked of
// git itself so that the answer is right inside a work tree as well as inside a
// plain clone.
func theRealGitDirectory(t *testing.T) string {
	t.Helper()
	command := exec.Command("git", "rev-parse", "--absolute-git-dir")
	command.Env = fixtureEnvironment()
	answer, err := command.Output()
	if err != nil {
		t.Fatalf("cannot ask git where this repository's git folder is: %v", err)
	}
	return strings.TrimSpace(string(answer))
}

func TestTheGeneratorIgnoresTheGitVariablesInItsOwnEnvironment(t *testing.T) {
	root := newFixtureRepository(t, "README.md")
	t.Setenv("GIT_DIR", theRealGitDirectory(t))

	generated, err := generate(root)

	if err != nil {
		t.Fatalf("generating the map with GIT_DIR set failed: %v", err)
	}
	tree := treeOf(t, generated)
	for _, unwanted := range []string{"ARCHITECTURE.md", "internal/testkit/", "scripts/repomap/"} {
		if strings.Contains(tree, unwanted) {
			t.Errorf("with GIT_DIR set the generator mapped this repository instead of the fixture, because it lists %q:\n%s",
				unwanted, tree)
		}
	}
	if !strings.Contains(tree, "README.md") {
		t.Errorf("the map left out the only file in the fixture:\n%s", tree)
	}
}

// treeOf is the list of paths inside a generated map, without the header, which
// names files of its own that would otherwise read as a match.
func treeOf(t *testing.T, generated string) string {
	t.Helper()
	_, tree, found := strings.Cut(generated, "```text\n")
	if !found {
		t.Fatalf("the generated map has no tree block in it:\n%s", generated)
	}
	return tree
}

func TestAGitWorkTreeGitCannotReadIsAnErrorRatherThanASilentWalk(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatalf("cannot write the fixture file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "scratch.txt"), []byte("untracked\n"), 0o644); err != nil {
		t.Fatalf("cannot write the untracked file: %v", err)
	}
	// A .git file git cannot make sense of: the folder looks like a work tree and
	// git refuses to read it.
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("this is not a git directory\n"), 0o644); err != nil {
		t.Fatalf("cannot write the broken .git file: %v", err)
	}

	generated, err := generate(root)

	if err == nil {
		t.Fatalf("a work tree git could not read was mapped by walking it instead, which lists untracked files:\n%s", generated)
	}
	if !strings.Contains(err.Error(), "git") {
		t.Errorf("the failure does not say git could not read it: %v", err)
	}
}

func TestReadingCappedStopsAtTheCapAndSaysSo(t *testing.T) {
	within, err := readCapped(strings.NewReader("a short answer"), 64)
	if err != nil {
		t.Fatalf("reading well inside the cap failed: %v", err)
	}
	if string(within) != "a short answer" {
		t.Errorf("the reader gave back %q, want what it was given", within)
	}

	_, err = readCapped(strings.NewReader(strings.Repeat("a", 100)), 64)

	if err == nil {
		t.Fatal("a hundred bytes came back through a sixty-four byte cap, and every buffer has a cap")
	}
	if !strings.Contains(err.Error(), "64") {
		t.Errorf("the failure does not say what the cap is: %v", err)
	}
}

// writeFixtureTree writes the paths given into a fresh folder that is not a git
// work tree, so the generator walks it.
func writeFixtureTree(t *testing.T, paths ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, path := range paths {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("cannot make the folder for %s: %v", path, err)
		}
		if err := os.WriteFile(full, []byte("fixture\n"), 0o644); err != nil {
			t.Fatalf("cannot write %s: %v", path, err)
		}
	}
	return root
}

func TestTheMapIsSortedEvenWhenTheWalkFindsThingsInAnotherOrder(t *testing.T) {
	// A walk visits the folder "a" before the file "a.md", and sorted order is the
	// other way round, because a full stop sorts before a slash.
	root := writeFixtureTree(t, "a.md", "a/z.md")

	generated, err := generate(root)

	if err != nil {
		t.Fatalf("generating the map failed: %v", err)
	}
	tree := treeOf(t, generated)
	if strings.Index(tree, "\na.md\n") > strings.Index(tree, "\na/z.md\n") {
		t.Errorf("the map lists a/z.md before a.md, so the paths were never sorted:\n%s", tree)
	}
}

func TestAGitFolderInsideTheTreeIsNeverListed(t *testing.T) {
	root := writeFixtureTree(t, "README.md", "vendor/library/.git/config", "vendor/library/index.js")

	generated, err := generate(root)

	if err != nil {
		t.Fatalf("generating the map failed: %v", err)
	}
	tree := treeOf(t, generated)
	if strings.Contains(tree, ".git") {
		t.Errorf("the map lists something inside a .git folder, and version-control internals are never in it:\n%s", tree)
	}
	if !strings.Contains(tree, "vendor/library/index.js") {
		t.Errorf("the map left out a file beside the .git folder:\n%s", tree)
	}
}

// runGitAllowingFailure runs one git command that is expected to fail, such as a
// merge that conflicts on purpose.
func runGitAllowingFailure(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	command.Env = fixtureEnvironment()
	_, _ = command.CombinedOutput()
}

// commitFixture makes one commit in the fixture repository, with an identity of
// its own so that it does not depend on whatever this machine is configured
// with.
func commitFixture(t *testing.T, root string, message string) {
	t.Helper()
	runGit(t, root, "-c", "user.email=fixture@example.test", "-c", "user.name=Fixture",
		"commit", "--no-gpg-sign", "-m", message)
}

// newConflictedRepository leaves one file in conflict, which is the one state
// where git prints a path more than once: one line per stage of the index.
func newConflictedRepository(t *testing.T) string {
	t.Helper()
	root := newFixtureRepository(t, "README.md", "notes.md")
	commitFixture(t, root, "the first commit")

	runGit(t, root, "checkout", "-b", "the-other-branch")
	writeFixtureFile(t, root, "README.md", "what the other branch says\n")
	runGit(t, root, "add", "--", "README.md")
	commitFixture(t, root, "the other branch's commit")

	runGit(t, root, "checkout", "-")
	writeFixtureFile(t, root, "README.md", "what this branch says\n")
	runGit(t, root, "add", "--", "README.md")
	commitFixture(t, root, "this branch's commit")

	runGitAllowingFailure(t, root, "merge", "--no-gpg-sign", "the-other-branch")
	return root
}

// writeFixtureFile writes one file inside a fixture repository.
func writeFixtureFile(t *testing.T, root string, path string, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("cannot write %s: %v", path, err)
	}
}

func TestAFileLeftInConflictIsListedOnceAndNotThreeTimes(t *testing.T) {
	root := newConflictedRepository(t)
	if !strings.Contains(gitListing(t, root), "README.md\x00README.md") {
		t.Skip("this git does not leave the conflicted file in the index more than once, so there is nothing to prove here")
	}

	generated, err := generate(root)

	if err != nil {
		t.Fatalf("generating the map for a repository with a conflict failed: %v", err)
	}
	tree := treeOf(t, generated)
	if times := strings.Count(tree, "README.md"); times != 1 {
		t.Errorf("the map lists README.md %d times, and git prints one line per stage of a file in conflict:\n%s", times, tree)
	}
}

// gitListing is what git prints for the tracked files, used to confirm the
// fixture really is in the state the test is about.
func gitListing(t *testing.T, root string) string {
	t.Helper()
	command := exec.Command("git", "-C", root, "ls-files", "-z")
	command.Env = fixtureEnvironment()
	printed, err := command.Output()
	if err != nil {
		t.Fatalf("cannot ask git which files the fixture tracks: %v", err)
	}
	return string(printed)
}

func TestGenerateSaysSoWhenTheFolderIsNotThere(t *testing.T) {
	if _, err := generate(filepath.Join(t.TempDir(), "nowhere")); err == nil {
		t.Fatal("generating a map for a folder that is not there was reported as a success, want an error naming it")
	}
}
