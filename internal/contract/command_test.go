package contract_test

import (
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

func TestAnUnknownCommandHasOneSentinel(t *testing.T) {
	if contract.ErrNoSuchCommand == nil || contract.ErrNoSuchCommand.Error() == "" {
		t.Fatal("the no-such-command sentinel is missing or empty")
	}
}
