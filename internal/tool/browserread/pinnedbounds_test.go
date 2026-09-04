package browserread_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/tool/browserread"
)

// TestEveryBoundOfThePageTextIsTheNumberItSays writes each bound out as the
// literal it is, with the reason it is that number beside it.
func TestEveryBoundOfThePageTextIsTheNumberItSays(t *testing.T) {
	numbers := []struct {
		name string
		is   int
		want int
		why  string
	}{
		{"MaxElements", browserread.MaxElements, 200,
			"two hundred elements is a page the model can still act on, and a longer one is a page to narrow before acting"},
		{"MaxNameRunes", browserread.MaxNameRunes, 120,
			"a page can call an element anything at all, and a hundred and twenty characters names one without filling the window"},
		{"MaxAddressRunes", browserread.MaxAddressRunes, 500,
			"a web address is longer than a name and the model acts on it, so it is shown whole up to five hundred characters"},
		{"MaxIntentRunes", browserread.MaxIntentRunes, 300,
			"the line saying what a step is for is read by a person in a preview, so it fits on a line"},
	}
	for _, number := range numbers {
		if number.is != number.want {
			t.Errorf("%s is %d, and the number this package is built on is %d, because %s",
				number.name, number.is, number.want, number.why)
		}
	}
}

func TestAPageOfMoreThanTwoHundredElementsIsCutAndSaysHowManyAreLeft(t *testing.T) {
	elements := make([]contract.Element, 0, 205)
	for at := 1; at <= 205; at++ {
		elements = append(elements, contract.Element{Ref: fmt.Sprintf("e%d", at), Role: "button", Name: "Press me"})
	}

	text := browserread.PageText(contract.Snapshot{URL: "https://fixture.test/", Title: "Many", Elements: elements})

	if strings.Contains(text, "e201 ") {
		t.Errorf("the two hundred and first element was listed, and the cap is two hundred")
	}
	if !strings.Contains(text, "5 more elements") {
		t.Errorf("the listing was cut without saying how many elements were left out")
	}
}
