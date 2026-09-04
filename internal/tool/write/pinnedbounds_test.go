package write_test

import (
	"testing"

	"github.com/JaredTate/nerdgenie/internal/tool/write"
)

// TestEveryBoundOfTheWriteToolIsTheNumberItSays writes each bound out as the
// literal it is, with the reason it is that number beside it, so that changing
// one changes a test and whoever changes it has to say why.
func TestEveryBoundOfTheWriteToolIsTheNumberItSays(t *testing.T) {
	numbers := []struct {
		name string
		is   int
		want int
		why  string
	}{
		{"MaxContentBytes", write.MaxContentBytes, 4 << 20,
			"four megabytes is far more than a model writes in one call on purpose, and a cap is cheaper to explain than a full disk"},
		{"MaxPriorContentsBytes", write.MaxPriorContentsBytes, 4 << 20,
			"what a file held is kept in the log so that undo can put it back, and it is kept up to the same four megabytes one write may put there"},
	}
	for _, number := range numbers {
		if number.is != number.want {
			t.Errorf("%s is %d, and the number this package is built on is %d, because %s",
				number.name, number.is, number.want, number.why)
		}
	}
}
