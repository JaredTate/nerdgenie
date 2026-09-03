package browser

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// Each site gets a daily budget of actions, and the call past it is refused with
// the number and the moment the budget comes back, which design section 9 asks
// for.
func TestTheDailyBudgetRefusesAtTheLimitAndSaysWhenItResets(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, func(options *Options) { options.DailyActionsPerSite = 3 })
	ctx := context.Background()

	openTheSimplePage(t, browser)
	for spent := 2; spent <= 3; spent++ {
		if _, err := browser.Press(ctx, "Enter", ""); err != nil {
			t.Fatalf("action %d of three failed: %v", spent, err)
		}
	}
	_, err := browser.Press(ctx, "Enter", "")
	if err == nil {
		t.Fatal("the fourth action on a budget of three should have been refused")
	}
	for _, wanted := range []string{"3 of its 3 actions", "fixture.test", "03 Sep 2026 00:00:00"} {
		if !strings.Contains(err.Error(), wanted) {
			t.Fatalf("the refusal said %q, and it should have said %q", err, wanted)
		}
	}
}

// Reading a page, listing tabs, and taking a picture are not actions, so they
// cost a site nothing.
func TestReadingAPageCostsTheSiteNothing(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, func(options *Options) { options.DailyActionsPerSite = 1 })
	ctx := context.Background()

	openTheSimplePage(t, browser)
	for round := 0; round < 3; round++ {
		if _, err := browser.Read(ctx, contract.ReadOptions{}); err != nil {
			t.Fatalf("reading the page failed: %v", err)
		}
		if _, err := browser.Tabs(ctx, contract.TabList, ""); err != nil {
			t.Fatalf("listing the tabs failed: %v", err)
		}
		if _, err := browser.Screenshot(ctx); err != nil {
			t.Fatalf("photographing the page failed: %v", err)
		}
	}
	if spent := browser.budget.spent("fixture.test"); spent != 1 {
		t.Fatalf("the site has spent %d actions, and only the one open should have counted", spent)
	}
}

// A batch spends one action for every step in it, because every step is an
// action on the site.
func TestABatchSpendsOneActionPerStep(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, func(options *Options) { options.DailyActionsPerSite = 3 })
	openTheSimplePage(t, browser)

	steps := []contract.ActStep{
		{Method: "type", Ref: testkit.FixtureUsernameRef, Text: "one", Expectation: "the box holds it"},
		{Method: "type", Ref: testkit.FixtureUsernameRef, Text: "two", Expectation: "the box holds it"},
		{Method: "type", Ref: testkit.FixtureUsernameRef, Text: "three", Expectation: "the box holds it"},
	}
	if _, err := browser.Act(context.Background(), steps); err == nil {
		t.Fatal("three steps on a budget with two actions left should have been refused before any of them ran")
	}
	if typed := world.worker.TypedInto(testkit.FixtureUsernameRef); len(typed) != 0 {
		t.Fatalf("the worker was typed %v, and a refused batch should not have run a single step", typed)
	}
}

// The budget starts again when the day turns over on the clock.
func TestTheBudgetStartsAgainTheNextDay(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, func(options *Options) {
		options.DailyActionsPerSite = 1
		options.IdleStop = 48 * time.Hour
	})
	ctx := context.Background()

	openTheSimplePage(t, browser)
	if _, err := browser.Press(ctx, "Enter", ""); err == nil {
		t.Fatal("the second action on a budget of one should have been refused")
	}
	world.clock.Advance(24 * time.Hour)
	if _, err := browser.Press(ctx, "Enter", ""); err != nil {
		t.Fatalf("the first action of the next day failed: %v", err)
	}
	if spent := browser.budget.spent("fixture.test"); spent != 1 {
		t.Fatalf("the site has spent %d actions today, and it should have been the one", spent)
	}
}

// A page with no hostname behind it, which is a browser that has opened nothing
// yet, is charged to nobody.
func TestAPageWithNoHostnameIsChargedToNobody(t *testing.T) {
	budget := newDailyBudget(testkit.NewFakeClock(theTestMoment), 1)
	for round := 0; round < 5; round++ {
		if err := budget.charge("", 1); err != nil {
			t.Fatalf("charging a page with no hostname said %v, and it should say nothing at all", err)
		}
	}
	if err := budget.charge("example.com", 0); err != nil {
		t.Fatalf("charging no actions at all said %v, and it should say nothing at all", err)
	}
	if spent := budget.spent("example.com"); spent != 0 {
		t.Fatalf("example.com has spent %d actions, and nothing was ever charged to it", spent)
	}
}

// One site is one entry in the budget, whether the address writes the leading
// "www." or not.
func TestOneSiteIsOneEntryWhicheverWayItIsWritten(t *testing.T) {
	pairs := []struct {
		address string
		host    string
	}{
		{address: "https://www.example.com/one", host: "example.com"},
		{address: "https://example.com/two", host: "example.com"},
		{address: "http://a.example.com/three", host: "a.example.com"},
		{address: "wobble", host: ""},
	}
	for _, pair := range pairs {
		if got := hostnameOf(pair.address); got != pair.host {
			t.Fatalf("the hostname of %q read as %q, and it should be %q", pair.address, got, pair.host)
		}
	}
}
