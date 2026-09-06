package testkit

import (
	"fmt"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// fixturePicture stands in for a screenshot. It is a real one-pixel PNG, so a
// test that decodes it gets a picture rather than a surprise.
const fixturePicture = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="

// fixturePages returns the five pages the fake browser knows: one with a link
// and a form, the page that link goes to, and the three walls. A wall rides on
// the page itself rather than being worked out later, so that open and read
// report it the way PROTOCOL.md says they do, and so that a test can add a page
// of its own with a wall on it.
func fixturePages() map[string]contract.Snapshot {
	return map[string]contract.Snapshot{
		FixtureSimplePage: {
			URL: FixtureSimplePage, Title: "A simple page", TabID: "t1", BelowFold: 4,
			Elements: []contract.Element{
				{Ref: FixtureChangeLinkRef, Role: "link", Name: "Change the page"},
				{Ref: FixtureUsernameRef, Role: "textbox", Name: "Name"},
				{Ref: FixturePasswordRef, Role: "textbox", Name: "Note"},
			},
		},
		FixtureChangedPage: {
			URL: FixtureChangedPage, Title: "The page changed", TabID: "t1",
			Elements: []contract.Element{
				{Ref: FixtureChangeLinkRef, Role: "link", Name: "Change the page"},
				{Ref: FixtureUsernameRef, Role: "textbox", Name: "Name"},
				{Ref: FixturePasswordRef, Role: "textbox", Name: "Note"},
				{Ref: "e4", Role: "heading", Name: "The page changed"},
				{Ref: "e5", Role: "button", Name: "Go back"},
			},
		},
		FixtureOverlaysPage: {
			URL: FixtureOverlaysPage, Title: "Overlays", TabID: "t1", HiddenYetDrawn: 2,
			Elements: []contract.Element{
				{Ref: "e1", Role: "heading", Name: "Press start"},
				{Ref: "e2", Role: "button", Name: "Start"},
				{Ref: "e3", Role: "heading", Name: "Game over", HiddenYetDrawn: true},
				{Ref: "e4", Role: "button", Name: "Play again", HiddenYetDrawn: true},
			},
		},
		FixtureLoginPage: {
			URL: FixtureLoginPage, Title: "Sign in", TabID: "t1",
			Wall: &contract.Wall{Kind: contract.WallLogin, Detail: "a password box named Password"},
			Elements: []contract.Element{
				{Ref: FixtureUsernameRef, Role: "textbox", Name: "Username"},
				{Ref: FixturePasswordRef, Role: "textbox", Name: "Password"},
				{Ref: "e4", Role: "button", Name: "Sign in"},
			},
		},
		FixtureTwoFactorPage: {
			URL: FixtureTwoFactorPage, Title: "Enter your code", TabID: "t1",
			Wall: &contract.Wall{Kind: contract.WallTwoFactor, Detail: "a box named Verification code"},
			Elements: []contract.Element{
				{Ref: "e5", Role: "textbox", Name: "Verification code"},
				{Ref: "e6", Role: "button", Name: "Verify"},
			},
		},
		FixtureCaptchaPage: {
			URL: FixtureCaptchaPage, Title: "Are you a person?", TabID: "t1",
			Wall: &contract.Wall{Kind: contract.WallCaptcha, Detail: "a checkbox named I am not a robot"},
			Elements: []contract.Element{
				{Ref: "e7", Role: "checkbox", Name: "I am not a robot"},
			},
		},
	}
}

// AddPage puts one more page on the fixture browser, and says where a click on a
// reference goes when the page holds a link to somewhere.
func (worker *FakeBrowserWorker) AddPage(page contract.Snapshot) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	worker.pages[page.URL] = page
}

// LinkGoesTo says that clicking a reference moves to a page.
func (worker *FakeBrowserWorker) LinkGoesTo(ref string, address string) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	worker.links[ref] = address
}

// page is the page the worker is on, and an error when it is on none.
func (worker *FakeBrowserWorker) page() (contract.Snapshot, error) {
	if worker.closed {
		return contract.Snapshot{}, ErrBrowserGone
	}
	if worker.current == "" {
		return contract.Snapshot{}, ErrNoPageOpen
	}
	return worker.pages[worker.current], nil
}

// findable checks that a reference is on the page, finding the element again by
// its role and name when the reference has gone stale, which is what the real
// worker does before it gives up.
func (worker *FakeBrowserWorker) findable(ref string) error {
	page, err := worker.page()
	if err != nil {
		return err
	}
	for _, element := range page.Elements {
		if element.Ref != ref {
			continue
		}
		if worker.stale[ref] {
			worker.stale[ref] = false
			worker.recoveries++
		}
		return nil
	}
	return fmt.Errorf("there is no element %q on this page: %w", ref, ErrNoSuchReference)
}

// actAndSettle does what every action does after the click or the keystroke:
// wait for the page to settle, take a new snapshot, and work out the diff.
//
// A page that keeps changing past the limit is read as it stands and reported
// with settled false, so that a live page such as a chat or a clock stays
// usable. Only a page that cannot be read at all is an error, which the protocol
// answers with -32001.
func (worker *FakeBrowserWorker) actAndSettle(goingTo string, expectation string, aimedAt string) (contract.Diff, error) {
	problem := worker.takeProblem()
	if problem.cannotRead {
		return contract.Diff{}, ErrSettleTimeout
	}
	target := worker.elementNamed(aimedAt)

	before := worker.previous
	moved := false
	if goingTo != "" && goingTo != worker.current && !problem.changeNothing {
		if _, found := worker.pages[goingTo]; found {
			worker.current = goingTo
			moved = true
		}
	}
	page, err := worker.page()
	if err != nil {
		return contract.Diff{}, err
	}

	if problem.dialog != nil {
		worker.openDialog = problem.dialog
	}
	diff := contract.Diff{
		URLChanged:  moved,
		URL:         page.URL,
		NewElements: elementsNotIn(page.Elements, before),
		Dialog:      problem.dialog,
		Download:    problem.download,
		NewTab:      problem.newTab,
		Wall:        page.Wall,
		Settled:     !problem.neverSettle,
		Snapshot:    page,
	}
	worker.previous = page.Elements
	if problem.newTab != "" {
		worker.tabs = append(worker.tabs, contract.Tab{ID: problem.newTab, URL: page.URL, Title: page.Title})
	}

	diff.ExpectationMet = diff.Wall == nil && diff.Settled && meetsExpectation(expectation, diff, target)
	if !diff.ExpectationMet {
		diff.Seen = whatChanged(expectation, diff)
	}
	return diff, nil
}

// elementNamed is the element an action was aimed at, as it stood before the
// action, or the zero value when the action was aimed at no element. The caller
// holds the lock.
func (worker *FakeBrowserWorker) elementNamed(ref string) contract.Element {
	if ref == "" {
		return contract.Element{}
	}
	page, err := worker.page()
	if err != nil {
		return contract.Element{}
	}
	for _, element := range page.Elements {
		if element.Ref == ref {
			return element
		}
	}
	return contract.Element{}
}

// elementsNotIn returns the elements of the new page that were not on the old
// one, each marked as new, which is what tells the model what its action did.
func elementsNotIn(after []contract.Element, before []contract.Element) []contract.Element {
	known := map[string]bool{}
	for _, element := range before {
		known[element.Ref] = true
	}
	fresh := []contract.Element{}
	for _, element := range after {
		if !known[element.Ref] {
			element.New = true
			fresh = append(fresh, element)
		}
	}
	return fresh
}

// switchTo makes another tab the active one.
func (worker *FakeBrowserWorker) switchTo(tabID string) error {
	for at := range worker.tabs {
		worker.tabs[at].Active = worker.tabs[at].ID == tabID
	}
	for _, tab := range worker.tabs {
		if tab.ID == tabID {
			return nil
		}
	}
	return fmt.Errorf("there is no tab called %q, so list the tabs and use one of those", tabID)
}

// closeTab closes one tab and leaves the rest.
func (worker *FakeBrowserWorker) closeTab(tabID string) {
	kept := worker.tabs[:0]
	for _, tab := range worker.tabs {
		if tab.ID != tabID {
			kept = append(kept, tab)
		}
	}
	worker.tabs = kept
}
