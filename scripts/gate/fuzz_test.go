package gate

import (
	"strings"
	"testing"
)

// aFuzzTarget is a package holding one fuzz target that always passes, so the
// script has something real to run.
func aFuzzTarget() map[string]string {
	return map[string]string{
		"go.mod": "module example.com/fuzzfixture\n\ngo 1.27\n",
		"internal/parser/parser.go": "// Package parser is a fixture package.\npackage parser\n\n" +
			"// Length is the length of the text.\nfunc Length(text string) int {\n\treturn len(text)\n}\n",
		"internal/parser/parser_test.go": "package parser\n\nimport \"testing\"\n\n" +
			"func FuzzLength(f *testing.F) {\n\tf.Add(\"hello\")\n" +
			"\tf.Fuzz(func(t *testing.T, text string) {\n\t\tif Length(text) < 0 {\n\t\t\tt.Fatal(\"a length went negative\")\n\t\t}\n\t})\n}\n",
	}
}

func TestTheFuzzScriptRunsEveryTargetItFinds(t *testing.T) {
	root := writeFixtureModule(t, aFuzzTarget())
	script := copyScript(t, root, "fuzz.sh")

	printed, code := runScript(t, script, "1s")

	if code != 0 {
		t.Fatalf("the fuzz script failed on a module with one working target, and it exited %d:\n%s", code, printed)
	}
	if !strings.Contains(printed, "FuzzLength") {
		t.Errorf("the script did not say it was fuzzing FuzzLength:\n%s", printed)
	}
}

func TestTheFuzzScriptFailsWhenATestBinaryWillNotBuild(t *testing.T) {
	files := aFuzzTarget()
	files["internal/broken/broken.go"] = "// Package broken is a fixture package.\npackage broken\n"
	files["internal/broken/broken_test.go"] = "package broken\n\nimport \"testing\"\n\n" +
		"func TestNothing(t *testing.T) {\n\tthisFunctionDoesNotExist()\n}\n"
	root := writeFixtureModule(t, files)
	script := copyScript(t, root, "fuzz.sh")

	printed, code := runScript(t, script, "1s")

	if code == 0 {
		t.Fatalf("a package whose test binary does not build was skipped in silence:\n%s", printed)
	}
	if !strings.Contains(printed, "internal/broken") {
		t.Errorf("the failure does not name the package that will not build:\n%s", printed)
	}
}

func TestTheFuzzScriptLooksBehindTheIntegrationTag(t *testing.T) {
	files := aFuzzTarget()
	files["internal/tagged/tagged.go"] = "// Package tagged is a fixture package.\npackage tagged\n\n" +
		"// Size is the size of the text.\nfunc Size(text string) int {\n\treturn len(text)\n}\n"
	files["internal/tagged/tagged_test.go"] = "//go:build integration\n\npackage tagged\n\nimport \"testing\"\n\n" +
		"func FuzzSize(f *testing.F) {\n\tf.Add(\"hello\")\n" +
		"\tf.Fuzz(func(t *testing.T, text string) {\n\t\tif Size(text) < 0 {\n\t\t\tt.Fatal(\"a size went negative\")\n\t\t}\n\t})\n}\n"
	root := writeFixtureModule(t, files)
	script := copyScript(t, root, "fuzz.sh")

	printed, code := runScript(t, script, "1s")

	if code != 0 {
		t.Fatalf("the fuzz script failed on a module with a target behind the integration tag:\n%s", printed)
	}
	if !strings.Contains(printed, "FuzzSize") {
		t.Errorf("the script never found the target behind the integration tag:\n%s", printed)
	}
}
