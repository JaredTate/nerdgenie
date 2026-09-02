package testkit_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestTheFakeBrowserOpensAFixturePageAndReadsItBack(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()

	snapshot, err := worker.Open(ctx, testkit.FixtureSimplePage)
	if err != nil {
		t.Fatalf("opening the simple fixture page failed: %v", err)
	}
	if snapshot.URL != testkit.FixtureSimplePage || len(snapshot.Elements) == 0 {
		t.Errorf("the page came back as %+v, want the simple fixture with elements on it", snapshot)
	}

	again, err := worker.Read(ctx, contract.ReadOptions{})
	if err != nil {
		t.Fatalf("reading the page failed: %v", err)
	}
	if again.URL != snapshot.URL {
		t.Errorf("reading gave the page %q, want the one that was open", again.URL)
	}
}

func TestTheFakeBrowserRefusesAPageItDoesNotHave(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()

	if _, err := worker.Open(context.Background(), "https://fixture.test/nowhere"); err == nil {
		t.Fatal("opening a page the fixture does not have was reported as a success, want an error naming it")
	}
}

func TestTheFakeBrowserClickingALinkMovesToTheNextPage(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	if _, err := worker.Open(ctx, testkit.FixtureSimplePage); err != nil {
		t.Fatalf("opening the page failed: %v", err)
	}

	diff, err := worker.Click(ctx, testkit.FixtureChangeLinkRef, "the page changes")
	if err != nil {
		t.Fatalf("clicking the link failed: %v", err)
	}
	if !diff.URLChanged || diff.Snapshot.URL != testkit.FixtureChangedPage {
		t.Errorf("the click gave %+v, want a move to the changed page", diff)
	}
	if len(diff.NewElements) == 0 {
		t.Error("the click produced no new elements, and the changed page has some")
	}
	if !diff.ExpectationMet {
		t.Error("the click says the expectation was not met, and the page did change")
	}
}

func TestTheFakeBrowserFindsAnElementAgainWhenItsReferenceHasGoneStale(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	if _, err := worker.Open(ctx, testkit.FixtureSimplePage); err != nil {
		t.Fatalf("opening the page failed: %v", err)
	}
	worker.MakeReferenceStale(testkit.FixtureChangeLinkRef)

	diff, err := worker.Click(ctx, testkit.FixtureChangeLinkRef, "the page changes")
	if err != nil {
		t.Fatalf("clicking a stale reference failed, and the worker is supposed to find it again: %v", err)
	}
	if !diff.URLChanged {
		t.Error("the click after a stale reference changed nothing")
	}
	if worker.StaleRecoveries() != 1 {
		t.Errorf("the worker recovered from %d stale references, want 1", worker.StaleRecoveries())
	}
}

func TestTheFakeBrowserCanBeToldToDoTheFourThingsThatGoWrong(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	if _, err := worker.Open(ctx, testkit.FixtureSimplePage); err != nil {
		t.Fatalf("opening the page failed: %v", err)
	}

	worker.NextActionChangesNothing()
	diff, err := worker.Click(ctx, testkit.FixtureChangeLinkRef, "the page changes")
	if err != nil {
		t.Fatalf("a click that changes nothing failed instead of reporting: %v", err)
	}
	if diff.ExpectationMet || diff.Seen == "" {
		t.Errorf("a click that changed nothing came back as %+v, want the expectation unmet and what was seen", diff)
	}

	worker.NextActionOpensADialog(contract.Dialog{Kind: "confirm", Message: "Are you sure?"})
	diff, err = worker.Click(ctx, testkit.FixtureChangeLinkRef, "the page changes")
	if err != nil || diff.Dialog == nil {
		t.Errorf("a click that opens a dialog gave %+v and error %v, want the dialog on the diff", diff, err)
	}

	worker.NextActionStartsADownload(contract.Download{Filename: "notes.pdf", Path: "/tmp/notes.pdf"})
	diff, err = worker.Click(ctx, testkit.FixtureChangeLinkRef, "a file downloads")
	if err != nil || diff.Download == nil {
		t.Errorf("a click that starts a download gave %+v and error %v, want the download on the diff", diff, err)
	}

	worker.NextActionTimesOutSettling()
	if _, err := worker.Click(ctx, testkit.FixtureChangeLinkRef, "the page changes"); err == nil {
		t.Error("a click that never settles was reported as a success, want an error saying the page did not settle")
	}
}

func TestTheFakeBrowserReportsTheThreeWalls(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		page string
		kind contract.WallKind
	}{
		{testkit.FixtureLoginPage, contract.WallLogin},
		{testkit.FixtureTwoFactorPage, contract.WallTwoFactor},
		{testkit.FixtureCaptchaPage, contract.WallCaptcha},
	}
	for _, test := range tests {
		worker := testkit.NewFakeBrowserWorker()
		if _, err := worker.Open(ctx, test.page); err != nil {
			t.Fatalf("opening %s failed: %v", test.page, err)
		}
		diff, err := worker.Press(ctx, "Enter", "the form is submitted")
		if err != nil {
			t.Fatalf("pressing a key on %s failed: %v", test.page, err)
		}
		if diff.Wall == nil || diff.Wall.Kind != test.kind {
			t.Errorf("%s reported the wall %+v, want %q", test.page, diff.Wall, test.kind)
		}
		if diff.ExpectationMet {
			t.Errorf("%s says the expectation was met, and a wall never meets one", test.page)
		}
		worker.Close()
	}
}

func TestTheFakeBrowserTypesAndNeverGivesTheCredentialsBack(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	if _, err := worker.Open(ctx, testkit.FixtureLoginPage); err != nil {
		t.Fatalf("opening the login page failed: %v", err)
	}

	diff, err := worker.LoginFill(ctx, contract.LoginFields{
		UsernameRef: testkit.FixtureUsernameRef,
		PasswordRef: testkit.FixturePasswordRef,
		Username:    "digibyte",
		Password:    "correct horse battery staple",
		Code:        "123456",
	})
	if err != nil {
		t.Fatalf("filling the login form failed: %v", err)
	}

	for _, secret := range []string{"correct horse battery staple", "123456"} {
		if strings.Contains(wholeDiffAsJSON(t, diff), secret) {
			t.Errorf("the diff from the login holds %q, and it must never hold a credential:\n%s",
				secret, wholeDiffAsJSON(t, diff))
		}
	}
	if typed := worker.TypedInto(testkit.FixtureUsernameRef); len(typed) != 1 || typed[0] != "digibyte" {
		t.Errorf("the worker typed %v into the username box, want the one username", typed)
	}
}

// wholeDiffAsJSON is every field of a diff as one piece of text, which is the
// only haystack that cannot miss a field somebody forgot to scrub.
func wholeDiffAsJSON(t *testing.T, diff contract.Diff) string {
	t.Helper()
	written, err := json.Marshal(diff)
	if err != nil {
		t.Fatalf("cannot write the diff as JSON: %v", err)
	}
	return string(written)
}

func TestALoginNeverHandsBackACredentialInAnElementThatAppeared(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	password := "correct horse battery staple"
	if _, err := worker.Open(ctx, testkit.FixtureLoginPage); err != nil {
		t.Fatalf("opening the login page failed: %v", err)
	}

	// The page grows a message while the login is being filled, the way a site
	// that echoes what was typed does.
	worker.AddPage(contract.Snapshot{
		URL: testkit.FixtureLoginPage, Title: "Sign in", TabID: "t1",
		Elements: []contract.Element{
			{Ref: testkit.FixtureUsernameRef, Role: "textbox", Name: "Username"},
			{Ref: testkit.FixturePasswordRef, Role: "textbox", Name: "Password"},
			{Ref: "e9", Role: "alert", Name: "we could not sign you in with " + password},
		},
	})

	diff, err := worker.LoginFill(ctx, contract.LoginFields{
		UsernameRef: testkit.FixtureUsernameRef,
		PasswordRef: testkit.FixturePasswordRef,
		Username:    "digibyte",
		Password:    password,
	})

	if err != nil {
		t.Fatalf("filling the login form failed: %v", err)
	}
	if len(diff.NewElements) == 0 {
		t.Fatal("no element appeared during the login, so the test proves nothing")
	}
	if strings.Contains(wholeDiffAsJSON(t, diff), password) {
		t.Errorf("the diff holds the password in an element that appeared:\n%s", wholeDiffAsJSON(t, diff))
	}
}

func TestTheFakeBrowserHandlesTabsScrollingActingAndScreenshots(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	if _, err := worker.Open(ctx, testkit.FixtureSimplePage); err != nil {
		t.Fatalf("opening the page failed: %v", err)
	}

	worker.NextActionOpensATab("t2")
	if _, err := worker.Click(ctx, testkit.FixtureChangeLinkRef, "a new tab opens"); err != nil {
		t.Fatalf("clicking failed: %v", err)
	}
	tabs, err := worker.Tabs(ctx, contract.TabList, "")
	if err != nil || len(tabs) != 2 {
		t.Fatalf("the worker lists %d tabs and error %v, want 2", len(tabs), err)
	}

	if _, err := worker.Scroll(ctx, contract.ScrollDown, 3, "more of the page shows"); err != nil {
		t.Fatalf("scrolling failed: %v", err)
	}

	diffs, err := worker.Act(ctx, []contract.ActStep{
		{Method: "type", Ref: testkit.FixtureUsernameRef, Text: "hello", Expectation: "the box holds the text"},
		{Method: "press", Key: "Enter", Expectation: "the form is submitted"},
	})
	if err != nil {
		t.Fatalf("acting failed: %v", err)
	}
	if len(diffs) != 2 {
		t.Errorf("acting on two steps gave %d diffs, want 2", len(diffs))
	}

	picture, err := worker.Screenshot(ctx)
	if err != nil || picture.PNGBase64 == "" || len(picture.Marks) == 0 {
		t.Errorf("the screenshot came back as %+v with error %v, want a picture with numbered marks", picture, err)
	}
}

func TestTheFakeBrowserKeepsTheBrowserWorkerContract(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()

	if err := testkit.CheckBrowserWorker(context.Background(), worker); err != nil {
		t.Fatalf("the fake browser worker does not keep the browser contract: %v", err)
	}
}
