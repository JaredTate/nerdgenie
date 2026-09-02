package gate

import (
	"strings"
	"testing"
)

func TestTheTerminalScreensLowerThresholdCoversItsSubPackages(t *testing.T) {
	files := aFixtureModule()
	// internal/tui and everything under it is tested by a person looking at it,
	// so the gate wants seventy percent rather than ninety.
	for _, folder := range []string{"tui", "tui/screen"} {
		files["internal/"+folder+"/doc.go"] = "// Package screen is a fixture package.\npackage screen\n"
		files["internal/"+folder+"/screen.go"] = "package screen\n\n" +
			"// Draw draws one thing.\nfunc Draw(wide bool) string {\n\tif wide {\n\t\treturn \"wide\"\n\t}\n\treturn \"narrow\"\n}\n\n" +
			"// Clear clears the screen.\nfunc Clear(wide bool) string {\n\tif wide {\n\t\treturn \"cleared wide\"\n\t}\n\treturn \"cleared\"\n}\n"
		files["internal/"+folder+"/screen_test.go"] = "package screen\n\nimport \"testing\"\n\n" +
			"func TestDraw(t *testing.T) {\n\tif Draw(true) != \"wide\" {\n\t\tt.Error(\"the drawing changed\")\n\t}\n" +
			"\tif Draw(false) != \"narrow\" {\n\t\tt.Error(\"the drawing changed\")\n\t}\n" +
			"\tif Clear(true) != \"cleared wide\" {\n\t\tt.Error(\"the clearing changed\")\n\t}\n}\n"
	}
	root := writeFixtureModule(t, files)
	script := copyScript(t, root, "coverage.sh")

	printed, code := runScript(t, script)

	if code != 0 {
		t.Fatalf("the gate failed a terminal screen package above seventy percent:\n%s", printed)
	}
	for _, row := range strings.Split(printed, "\n") {
		if !strings.Contains(row, "internal/tui") {
			continue
		}
		if !strings.Contains(row, "70%") {
			t.Errorf("the row for a terminal screen package asks for the wrong threshold:\n%s", row)
		}
	}
}
