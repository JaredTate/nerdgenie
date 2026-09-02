package lint

import (
	"fmt"
	"go/ast"
	"strings"
	"unicode"
)

// The projects Coeus borrowed designs from. A file whose top comment names one
// of them must also say where the reference file lives, so that the next reader
// can go and look at it.
//
// These are the folder names, because that is what CLAUDE.md teaches a worker to
// write: the folder beside this repository for a project on disk, and the folder
// under docs/reference for one that is not. They are matched with the case
// folded, so a header that writes a project's own capitals is caught too. Prime
// Agent is here twice because its folder uses a hyphen and its name uses a
// space, and neither spelling contains the other.
var borrowedProjects = []string{
	"openclaw", "hermes", "prime-agent", "prime agent", "opencode",
	"zeroclaw", "homerecon", "browser-use", "codex", "moltis",
}

// The two shapes a reference path takes: a project cloned beside this one, or a
// copy kept in this repository.
var referencePathMarkers = []string{"~/Code/", "/home/jared/Code/", "docs/reference/"}

// The vendors' own command-line programs, written the way a comment writes them
// when it says how to run one. Their names are also the names of projects Coeus
// read designs from, so a header saying how to run the program is naming no
// design at all and these are taken out of the header before the projects are
// looked for.
var programInvocations = []string{"codex exec", "claude -p"}

// checkFileLength reports a file longer than the limit.
func (inspector *fileInspector) checkFileLength(source []byte) {
	lines := strings.Count(string(source), "\n")
	if !strings.HasSuffix(string(source), "\n") && len(source) > 0 {
		lines++
	}
	if lines > MaxFileLines {
		inspector.report(inspector.file.Package, RuleFileLength,
			fmt.Sprintf("this file is %d lines and the limit is %d, so split it into files that each do one thing", lines, MaxFileLines))
	}
}

// checkBorrowedHeader reports a top comment that names a borrowed-from project
// without saying where its reference file lives.
func (inspector *fileInspector) checkBorrowedHeader() {
	header := inspector.topComment()
	if header == nil {
		return
	}
	text := header.Text()
	folded := strings.ToLower(text)
	for _, invocation := range programInvocations {
		folded = strings.ReplaceAll(folded, invocation, " ")
	}
	named := ""
	for _, project := range borrowedProjects {
		if strings.Contains(folded, project) {
			named = project
			break
		}
	}
	if named == "" {
		return
	}
	for _, marker := range referencePathMarkers {
		if strings.Contains(text, marker) {
			return
		}
	}
	inspector.report(header.Pos(), RuleBorrowedHeader,
		fmt.Sprintf("this header names %s but no reference path, so add the path of the file the design came from", named))
}

// topComment returns the comment group above the package clause, which is where
// a borrowed design is named.
func (inspector *fileInspector) topComment() *ast.CommentGroup {
	if inspector.file.Doc != nil {
		return inspector.file.Doc
	}
	for _, group := range inspector.file.Comments {
		if group.End() < inspector.file.Package {
			return group
		}
	}
	return nil
}

// documented is one comment group and the name it sits above, which is empty
// when the comment sits above something with no name of its own.
type documented struct {
	comment *ast.CommentGroup
	name    string
}

// checkComments reports every comment on a declaration that is not a complete
// sentence.
func (inspector *fileInspector) checkComments() {
	for _, above := range inspector.declarationComments() {
		inspector.checkOneComment(above)
	}
}

// checkOneComment holds the sentence rule for one comment group.
//
// A doc comment that begins with the name it documents is a sentence starting
// with a proper noun, so a lowercase name is accepted there and nowhere else.
// That is Go's own convention, and breaking it would make "go doc" read wrong.
func (inspector *fileInspector) checkOneComment(above documented) {
	group := above.comment
	if group == nil {
		return
	}
	text := strings.TrimSpace(sentenceText(group))
	if text == "" {
		return
	}
	first := []rune(text)[0]
	namesItself := above.name != "" && (text == above.name || strings.HasPrefix(text, above.name+" "))
	if unicode.IsLower(first) && !namesItself {
		inspector.report(group.Pos(), RuleCommentSentence,
			"start this comment with a capital letter or with the name it documents, because a comment is a complete sentence")
		return
	}
	last := text[len(text)-1]
	if last != '.' && last != '?' && last != '!' {
		inspector.report(group.Pos(), RuleCommentSentence,
			"end this comment with a full stop, a question mark, or an exclamation mark, because a comment is a complete sentence")
	}
}

// sentenceText returns the part of a comment group the sentence rule reads,
// leaving out build directives, nolint lines, and lines that are only a link.
func sentenceText(group *ast.CommentGroup) string {
	kept := []string{}
	for _, comment := range group.List {
		line := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
		line = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "/*"), "*/"))
		if line == "" || strings.HasPrefix(line, "go:") || strings.HasPrefix(line, "nolint") {
			continue
		}
		if strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, " ")
}

// declarationComments returns every comment group the sentence rule applies to:
// the ones attached to a declaration, a specification, or a field, each with the
// name it documents.
func (inspector *fileInspector) declarationComments() []documented {
	found := []documented{{comment: inspector.file.Doc, name: "Package"}}
	ast.Inspect(inspector.file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.FuncDecl:
			found = append(found, documented{typed.Doc, typed.Name.Name})
		case *ast.GenDecl:
			found = append(found, documented{typed.Doc, onlySpecName(typed)})
		case *ast.ValueSpec:
			found = append(found, documented{typed.Doc, firstName(typed.Names)})
		case *ast.TypeSpec:
			found = append(found, documented{typed.Doc, typed.Name.Name})
		case *ast.Field:
			// A field carries two comments: the one above it and the one on the end
			// of its line. Both are read by whoever reads the field, so both are
			// held to the sentence rule.
			found = append(found, documented{typed.Doc, firstName(typed.Names)})
			found = append(found, documented{typed.Comment, firstName(typed.Names)})
		}
		return true
	})
	return found
}

// onlySpecName returns the name a one-specification declaration declares,
// because the parser puts that declaration's comment on the block rather than on
// the specification inside it.
func onlySpecName(declaration *ast.GenDecl) string {
	if len(declaration.Specs) != 1 {
		return ""
	}
	switch typed := declaration.Specs[0].(type) {
	case *ast.TypeSpec:
		return typed.Name.Name
	case *ast.ValueSpec:
		return firstName(typed.Names)
	default:
		return ""
	}
}

// firstName returns the first of a list of names, or an empty string when the
// list is empty.
func firstName(names []*ast.Ident) string {
	if len(names) == 0 {
		return ""
	}
	return names[0].Name
}
