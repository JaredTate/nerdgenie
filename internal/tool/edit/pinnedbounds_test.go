package edit_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/tool/edit"
)

// TestEveryBoundOfTheEditToolIsTheNumberItSays writes each bound out as the
// literal it is, with the reason it is that number beside it, so that changing
// one changes a test and whoever changes it has to say why.
func TestEveryBoundOfTheEditToolIsTheNumberItSays(t *testing.T) {
	numbers := []struct {
		name string
		is   int
		want int
		why  string
	}{
		{"MaxFileBytes", edit.MaxFileBytes, 8 << 20,
			"a file of more than eight megabytes is not one a model can hold enough of in mind to quote a span from, and the matchers would walk the whole of it"},
		{"MaxCandidateSpans", edit.MaxCandidateSpans, 200,
			"a matcher that has offered two hundred places has not found one place, and every candidate costs a walk of the file"},
		{"MaxMatchGrowth", edit.MaxMatchGrowth, 3,
			"a forgiving matcher that lands on four times what the model quoted has found something else, and replacing it would be a change nobody asked for"},
	}
	for _, number := range numbers {
		if number.is != number.want {
			t.Errorf("%s is %d, and the number this package is built on is %d, because %s",
				number.name, number.is, number.want, number.why)
		}
	}
}

// TestASpanIsAllowedToGrowThreeTimesAndNoFurther brackets the growth cap by the
// number three rather than by the constant that holds it: nine characters
// quoted may land on a span of thirty-four and not on one of thirty-eight.
func TestASpanIsAllowedToGrowThreeTimesAndNoFurther(t *testing.T) {
	wanted := "let x = 1"

	near := "let" + strings.Repeat(" ", 26) + "x = 1\n"
	if _, _, err := edit.Replace(near, wanted, "let x = 2"); err != nil {
		t.Errorf("a span of %d characters against the %d quoted was refused, and three times over is allowed: %v",
			len(near)-1, len(wanted), err)
	}

	far := "let" + strings.Repeat(" ", 30) + "x = 1\n"
	if _, _, err := edit.Replace(far, wanted, "let x = 2"); err == nil {
		t.Errorf("a span of %d characters against the %d quoted was replaced, and more than three times over is not the edit the model meant",
			len(far)-1, len(wanted))
	}
}
