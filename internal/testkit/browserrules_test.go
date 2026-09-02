package testkit_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// The rules worker/browser/PROTOCOL.md writes out: what a wall is reported on,
// how an expectation is judged, and what a page that never comes to rest does.

func TestOpenAndReadReportTheWallThePageShows(t *testing.T) {
	ctx := context.Background()
	for _, row := range []struct {
		page string
		kind contract.WallKind
	}{
		{testkit.FixtureLoginPage, contract.WallLogin},
		{testkit.FixtureTwoFactorPage, contract.WallTwoFactor},
		{testkit.FixtureCaptchaPage, contract.WallCaptcha},
	} {
		t.Run(string(row.kind), func(t *testing.T) {
			worker := testkit.NewFakeBrowserWorker()
			defer worker.Close()

			opened, err := worker.Open(ctx, row.page)
			if err != nil {
				t.Fatalf("opening %s failed: %v", row.page, err)
			}
			if opened.Wall == nil || opened.Wall.Kind != row.kind {
				t.Errorf("opening %s reported the wall %+v, want a %q wall on the snapshot", row.page, opened.Wall, row.kind)
			}

			read, err := worker.Read(ctx, contract.ReadOptions{})
			if err != nil {
				t.Fatalf("reading %s failed: %v", row.page, err)
			}
			if read.Wall == nil || read.Wall.Kind != row.kind {
				t.Errorf("reading %s reported the wall %+v, want a %q wall on the snapshot", row.page, read.Wall, row.kind)
			}
		})
	}

	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	opened, err := worker.Open(ctx, testkit.FixtureSimplePage)
	if err != nil {
		t.Fatalf("opening the simple page failed: %v", err)
	}
	if opened.Wall != nil {
		t.Errorf("the simple page reported the wall %+v, and there is nothing in the way of it", opened.Wall)
	}
}

func TestAPageThatNeverComesToRestIsReturnedAsItStands(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	if _, err := worker.Open(ctx, testkit.FixtureSimplePage); err != nil {
		t.Fatalf("opening the page failed: %v", err)
	}

	worker.NextActionTimesOutSettling()
	diff, err := worker.Click(ctx, testkit.FixtureChangeLinkRef, "the page comes to rest")

	if err != nil {
		t.Fatalf("a page that kept changing was an error, and a live page must stay usable: %v", err)
	}
	if diff.Settled {
		t.Error("the diff says the page settled, and it was told to keep changing")
	}
	if diff.Snapshot.URL == "" {
		t.Errorf("the diff carries no snapshot of the page as it stood: %+v", diff)
	}
	if !strings.Contains(diff.Seen, "changing") {
		t.Errorf("what was seen is %q, and it must say the page kept changing", diff.Seen)
	}
	if diff.ExpectationMet {
		t.Error("a page that never came to rest met its expectation")
	}
}

func TestAPageThatCannotBeReadAtAllIsStillAnError(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	if _, err := worker.Open(ctx, testkit.FixtureSimplePage); err != nil {
		t.Fatalf("opening the page failed: %v", err)
	}

	worker.NextActionCannotBeRead()
	_, err := worker.Click(ctx, testkit.FixtureChangeLinkRef, "anything at all")

	if err == nil {
		t.Fatal("a page that cannot be read at all came back as a diff, and that is what -32001 is for")
	}
	if !strings.Contains(err.Error(), "settle") {
		t.Errorf("the failure is %q, want the settle timeout", err)
	}
}

func TestAnExpectationIsJudgedByTheRuleTheProtocolWritesOut(t *testing.T) {
	for _, row := range []struct {
		name        string
		expectation string
		met         bool
	}{
		{"a word of the new title", "the page changed", true},
		{"a word of a new element's name", "a heading appears", true},
		{"a word of a new element's role", "a button appears", true},
		{"nothing on the page at all", "the timeline fills with posts", false},
		{"only stop words and short words", "it is up to the", true},
		{"a word that is only in the old page", "the note box is empty", false},
	} {
		t.Run(row.name, func(t *testing.T) {
			ctx := context.Background()
			worker := testkit.NewFakeBrowserWorker()
			defer worker.Close()
			if _, err := worker.Open(ctx, testkit.FixtureSimplePage); err != nil {
				t.Fatalf("opening the page failed: %v", err)
			}

			diff, err := worker.Click(ctx, testkit.FixtureChangeLinkRef, row.expectation)

			if err != nil {
				t.Fatalf("clicking failed: %v", err)
			}
			if diff.ExpectationMet != row.met {
				t.Errorf("the expectation %q was judged %v, want %v. The page is now %q with %d new elements",
					row.expectation, diff.ExpectationMet, row.met, diff.Snapshot.Title, len(diff.NewElements))
			}
			if !diff.ExpectationMet && diff.Seen == "" {
				t.Error("the expectation was not met and nothing says what did happen instead")
			}
		})
	}
}

func TestTypingMeetsAnExpectationThatNamesTheBoxItWasTypedInto(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	worker.AddPage(contract.Snapshot{
		URL: "https://fixture.test/compose", Title: "Compose post", TabID: "t1",
		Elements: []contract.Element{{Ref: "e3", Role: "textbox", Name: "Post text"}},
	})
	if _, err := worker.Open(ctx, "https://fixture.test/compose"); err != nil {
		t.Fatalf("opening the compose page failed: %v", err)
	}

	// Typing changes no element, so the only place these words can be found is
	// the box that was typed into.
	diff, err := worker.Type(ctx, "e3", "Nine years of DigiByte.", "the text box holds the post")

	if err != nil {
		t.Fatalf("typing failed: %v", err)
	}
	if !diff.ExpectationMet {
		t.Errorf("typing into the box named Post text did not meet %q: %+v", "the text box holds the post", diff)
	}
}

func TestAWallNeverMeetsAnExpectationHoweverItIsWorded(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	if _, err := worker.Open(ctx, testkit.FixtureLoginPage); err != nil {
		t.Fatalf("opening the login page failed: %v", err)
	}

	diff, err := worker.Press(ctx, "Enter", "the sign in form is there")

	if err != nil {
		t.Fatalf("pressing a key failed: %v", err)
	}
	if diff.Wall == nil {
		t.Fatalf("the login page reported no wall: %+v", diff)
	}
	if diff.ExpectationMet {
		t.Error("a wall met an expectation, and the worker never gets past a wall")
	}
}
