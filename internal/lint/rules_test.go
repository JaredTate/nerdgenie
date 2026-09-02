package lint_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/lint"
)

// checkOneFile runs the checker over a snippet and returns the rules it
// reported, so that a table can say what each snippet should produce.
func checkOneFile(source string) []string {
	rules := []string{}
	for _, violation := range lint.CheckSource("example.go", []byte(source)) {
		rules = append(rules, violation.Rule)
	}
	return rules
}

func TestCommentSentenceRuleAcceptsSentencesAndDirectives(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		reported bool
	}{
		{"a complete sentence", "package example\n\n// Count returns nothing.\nfunc Count() int { return 0 }\n", false},
		{"a question", "package example\n\n// Count returns what?\nfunc Count() int { return 0 }\n", false},
		{"starting lowercase", "package example\n\n// count returns nothing.\nfunc Count() int { return 0 }\n", true},
		{"no full stop", "package example\n\n// Count returns nothing\nfunc Count() int { return 0 }\n", true},
		{"a build directive", "//go:build example\n\npackage example\n", false},
		{"a nolint line", "package example\n\n//nolint\nfunc Count() int { return 0 }\n", true},
		{"a link on its own line", "package example\n\n// Count returns nothing.\n// https://example.com/notes\nfunc Count() int { return 0 }\n", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reported := containsRule(checkOneFile(test.source), lint.RuleCommentSentence)
			if reported != test.reported {
				t.Errorf("the comment-sentence rule reported %v, want %v, for:\n%s", reported, test.reported, test.source)
			}
		})
	}
}

func TestDocCommentRuleCoversFunctionsTypesFieldsAndMethods(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		reported bool
	}{
		{"a documented function", "package example\n\n// Count returns nothing.\nfunc Count() int { return 0 }\n", false},
		{"an undocumented function", "package example\n\nfunc Count() int { return 0 }\n", true},
		{"an unexported function needs nothing", "package example\n\nfunc count() int { return 0 }\n", false},
		{"an undocumented type", "package example\n\ntype Counter struct{}\n", true},
		{"an undocumented struct field", "package example\n\n// Counter counts.\ntype Counter struct {\n\tTotal int\n}\n", true},
		{"an undocumented interface method", "package example\n\n// Counter counts.\ntype Counter interface {\n\tCount() int\n}\n", true},
		{"an undocumented constant", "package example\n\nconst Greeting = \"hello\"\n", true},
		{"a constant documented by its block", "package example\n\n// Greeting is a greeting.\nconst (\n\tGreeting = \"hello\"\n)\n", false},
		{"a test file is left alone", "package example\n\nfunc TestCount(t int) { _ = t }\n", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := "example.go"
			if strings.HasPrefix(test.name, "a test file") {
				path = "example_test.go"
			}
			rules := []string{}
			for _, violation := range lint.CheckSource(path, []byte(test.source)) {
				rules = append(rules, violation.Rule)
			}
			if containsRule(rules, lint.RuleDocComment) != test.reported {
				t.Errorf("the doc-comment rule reported %v, want %v, for:\n%s", !test.reported, test.reported, test.source)
			}
		})
	}
}

func TestErrorMessageRuleWantsFourLowercaseWordsWithNoFullStop(t *testing.T) {
	build := func(message string) string {
		return "package example\n\nimport \"errors\"\n\n// Fail fails.\nfunc Fail() error { return errors.New(" + message + ") }\n"
	}
	tests := []struct {
		name     string
		source   string
		reported bool
	}{
		{"four lowercase words", build(`"cannot open the vault"`), false},
		{"three words", build(`"cannot open vault"`), true},
		{"starting uppercase", build(`"Cannot open the vault"`), true},
		{"ending in a full stop", build(`"cannot open the vault."`), true},
		{"ending in a colon", build(`"cannot open the vault:"`), true},
		{"a wrapped error at the end", "package example\n\nimport \"fmt\"\n\n// Fail fails.\nfunc Fail(err error) error { return fmt.Errorf(\"cannot open the vault: %w\", err) }\n", false},
		{"a message built at run time", "package example\n\nimport \"errors\"\n\n// Fail fails.\nfunc Fail(text string) error { return errors.New(text) }\n", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reported := containsRule(checkOneFile(test.source), lint.RuleErrorMessage)
			if reported != test.reported {
				t.Errorf("the error-message rule reported %v, want %v, for:\n%s", reported, test.reported, test.source)
			}
		})
	}
}

func TestIdentifierNameRuleRefusesOneLettersAndAbbreviations(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		reported bool
	}{
		{"a full word", "package example\n\n// Count counts.\nfunc Count(text string) int { return len(text) }\n", false},
		{"a loop index", "package example\n\n// Count counts.\nfunc Count() int {\n\ttotal := 0\n\tfor i := 0; i < 3; i++ {\n\t\ttotal += i\n\t}\n\treturn total\n}\n", false},
		{"a range key", "package example\n\n// Count counts.\nfunc Count(list []int) int {\n\ttotal := 0\n\tfor i, v := range list {\n\t\ttotal += i + v\n\t}\n\treturn total\n}\n", false},
		{"a receiver", "package example\n\n// Counter counts.\ntype Counter struct{}\n\n// Count counts.\nfunc (c Counter) Count() int { return 0 }\n", false},
		{"a conventional test name", "package example\n\n// Count counts.\nfunc Count() int {\n\tt := 1\n\treturn t\n}\n", false},
		{"a bare letter", "package example\n\n// Count counts.\nfunc Count() int {\n\tx := 1\n\treturn x\n}\n", true},
		{"a banned abbreviation", "package example\n\n// Count counts.\nfunc Count() int {\n\tcfg := 1\n\treturn cfg\n}\n", true},
		{"ctx is allowed", "package example\n\n// Count counts.\nfunc Count() int {\n\tctx := 1\n\treturn ctx\n}\n", false},
		{"err is allowed", "package example\n\n// Count counts.\nfunc Count() int {\n\terr := 1\n\treturn err\n}\n", false},
		{"a banned abbreviation as a parameter", "package example\n\n// Count counts.\nfunc Count(msg string) int { return len(msg) }\n", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reported := containsRule(checkOneFile(test.source), lint.RuleIdentifierName)
			if reported != test.reported {
				t.Errorf("the identifier-name rule reported %v, want %v, for:\n%s", reported, test.reported, test.source)
			}
		})
	}
}

func TestBorrowedHeaderRuleWantsAPathBesideTheProjectName(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		reported bool
	}{
		{"a project and a path", "// Ported from ZeroClaw's parser at ~/Code/zeroclaw/crates/parser/src/lib.rs.\n\npackage example\n", false},
		{"a project and a reference copy", "// Ported from Moltis at docs/reference/moltis/snapshot.rs.\n\npackage example\n", false},
		{"a project and no path", "// Ported from ZeroClaw's parser, wherever that is.\n\npackage example\n", true},
		{"no project named", "// This file is all our own work.\n\npackage example\n", false},
		{"no header at all", "package example\n", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reported := containsRule(checkOneFile(test.source), lint.RuleBorrowedHeader)
			if reported != test.reported {
				t.Errorf("the borrowed-design rule reported %v, want %v, for:\n%s", reported, test.reported, test.source)
			}
		})
	}
}

func TestThePackageDocRuleWantsTheFirstSentenceToNameThePackage(t *testing.T) {
	rules := []string{}
	for _, violation := range lint.CheckSource("doc.go", []byte("// This file explains the package.\npackage example\n")) {
		rules = append(rules, violation.Rule)
	}
	if !containsRule(rules, lint.RuleDocComment) {
		t.Error("a doc.go whose comment does not begin with \"Package example\" was accepted, want it refused")
	}
}

// containsRule says whether the rule appears in the list.
func containsRule(rules []string, wanted string) bool {
	for _, rule := range rules {
		if rule == wanted {
			return true
		}
	}
	return false
}
