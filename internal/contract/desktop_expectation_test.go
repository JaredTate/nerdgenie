package contract_test

import (
	"context"

	"github.com/JaredTate/coeus/internal/contract"
)

// The five desktop actions each carry what the model expected to happen, which
// is the act-and-assert rule of the design: an action the model cannot say the
// outcome of is an action it should not take. These lines fail to build the
// moment one of them loses its expectation.
var (
	_ func(contract.Desktop, context.Context, string, string) error   = contract.Desktop.Launch
	_ func(contract.Desktop, context.Context, int, string) error      = contract.Desktop.Click
	_ func(contract.Desktop, context.Context, string, string) error   = contract.Desktop.Type
	_ func(contract.Desktop, context.Context, string, string) error   = contract.Desktop.Press
	_ func(contract.Desktop, context.Context, int, int, string) error = contract.Desktop.Drag
)
