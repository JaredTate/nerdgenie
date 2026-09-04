package search_test

import (
	"testing"

	"github.com/JaredTate/nerdgenie/internal/tool/search"
)

// TestEveryBoundOfTheSearchToolIsTheNumberItSays writes each bound out as the
// literal it is, with the reason it is that number beside it, so that changing
// one changes a test and whoever changes it has to say why.
func TestEveryBoundOfTheSearchToolIsTheNumberItSays(t *testing.T) {
	numbers := []struct {
		name string
		is   int
		want int
		why  string
	}{
		{"MaxRows", search.MaxRows, 50,
			"a search is a question, and a question with more than fifty answers has not been asked properly"},
		{"MaxPatternRunes", search.MaxPatternRunes, 500,
			"five hundred characters is a long regular expression, and a longer one is a model pasting a file into the pattern"},
		{"MaxFilesWalked", search.MaxFilesWalked, 20000,
			"twenty thousand files is a large repository walked in a second or two, and a bigger tree wants a narrower path"},
		{"MaxFileBytes", search.MaxFileBytes, 2 << 20,
			"a file of more than two megabytes is a log or a build product rather than something with a line worth reading in it"},
		{"MaxRowRunes", search.MaxRowRunes, 300,
			"three hundred characters is enough of a matching line to know whether it is the one, and fifty rows of them still fit in a result"},
	}
	for _, number := range numbers {
		if number.is != number.want {
			t.Errorf("%s is %d, and the number this package is built on is %d, because %s",
				number.name, number.is, number.want, number.why)
		}
	}
}
