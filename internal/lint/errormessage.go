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
		if !errorBuildingCall(call.Fun) {
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
func errorBuildingCall(function ast.Expr) bool {
	selector, isSelector := function.(*ast.SelectorExpr)
	if !isSelector {
		return false
	}
	packageName, isName := selector.X.(*ast.Ident)
	if !isName {
		return false
	}
	if packageName.Name == "errors" && selector.Sel.Name == "New" {
		return true
	}
	return packageName.Name == "fmt" && selector.Sel.Name == "Errorf"
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
	if words := len(strings.Fields(trimmed)); words < MinErrorMessageWords {
		inspector.report(at, RuleErrorMessage,
			fmt.Sprintf("this error message is %d words, so make it at least %d and say what to do about it", words, MinErrorMessageWords))
	}
}
