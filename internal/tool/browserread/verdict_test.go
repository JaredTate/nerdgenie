package browserread_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/browserread"
)

// TestAChangeThatMissedItsWordsButChangedThePageIsNotARefusal: on run 23 a
// correct click on cell 1 came back "not what was expected / what happened
// instead: the text now says O to move", because the model wrote "O's turn"
// and the worker matches words, and the model re-read the page and replayed
// the game on that. When the page changed, the result says what changed and
// passes no verdict; "not what was expected" is kept for a page that did not
// change at all, and for one that ran into a wall.
func TestAChangeThatMissedItsWordsButChangedThePageIsNotARefusal(t *testing.T) {
	changed := browserread.ChangeText(contract.Diff{
		URL: testkit.FixtureSimplePage, ExpectationMet: false, Settled: true,
		Seen:     `the text now says "O to move"`,
		Snapshot: contract.Snapshot{URL: testkit.FixtureSimplePage, Title: "Tic Tac Toe"},
	})
	if !strings.Contains(changed, `the page changed: the text now says "O to move"`) || strings.Contains(changed, "not what was expected") {
		t.Errorf("a change that missed the model's words reads as a refusal:\n%s", changed)
	}
	unchanged := browserread.ChangeText(contract.Diff{
		URL: testkit.FixtureSimplePage, ExpectationMet: false, Settled: true, Seen: "nothing changed",
		Snapshot: contract.Snapshot{URL: testkit.FixtureSimplePage, Title: "Tic Tac Toe"},
	})
	if !strings.Contains(unchanged, "not what was expected\nwhat happened instead: nothing changed") {
		t.Errorf("a page that did not change lost its refusal:\n%s", unchanged)
	}
	walled := browserread.ChangeText(contract.Diff{
		URL: testkit.FixtureSimplePage, ExpectationMet: false, Settled: true, Seen: "the page is showing a login wall",
		Wall:     &contract.Wall{Kind: contract.WallLogin, Detail: "a password box named Password"},
		Snapshot: contract.Snapshot{URL: testkit.FixtureSimplePage, Title: "Sign in"},
	})
	if !strings.Contains(walled, "not what was expected") {
		t.Errorf("a wall lost its refusal:\n%s", walled)
	}
}

// TestAChangeWithNoNewElementsLeavesTheOutlineOut: every click on run 23
// handed back the whole outline again, twelve buttons and the page's text.
// When no element was added the refs did not move, so the result carries the
// verdict, what changed, the page's errors, and one line saying the outline
// is unchanged; when elements were added the whole outline follows as before.
func TestAChangeWithNoNewElementsLeavesTheOutlineOut(t *testing.T) {
	page := contract.Snapshot{
		URL: testkit.FixtureSimplePage, Title: "Tic Tac Toe", TabID: "t1",
		Elements: []contract.Element{{Ref: "e1", Role: "button", Name: "cell 1"}, {Ref: "e2", Role: "button", Name: "cell 2"}},
		Text:     "TIC TAC TOE\nX to move", Errors: []string{"Uncaught TypeError: x is not a function"},
	}
	same := browserread.ChangeText(contract.Diff{URL: page.URL, ExpectationMet: true, Settled: true, Snapshot: page})
	if !strings.HasSuffix(same, browserread.TheOutlineUnchangedLine) || strings.Contains(same, `e1 button "cell 1"`) || strings.Contains(same, "text on the page") {
		t.Errorf("a change that added no element still carries the outline:\n%s", same)
	}
	if !strings.Contains(same, "Uncaught TypeError") {
		t.Errorf("the page's errors were left out of the change:\n%s", same)
	}
	added := browserread.ChangeText(contract.Diff{
		URL: page.URL, ExpectationMet: true, Settled: true, Snapshot: page,
		NewElements: []contract.Element{{Ref: "e2", Role: "button", Name: "cell 2", New: true}},
	})
	if !strings.Contains(added, "---\n") || !strings.Contains(added, `e1 button "cell 1"`) || strings.Contains(added, browserread.TheOutlineUnchangedLine) {
		t.Errorf("a change that added an element does not carry the whole outline:\n%s", added)
	}
}
