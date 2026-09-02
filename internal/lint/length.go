package lint

import (
	"fmt"
	"go/ast"
)

// checkFunctionLengths reports every function body longer than the limit. A
// function that does not fit on three screens is doing more than one thing.
func (inspector *fileInspector) checkFunctionLengths() {
	ast.Inspect(inspector.file, func(node ast.Node) bool {
		declaration, isFunction := node.(*ast.FuncDecl)
		if !isFunction || declaration.Body == nil {
			return true
		}
		opened := inspector.positions.Position(declaration.Body.Lbrace).Line
		closed := inspector.positions.Position(declaration.Body.Rbrace).Line
		lines := closed - opened - 1
		if lines > MaxFunctionLines {
			inspector.report(declaration.Pos(), RuleFunctionLength,
				fmt.Sprintf("%s is %d lines and the limit is %d, so split out the part that has its own name",
					declaration.Name.Name, lines, MaxFunctionLines))
		}
		return true
	})
}
