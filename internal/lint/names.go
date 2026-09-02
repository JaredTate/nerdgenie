package lint

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"
	"strings"
)

// The one-letter names that say enough on their own, in the two places Go's own
// convention puts them: the testing handle, a byte buffer, a file, a reader, and
// a writer. Brief 0.1 allows them "in tests and handlers", so they are allowed in
// a test file and inside a request handler, and nowhere else. Every other bare
// letter has to earn its place by being a receiver or a loop index.
var allowedOneLetterNames = []string{"t", "b", "f", "r", "w"}

// The abbreviations that say less than the word they came from. "ctx" and "err"
// are not here, because Go writes them everywhere and every reader knows them.
var bannedAbbreviations = []string{"cfg", "msg", "req", "resp", "buf", "tmp", "str", "num", "val"}

// checkIdentifierNames reports every declared name that is a bare letter with no
// excuse, or one of the abbreviations that says less than the word it came from.
func (inspector *fileInspector) checkIdentifierNames() {
	exempt := inspector.namesWithAnExcuse()
	ast.Inspect(inspector.file, func(node ast.Node) bool {
		for _, name := range declaredNames(node) {
			inspector.checkOneName(name, exempt)
		}
		return true
	})
}

// checkOneName holds the two halves of the identifier rule for one name.
func (inspector *fileInspector) checkOneName(name *ast.Ident, exempt map[*ast.Ident]bool) {
	if name == nil || name.Name == "_" {
		return
	}
	if slices.Contains(bannedAbbreviations, strings.ToLower(name.Name)) {
		inspector.report(name.Pos(), RuleIdentifierName,
			fmt.Sprintf("rename %q to the whole word it stands for, because an identifier says what it is", name.Name))
		return
	}
	if len(name.Name) != 1 || exempt[name] {
		return
	}
	inspector.report(name.Pos(), RuleIdentifierName,
		fmt.Sprintf("give %q a name that says what it holds, because one letter says nothing", name.Name))
}

// namesWithAnExcuse collects the bare letters that are allowed: a method's
// receiver, a name bound by a loop, and the conventional letters inside a test
// file or a request handler.
func (inspector *fileInspector) namesWithAnExcuse() map[*ast.Ident]bool {
	exempt := map[*ast.Ident]bool{}
	ast.Inspect(inspector.file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.FuncDecl:
			if typed.Recv != nil {
				for _, field := range typed.Recv.List {
					markNames(exempt, field.Names)
				}
			}
			if isTestOrHandler(typed.Type) {
				markConventionalNames(exempt, typed)
			}
		case *ast.FuncLit:
			if isTestOrHandler(typed.Type) {
				markConventionalNames(exempt, typed)
			}
		case *ast.ForStmt:
			if assignment, isAssignment := typed.Init.(*ast.AssignStmt); isAssignment {
				markExpressions(exempt, assignment.Lhs)
			}
		case *ast.RangeStmt:
			if typed.Tok == token.DEFINE {
				markExpressions(exempt, []ast.Expr{typed.Key, typed.Value})
			}
		}
		return true
	})
	if inspector.isTest {
		markConventionalNames(exempt, inspector.file)
	}
	return exempt
}

// The parameter types that make a function a test or a handler: the testing
// handles, which are what t, b and f are named after, and the response writer,
// which is where Go's own convention calls the writer w and the request r.
var conventionalParameterTypes = []string{
	"testing.TB", "*testing.T", "*testing.B", "*testing.F", "http.ResponseWriter",
}

// isTestOrHandler says whether the signature is a test helper's or an HTTP
// handler's, which are the two places brief 0.1 allows the conventional
// one-letter names. A helper that takes a testing handle is test code wherever
// its file happens to live.
func isTestOrHandler(signature *ast.FuncType) bool {
	if signature == nil || signature.Params == nil {
		return false
	}
	for _, field := range signature.Params.List {
		if slices.Contains(conventionalParameterTypes, typeName(field.Type)) {
			return true
		}
	}
	return false
}

// typeName renders the simple forms of a type: a plain name, a name from another
// package, and a pointer to either.
func typeName(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.StarExpr:
		return "*" + typeName(typed.X)
	case *ast.SelectorExpr:
		if name, isName := typed.X.(*ast.Ident); isName {
			return name.Name + "." + typed.Sel.Name
		}
	case *ast.Ident:
		return typed.Name
	}
	return ""
}

// markConventionalNames marks every conventional one-letter name anywhere inside
// the node, which is what makes t, b, f, r, and w allowed in a test file and in a
// handler.
func markConventionalNames(exempt map[*ast.Ident]bool, inside ast.Node) {
	ast.Inspect(inside, func(node ast.Node) bool {
		name, isName := node.(*ast.Ident)
		if isName && slices.Contains(allowedOneLetterNames, name.Name) {
			exempt[name] = true
		}
		return true
	})
}

// declaredNames returns the names one node declares, which is the only place the
// identifier rule looks: a use of a name somewhere else is not a naming choice.
func declaredNames(node ast.Node) []*ast.Ident {
	switch typed := node.(type) {
	case *ast.FuncDecl:
		return []*ast.Ident{typed.Name}
	case *ast.Field:
		return typed.Names
	case *ast.ValueSpec:
		return typed.Names
	case *ast.TypeSpec:
		return []*ast.Ident{typed.Name}
	case *ast.AssignStmt:
		if typed.Tok == token.DEFINE {
			return identifiersIn(typed.Lhs)
		}
	case *ast.RangeStmt:
		if typed.Tok == token.DEFINE {
			return identifiersIn([]ast.Expr{typed.Key, typed.Value})
		}
	}
	return nil
}

// identifiersIn picks the plain names out of a list of expressions.
func identifiersIn(expressions []ast.Expr) []*ast.Ident {
	names := []*ast.Ident{}
	for _, expression := range expressions {
		if name, isName := expression.(*ast.Ident); isName {
			names = append(names, name)
		}
	}
	return names
}

// markNames adds every name to the exempt set.
func markNames(exempt map[*ast.Ident]bool, names []*ast.Ident) {
	for _, name := range names {
		exempt[name] = true
	}
}

// markExpressions adds every plain name among the expressions to the exempt set.
func markExpressions(exempt map[*ast.Ident]bool, expressions []ast.Expr) {
	markNames(exempt, identifiersIn(expressions))
}
