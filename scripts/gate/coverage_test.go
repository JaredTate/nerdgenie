package gate

import (
	"strings"
	"testing"
)

// anUntestedPackage is a package with one exported function and no test file at
// all, which is the hole the reviewer found in the coverage gate.
func anUntestedPackage() map[string]string {
	return map[string]string{
		"internal/untested/doc.go": "// Package untested is a fixture package with no test file.\npackage untested\n",
		"internal/untested/untested.go": "package untested\n\n" +
			"// Shout returns a greeting nobody tests.\nfunc Shout() string {\n\treturn \"HELLO\"\n}\n",
	}
}

func TestTheCoverageGateFailsOnAPackageWithNoTestFile(t *testing.T) {
	files := aFixtureModule()
	for path, content := range anUntestedPackage() {
		files[path] = content
	}
	root := writeFixtureModule(t, files)
	script := copyScript(t, root, "coverage.sh")

	printed, code := runScript(t, script)

	if code == 0 {
		t.Fatalf("the coverage gate passed a package with no test file:\n%s", printed)
	}
	if !strings.Contains(printed, "internal/untested") {
		t.Errorf("the gate failed without naming the untested package, so nobody knows what to fix:\n%s", printed)
	}
	if !strings.Contains(printed, "NO TEST FILES") {
		t.Errorf("the gate did not say the package has no test files:\n%s", printed)
	}
}

func TestTheCoverageGatePassesAModuleWhereEveryPackageIsTested(t *testing.T) {
	root := writeFixtureModule(t, aFixtureModule())
	script := copyScript(t, root, "coverage.sh")

	printed, code := runScript(t, script)

	if code != 0 {
		t.Fatalf("the coverage gate failed a module that is fully tested, and it exited %d:\n%s", code, printed)
	}
	for _, folder := range []string{"internal/tested", "scripts/helper", "cmd/coeus"} {
		if !strings.Contains(printed, folder) {
			t.Errorf("the gate printed no row for %s, so a package could go unmeasured:\n%s", folder, printed)
		}
	}
}

func TestTheCoverageGateFailsAPackageBelowTheThreshold(t *testing.T) {
	files := aFixtureModule()
	files["internal/thin/thin.go"] = "package thin\n\n" +
		"// Half is covered and Rest is not.\nfunc Half() int {\n\treturn 1\n}\n\n" +
		"// Rest is never called by any test.\nfunc Rest() int {\n\tif Half() == 1 {\n\t\treturn 2\n\t}\n\treturn 3\n}\n"
	files["internal/thin/doc.go"] = "// Package thin is a fixture package that is barely tested.\npackage thin\n"
	files["internal/thin/thin_test.go"] = "package thin\n\nimport \"testing\"\n\n" +
		"func TestHalf(t *testing.T) {\n\tif Half() != 1 {\n\t\tt.Error(\"the number changed\")\n\t}\n}\n"
	root := writeFixtureModule(t, files)
	script := copyScript(t, root, "coverage.sh")

	printed, code := runScript(t, script)

	if code == 0 {
		t.Fatalf("the coverage gate passed a package under the threshold:\n%s", printed)
	}
	if !strings.Contains(printed, "UNDER THRESHOLD") {
		t.Errorf("the gate did not say which package is under the threshold:\n%s", printed)
	}
}
