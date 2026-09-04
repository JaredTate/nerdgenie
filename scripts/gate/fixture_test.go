package gate

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The gate is three shell scripts and a Makefile, and none of them are Go, so
// the only honest way to test them is to build a small module in a temporary
// folder, copy the script into it, and run it. Every test in this package works
// that way.

// scriptsRoot is the folder this package's tests copy the scripts out of.
func scriptsRoot(t *testing.T) string {
	t.Helper()
	here, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot find the working directory: %v", err)
	}
	return filepath.Dir(here)
}

// writeFixtureModule writes the given files into a fresh temporary folder and
// returns its path. Every path is written with slashes and turned into the
// operating system's own form here.
func writeFixtureModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for path, content := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("cannot make the folder for %s: %v", path, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("cannot write %s into the fixture: %v", path, err)
		}
	}
	return root
}

// makeFixtureRepository turns the folder into a git repository and stages the
// paths given, so that a script driven by "git ls-files" has something to read.
//
// Every git command runs with the GIT_ settings stripped out of its
// environment: git exports GIT_DIR and GIT_WORK_TREE to the programs it runs,
// and GIT_DIR outranks the -C flag, so a fixture that inherited them would stage
// files into the real repository.
func makeFixtureRepository(t *testing.T, root string, paths ...string) {
	t.Helper()
	runGitInFixture(t, root, "init")
	runGitInFixture(t, root, append([]string{"add", "--"}, paths...)...)
}

// runGitInFixture runs one git command inside the fixture repository.
func runGitInFixture(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	command.Env = environmentWithoutGitSettings()
	if printed, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed in the fixture: %v\n%s", strings.Join(arguments, " "), err, printed)
	}
}

// environmentWithoutGitSettings is this process's environment with every GIT_
// setting taken out.
func environmentWithoutGitSettings() []string {
	kept := []string{}
	for _, setting := range os.Environ() {
		if strings.HasPrefix(setting, "GIT_") {
			continue
		}
		kept = append(kept, setting)
	}
	return kept
}

// copyScript copies one script out of the repository's scripts folder into the
// fixture module, keeping it executable, so that the test runs the same file the
// gate runs.
func copyScript(t *testing.T, root string, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(scriptsRoot(t), name))
	if err != nil {
		t.Fatalf("cannot read the script %s to copy it: %v", name, err)
	}
	destination := filepath.Join(root, "scripts", name)
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatalf("cannot make the fixture's scripts folder: %v", err)
	}
	if err := os.WriteFile(destination, content, 0o755); err != nil {
		t.Fatalf("cannot write the script %s into the fixture: %v", name, err)
	}
	return destination
}

// runScript runs one script inside the fixture module and returns everything it
// printed and the code it exited with.
func runScript(t *testing.T, path string, arguments ...string) (string, int) {
	t.Helper()
	command := exec.Command("bash", append([]string{path}, arguments...)...)
	command.Dir = filepath.Dir(path)
	printed, err := command.CombinedOutput()
	if err == nil {
		return string(printed), 0
	}
	var exited *exec.ExitError
	if !errors.As(err, &exited) {
		t.Fatalf("cannot run the script %s: %v\n%s", path, err, printed)
	}
	return string(printed), exited.ExitCode()
}

// aTestedPackage is the source of a package with one function and a test that
// covers it, which is what a package that passes the gate looks like.
func aTestedPackage(name string) map[string]string {
	return map[string]string{
		"internal/" + name + "/doc.go": "// Package " + name + " is a fixture package.\npackage " + name + "\n",
		"internal/" + name + "/" + name + ".go": "package " + name + "\n\n" +
			"// Greet returns a greeting.\nfunc Greet() string {\n\treturn \"hello\"\n}\n",
		"internal/" + name + "/" + name + "_test.go": "package " + name + "\n\nimport \"testing\"\n\n" +
			"func TestGreet(t *testing.T) {\n\tif Greet() != \"hello\" {\n\t\tt.Error(\"the greeting changed\")\n\t}\n}\n",
	}
}

// aFixtureModule is a module with the same three roots the repository has, every
// package of it tested, so that a test can add one broken package and see the
// gate catch it.
func aFixtureModule() map[string]string {
	files := map[string]string{
		"go.mod": "module example.com/gatefixture\n\ngo 1.27\n",
		"cmd/nerdgenie/main.go": "package main\n\n// Run returns the exit code.\nfunc Run() int {\n\treturn 0\n}\n\n" +
			"func main() {\n\t_ = Run()\n}\n",
		"cmd/nerdgenie/main_test.go": "package main\n\nimport \"testing\"\n\n" +
			"func TestRun(t *testing.T) {\n\tif Run() != 0 {\n\t\tt.Error(\"the exit code changed\")\n\t}\n}\n",
		"scripts/helper/helper.go": "// Package helper is a fixture package.\npackage helper\n\n" +
			"// Help returns advice.\nfunc Help() string {\n\treturn \"read the brief\"\n}\n",
		"scripts/helper/helper_test.go": "package helper\n\nimport \"testing\"\n\n" +
			"func TestHelp(t *testing.T) {\n\tif Help() == \"\" {\n\t\tt.Error(\"the advice went missing\")\n\t}\n}\n",
	}
	for path, content := range aTestedPackage("tested") {
		files[path] = content
	}
	return files
}
