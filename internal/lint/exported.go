package lint

import (
	"fmt"
	"go/ast"
	"strings"
)

// checkDocComments reports every exported name with no doc comment, and a doc.go
// whose first sentence does not begin with "Package" and the package name. Test
// files are left alone, because a test's name is its own documentation.
func (inspector *fileInspector) checkDocComments() {
	if inspector.isTest {
		return
	}
	inspector.checkPackageSentence()
	for _, declaration := range inspector.file.Decls {
		switch typed := declaration.(type) {
		case *ast.FuncDecl:
			inspector.wantDoc(typed.Name, typed.Doc, "function or method")
		case *ast.GenDecl:
			inspector.checkGeneralDeclaration(typed)
		}
	}
}

// checkPackageSentence holds the rule that a doc.go names its own package in its
// first sentence, which is what makes "go doc" read properly.
func (inspector *fileInspector) checkPackageSentence() {
	if !strings.HasSuffix(inspector.path, "doc.go") {
		return
	}
	wanted := "Package " + inspector.file.Name.Name
	if inspector.file.Doc == nil || !strings.HasPrefix(strings.TrimSpace(sentenceText(inspector.file.Doc)), wanted) {
		inspector.report(inspector.file.Package, RuleDocComment,
			fmt.Sprintf("begin this file's comment with %q, so that the package reads properly in go doc", wanted))
	}
}

// checkGeneralDeclaration walks a const, var, or type block and asks for a doc
// comment on every exported name in it, and on every exported struct field and
// interface method inside a type.
func (inspector *fileInspector) checkGeneralDeclaration(declaration *ast.GenDecl) {
	for _, specification := range declaration.Specs {
		switch typed := specification.(type) {
		case *ast.ValueSpec:
			for _, name := range typed.Names {
				inspector.wantDoc(name, firstComment(typed.Doc, declaration.Doc), "constant or variable")
			}
		case *ast.TypeSpec:
			inspector.wantDoc(typed.Name, firstComment(typed.Doc, declaration.Doc), "type")
			inspector.checkTypeMembers(typed)
		}
	}
}

// checkTypeMembers asks for a doc comment on every exported struct field and
// every interface method, because those are read far more often than they are
// written.
func (inspector *fileInspector) checkTypeMembers(specification *ast.TypeSpec) {
	var fields *ast.FieldList
	switch typed := specification.Type.(type) {
	case *ast.StructType:
		fields = typed.Fields
	case *ast.InterfaceType:
		fields = typed.Methods
	default:
		return
	}
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		for _, name := range field.Names {
			inspector.wantDoc(name, field.Doc, "field or method")
		}
	}
}

// wantDoc reports an exported name whose doc comment is missing or says nothing.
func (inspector *fileInspector) wantDoc(name *ast.Ident, doc *ast.CommentGroup, what string) {
	if name == nil || !name.IsExported() {
		return
	}
	if doc != nil && strings.TrimSpace(sentenceText(doc)) != "" {
		return
	}
	inspector.report(name.Pos(), RuleDocComment,
		fmt.Sprintf("write a doc comment above the exported %s %s, beginning with its name", what, name.Name))
}

// firstComment returns the specification's own comment when it has one, and the
// block's comment otherwise, which is how a single-name const block is
// documented by the line above the block.
func firstComment(own *ast.CommentGroup, block *ast.CommentGroup) *ast.CommentGroup {
	if own != nil {
		return own
	}
	return block
}
