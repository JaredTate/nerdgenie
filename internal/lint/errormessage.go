package lint

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"
	"unicode"
)

// MinErrorMessageWords is the fewest words an error message may have. Four is
// enough room to say what went wrong and what to do, and "bad input" is not.
const MinErrorMessageWords = 4

// checkErrorMessages reports every error message written as a literal that does
// not say enough: fewer than four words, a capital letter at the start, or
// punctuation at the end. A message built at run time is left alone, because
// this checker cannot read it.
func (inspector *fileInspector) checkErrorMessages() {
	ast.Inspect(inspector.file, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall || len(call.Args) == 0 {
			return true
		}
		if !inspector.errorBuildingCall(call.Fun) {
			return true
		}
		literal, isLiteral := call.Args[0].(*ast.BasicLit)
		if !isLiteral || literal.Kind != token.STRING {
			return true
		}
		message, err := strconv.Unquote(literal.Value)
		if err != nil {
			return true
		}
		inspector.checkOneErrorMessage(literal.Pos(), message)
		return true
	})
}

// errorBuildingCall says whether the call is errors.New or fmt.Errorf, the two
// places an error message is written.
//
// The package is resolved through the file's own imports rather than by the name
// in the source, because an alias, a dot import, or somebody else's package
// called "errors" would otherwise decide the answer.
func (inspector *fileInspector) errorBuildingCall(function ast.Expr) bool {
	switch called := function.(type) {
	case *ast.SelectorExpr:
		name, isName := called.X.(*ast.Ident)
		if !isName {
			return false
		}
		return builtByPackage(inspector.importPathOf(name.Name), called.Sel.Name)
	case *ast.Ident:
		// A dot import brings New and Errorf into the file's own namespace.
		return builtByPackage(inspector.dotImportHolding(called.Name), called.Name)
	default:
		return false
	}
}

// builtByPackage says whether the standard library package at that path builds
// an error with that function.
func builtByPackage(path string, function string) bool {
	return (path == "errors" && function == "New") || (path == "fmt" && function == "Errorf")
}

// importPathOf returns the path the file imports under that name, or an empty
// string when the file imports nothing under it.
func (inspector *fileInspector) importPathOf(name string) string {
	for _, imported := range inspector.file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			continue
		}
		if imported.Name != nil {
			if imported.Name.Name == name {
				return path
			}
			continue
		}
		if lastPart(path) == name {
			return path
		}
	}
	return ""
}

// dotImportHolding returns the path of the dot-imported package that would bring
// this bare function name into the file, or an empty string when there is none.
func (inspector *fileInspector) dotImportHolding(function string) string {
	wanted := ""
	switch function {
	case "New":
		wanted = "errors"
	case "Errorf":
		wanted = "fmt"
	default:
		return ""
	}
	for _, imported := range inspector.file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil || imported.Name == nil || imported.Name.Name != "." {
			continue
		}
		if path == wanted {
			return path
		}
	}
	return ""
}

// lastPart is the final element of an import path, which is the name a file uses
// for it when there is no alias.
func lastPart(path string) string {
	at := strings.LastIndex(path, "/")
	if at < 0 {
		return path
	}
	return path[at+1:]
}

// checkOneErrorMessage holds the three parts of the error-message rule.
func (inspector *fileInspector) checkOneErrorMessage(at token.Pos, message string) {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		inspector.report(at, RuleErrorMessage, "give this error a message saying what went wrong and what to do")
		return
	}
	if unicode.IsUpper([]rune(trimmed)[0]) {
		inspector.report(at, RuleErrorMessage,
			"start this error message with a lowercase letter, because it is often printed after other text")
		return
	}
	if strings.ContainsRune(".!?:;,", rune(trimmed[len(trimmed)-1])) {
		inspector.report(at, RuleErrorMessage,
			"take the punctuation off the end of this error message, because it is often printed inside a longer sentence")
		return
	}
	if words := len(strings.Fields(withoutFormatVerbs(trimmed))); words < MinErrorMessageWords {
		inspector.report(at, RuleErrorMessage,
			fmt.Sprintf("this error message is %d words, so make it at least %d and say what to do about it", words, MinErrorMessageWords))
	}
}

// withoutFormatVerbs takes the format verbs out of a message, because "%s" and
// "%w" are not words a reader can read and a message padded out with them says
// no more than one without them. A doubled percent sign is a real character and
// stays.
func withoutFormatVerbs(message string) string {
	var kept strings.Builder
	runes := []rune(message)
	at := 0
	for at < len(runes) {
		if runes[at] != '%' {
			kept.WriteRune(runes[at])
			at++
			continue
		}
		at++
		if at < len(runes) && runes[at] == '%' {
			kept.WriteRune('%')
			at++
			continue
		}
		for at < len(runes) && !unicode.IsLetter(runes[at]) {
			at++
		}
		if at < len(runes) {
			at++
		}
	}
	return kept.String()
}
