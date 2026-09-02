package lint_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/lint"
)

// TestOneFixtureFolderPerRuleReportsExactlyThatRule is the acceptance test the
// brief names: a fixture tree with one style violation of each kind fails the
// checker, and the clean folder passes.
func TestOneFixtureFolderPerRuleReportsExactlyThatRule(t *testing.T) {
	tests := []struct {
		folder string
		rule   string
	}{
		{"commentsentence", lint.RuleCommentSentence},
		{"doccomment", lint.RuleDocComment},
		{"packagedoc", lint.RuleDocComment},
		{"functionlength", lint.RuleFunctionLength},
		{"filelength", lint.RuleFileLength},
		{"errormessage", lint.RuleErrorMessage},
		{"identifiername", lint.RuleIdentifierName},
		{"borrowedheader", lint.RuleBorrowedHeader},
	}
	for _, test := range tests {
		t.Run(test.folder, func(t *testing.T) {
			violations, err := lint.CheckPackage(filepath.Join("testdata", test.folder))
			if err != nil {
				t.Fatalf("checking the %s fixture failed: %v", test.folder, err)
			}
			if len(violations) != 1 {
				t.Fatalf("the %s fixture reported %d violations, want exactly one:\n%s",
					test.folder, len(violations), joinViolations(violations))
			}
			if violations[0].Rule != test.rule {
				t.Errorf("the %s fixture reported the rule %q, want %q", test.folder, violations[0].Rule, test.rule)
			}
			if violations[0].Advice == "" {
				t.Error("the violation carries no advice, and every message must say what to do")
			}
			if violations[0].Line < 1 {
				t.Errorf("the violation points at line %d, want a real line number", violations[0].Line)
			}
		})
	}
}

func TestTheCleanFixtureReportsNothing(t *testing.T) {
	violations, err := lint.CheckPackage(filepath.Join("testdata", "clean"))
	if err != nil {
		t.Fatalf("checking the clean fixture failed: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("the clean fixture reported %d violations, want none:\n%s", len(violations), joinViolations(violations))
	}
}

func TestAViolationPrintsAsPathLineRuleAdvice(t *testing.T) {
	violation := lint.Violation{
		Path:   "internal/example/thing.go",
		Line:   12,
		Rule:   lint.RuleFileLength,
		Advice: "split this file, because it is longer than five hundred lines",
	}
	want := "internal/example/thing.go:12: file-length: split this file, because it is longer than five hundred lines"
	if violation.String() != want {
		t.Errorf("a violation prints as %q, want %q", violation.String(), want)
	}
}

func TestTheLimitsAreTheOnesTheWorkPlanNames(t *testing.T) {
	if lint.MaxFunctionLines != 60 {
		t.Errorf("the function-length limit is %d, want 60", lint.MaxFunctionLines)
	}
	if lint.MaxFileLines != 500 {
		t.Errorf("the file-length limit is %d, want 500", lint.MaxFileLines)
	}
}

func TestCheckTreeWalksEveryPackageAndSkipsTestdata(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "good", "doc.go"), "// Package good is clean.\npackage good\n")
	writeFile(t, filepath.Join(root, "good", "testdata", "bad", "bad.go"), "package bad\n\nfunc Count() int { return 0 }\n")
	writeFile(t, filepath.Join(root, "bad", "bad.go"), "package bad\n\nfunc Count() int { return 0 }\n")

	violations, err := lint.CheckTree(root)
	if err != nil {
		t.Fatalf("walking the tree failed: %v", err)
	}
	for _, violation := range violations {
		if strings.Contains(violation.Path, "testdata") {
			t.Errorf("the checker looked inside testdata and reported %s", violation)
		}
	}
	if len(violations) == 0 {
		t.Error("the tree holds a package with no doc.go and an undocumented exported name, and nothing was reported")
	}
}

func TestCheckTreeSaysSoWhenTheFolderIsNotThere(t *testing.T) {
	if _, err := lint.CheckTree(filepath.Join(t.TempDir(), "nowhere")); err == nil {
		t.Fatal("walking a folder that is not there was reported as a success, want an error naming the folder")
	}
}

func TestAFolderOfOnlyTestFilesNeedsNoDocFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "example_test.go"), "package example_test\n\nimport \"testing\"\n\nfunc TestNothing(t *testing.T) { _ = t }\n")

	violations, err := lint.CheckPackage(root)
	if err != nil {
		t.Fatalf("checking a folder of only test files failed: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("a folder holding only test files reported %d violations, want none:\n%s", len(violations), joinViolations(violations))
	}
}

func TestASourceFileOverTheCapIsRefusedRatherThanReadWhole(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "doc.go"), "// Package huge is a fixture package.\npackage huge\n")
	writeFile(t, filepath.Join(root, "huge.go"),
		"package huge\n\n// "+strings.Repeat("a", lint.MaxSourceFileBytes)+"\n")

	_, err := lint.CheckPackage(root)

	if err == nil {
		t.Fatal("a source file over the cap was read whole, and every buffer in Coeus has a cap")
	}
	if !strings.Contains(err.Error(), "huge.go") {
		t.Errorf("the failure does not name the file that is too big: %v", err)
	}
}

// aFunctionOfBodyLines builds a file holding one function whose body is the
// number of lines given.
func aFunctionOfBodyLines(lines int) string {
	body := strings.Repeat("\t_ = 1\n", lines)
	return "package example\n\n// Count counts.\nfunc Count() {\n" + body + "}\n"
}

func TestTheFunctionLengthRuleBitesAtSixtyOneLinesAndNotAtSixty(t *testing.T) {
	if containsRule(checkOneFile(aFunctionOfBodyLines(lint.MaxFunctionLines)), lint.RuleFunctionLength) {
		t.Errorf("a function of exactly %d body lines was reported, and the limit is %d",
			lint.MaxFunctionLines, lint.MaxFunctionLines)
	}
	if !containsRule(checkOneFile(aFunctionOfBodyLines(lint.MaxFunctionLines+1)), lint.RuleFunctionLength) {
		t.Errorf("a function of %d body lines was allowed, and the limit is %d",
			lint.MaxFunctionLines+1, lint.MaxFunctionLines)
	}
}

// aFileOfLines builds a file of exactly the number of lines given.
func aFileOfLines(lines int) string {
	source := "package example\n"
	for at := 1; at < lines; at++ {
		source += "\n"
	}
	return source
}

func TestTheFileLengthRuleBitesAtFiveHundredAndOneLinesAndNotAtFiveHundred(t *testing.T) {
	if containsRule(checkOneFile(aFileOfLines(lint.MaxFileLines)), lint.RuleFileLength) {
		t.Errorf("a file of exactly %d lines was reported, and the limit is %d", lint.MaxFileLines, lint.MaxFileLines)
	}
	if !containsRule(checkOneFile(aFileOfLines(lint.MaxFileLines+1)), lint.RuleFileLength) {
		t.Errorf("a file of %d lines was allowed, and the limit is %d", lint.MaxFileLines+1, lint.MaxFileLines)
	}
}

func TestSourceThatWillNotParseIsReportedByName(t *testing.T) {
	violations := lint.CheckSource("broken.go", []byte("package ???"))

	if len(violations) == 0 {
		t.Fatal("source that will not parse reported nothing, so a broken file passes the gate in silence")
	}
	if violations[0].Rule != lint.RuleParseError {
		t.Errorf("the violation is the rule %q, want %q", violations[0].Rule, lint.RuleParseError)
	}
	if violations[0].Path != "broken.go" {
		t.Errorf("the violation names %q, want the file that will not parse", violations[0].Path)
	}
	if violations[0].Advice == "" {
		t.Error("the violation says nothing about what to do")
	}

	good := lint.CheckSource("fine.go", []byte("package example\n"))
	for _, violation := range good {
		if violation.Rule == lint.RuleParseError {
			t.Errorf("a file that parses was reported as unparseable: %s", violation)
		}
	}
}

func FuzzCheckSource(f *testing.F) {
	seeds := []string{
		"package example\n",
		"package example\n\nfunc Count() int { return 0 }\n",
		"// A header.\npackage example\n",
		"",
		"package example\n\nimport \"errors\"\n\nvar E = errors.New(\"x\")\n",
	}
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, source []byte) {
		for _, violation := range lint.CheckSource("fuzz.go", source) {
			if violation.Rule == "" || violation.Advice == "" {
				t.Fatalf("the checker produced a violation with no rule or no advice: %+v", violation)
			}
		}
	})
}

// joinViolations turns a list of violations into one block of text a failing
// test can print.
func joinViolations(violations []lint.Violation) string {
	lines := make([]string, 0, len(violations))
	for _, violation := range violations {
		lines = append(lines, "  "+violation.String())
	}
	return strings.Join(lines, "\n")
}

// writeFile writes one file and the folders above it, failing the test when it
// cannot.
func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("cannot make the folder for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("cannot write %s: %v", path, err)
	}
}
