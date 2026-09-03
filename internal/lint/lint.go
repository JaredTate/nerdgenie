package lint

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// The names of the seven rules. A violation always names one of them, so that a
// reader can look the rule up in this package's doc comment.
const (
	// RuleCommentSentence covers comments on declarations.
	RuleCommentSentence = "comment-sentence"
	// RuleDocComment covers exported names with no doc comment, and packages
	// with no doc.go file.
	RuleDocComment = "doc-comment"
	// RuleFunctionLength covers function bodies past sixty lines.
	RuleFunctionLength = "function-length"
	// RuleFileLength covers files past five hundred lines.
	RuleFileLength = "file-length"
	// RuleErrorMessage covers error messages that do not say enough.
	RuleErrorMessage = "error-message"
	// RuleIdentifierName covers bare letters and jargon abbreviations.
	RuleIdentifierName = "identifier-name"
	// RuleBorrowedHeader covers a borrowed design named without its reference
	// path.
	RuleBorrowedHeader = "borrowed-design-header"
	// RuleParseError covers a file the checker cannot read at all, which is a
	// file the compiler will refuse too.
	RuleParseError = "parse-error"
)

// The two limits from the definition of done in docs/WORK_PLAN.md.
const (
	// MaxFunctionLines is the longest a function body may be.
	MaxFunctionLines = 60
	// MaxFileLines is the longest a file may be.
	MaxFileLines = 500
	// MaxSourceFileBytes is the most of one file the checker will read. Rule 4
	// caps a file at five hundred lines, so a megabyte is far more than any file
	// this checker should ever meet, and a file past it is refused by name rather
	// than read into memory whole.
	MaxSourceFileBytes = 1 << 20
)

// Violation is one style rule broken at one place, with what to do about it.
type Violation struct {
	// Path is the file it happened in.
	Path string
	// Line is the line it happened on.
	Line int
	// Rule is which of the seven rules was broken.
	Rule string
	// Advice says what to do, in one sentence.
	Advice string
}

// String prints the violation as "path:line: rule: what to do", which is the
// shape an editor can jump to.
func (violation Violation) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", violation.Path, violation.Line, violation.Rule, violation.Advice)
}

// CheckSource checks one Go file and returns every violation in it. Source the
// parser cannot read is one violation of its own, naming the file: the compiler
// will refuse it too, and a checker that said nothing would let a broken file
// through the gate in silence.
func CheckSource(path string, source []byte) []Violation {
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, path, source, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return []Violation{{
			Path:   path,
			Line:   parseErrorLine(err),
			Rule:   RuleParseError,
			Advice: fmt.Sprintf("fix the syntax the compiler will refuse too: %v", err),
		}}
	}
	inspector := &fileInspector{
		path:      path,
		positions: positions,
		file:      file,
		isTest:    strings.HasSuffix(path, "_test.go"),
	}
	inspector.checkFileLength(source)
	inspector.checkBorrowedHeader()
	inspector.checkComments()
	inspector.checkDocComments()
	inspector.checkFunctionLengths()
	inspector.checkErrorMessages()
	inspector.checkIdentifierNames()
	slices.SortStableFunc(inspector.found, func(left, right Violation) int { return left.Line - right.Line })
	return inspector.found
}

// parseErrorLine is the line the parser gave up on, or the first line when it
// did not say.
func parseErrorLine(err error) int {
	var found scanner.ErrorList
	if errors.As(err, &found) && len(found) > 0 {
		return found[0].Pos.Line
	}
	return 1
}

// CheckPackage checks every Go file in one folder, and adds the one rule that
// needs the whole folder: a package that has any file other than a test must
// have a doc.go whose first sentence begins with "Package" and the package name.
func CheckPackage(folder string) ([]Violation, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, fmt.Errorf("cannot read the folder %s to check it: %w", folder, err)
	}

	found := []Violation{}
	sourceFiles := 0
	hasDocFile := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		path := filepath.Join(folder, entry.Name())
		source, err := readSourceFile(path, entry)
		if err != nil {
			return nil, err
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			sourceFiles++
		}
		if entry.Name() == "doc.go" {
			hasDocFile = true
		}
		found = append(found, CheckSource(path, source)...)
	}

	if sourceFiles > 0 && !hasDocFile {
		found = append(found, Violation{
			Path:   filepath.Join(folder, "doc.go"),
			Line:   1,
			Rule:   RuleDocComment,
			Advice: "add a doc.go whose first sentence begins with \"Package\" and the package name, then a paragraph a new reader could understand",
		})
	}
	return found, nil
}

// readSourceFile reads one file, refusing anything past the cap by name rather
// than holding it in memory whole.
func readSourceFile(path string, entry os.DirEntry) ([]byte, error) {
	about, err := entry.Info()
	if err != nil {
		return nil, fmt.Errorf("cannot look at the file %s to check it: %w", path, err)
	}
	if about.Size() > MaxSourceFileBytes {
		return nil, fmt.Errorf("the file %s is %d bytes and the cap is %d, so split it before the checker reads it",
			path, about.Size(), MaxSourceFileBytes)
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read the file %s to check it: %w", path, err)
	}
	return source, nil
}

// CheckTree walks a folder and checks every package under it. It skips the
// folders no style rule belongs in: testdata, which holds files that are wrong
// on purpose, vendored code, and anything whose name begins with a dot.
func CheckTree(root string) ([]Violation, error) {
	if _, err := os.Stat(root); err != nil {
		return nil, fmt.Errorf("cannot check the folder %s, because it is not there: %w", root, err)
	}

	found := []Violation{}
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if skippedFolder(entry.Name()) && path != root {
			return filepath.SkipDir
		}
		violations, err := CheckPackage(path)
		if err != nil {
			return err
		}
		found = append(found, violations...)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("cannot walk the folder %s to check it: %w", root, walkErr)
	}
	slices.SortStableFunc(found, func(left, right Violation) int {
		if left.Path != right.Path {
			return strings.Compare(left.Path, right.Path)
		}
		return left.Line - right.Line
	})
	return found, nil
}

// skippedFolder says whether a folder holds anything the style rules apply to.
func skippedFolder(name string) bool {
	if strings.HasPrefix(name, ".") && name != "." {
		return true
	}
	return name == "testdata" || name == "vendor" || name == "node_modules" || name == "bin" || name == "dist"
}

// fileInspector carries the one file being checked and the violations found in
// it, so that each rule can be a short method rather than a long function with
// six return values.
type fileInspector struct {
	path      string
	positions *token.FileSet
	file      *ast.File
	isTest    bool
	found     []Violation
}

// report adds one violation at a position in the file.
func (inspector *fileInspector) report(at token.Pos, rule string, advice string) {
	inspector.found = append(inspector.found, Violation{
		Path:   inspector.path,
		Line:   inspector.positions.Position(at).Line,
		Rule:   rule,
		Advice: advice,
	})
}
