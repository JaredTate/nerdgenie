package testkit_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestATestCanAddItsOwnPageAndSayWhereALinkGoes(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()

	worker.AddPage(contract.Snapshot{
		URL: "https://fixture.test/blog", Title: "The blog", TabID: "t1",
		Elements: []contract.Element{{Ref: "e20", Role: "heading", Name: "The blog"}},
	})
	worker.LinkGoesTo(testkit.FixtureChangeLinkRef, "https://fixture.test/blog")

	if _, err := worker.Open(ctx, testkit.FixtureSimplePage); err != nil {
		t.Fatalf("opening the page failed: %v", err)
	}
	diff, err := worker.Click(ctx, testkit.FixtureChangeLinkRef, "the blog opens")
	if err != nil {
		t.Fatalf("clicking the link failed: %v", err)
	}
	if diff.Snapshot.URL != "https://fixture.test/blog" {
		t.Errorf("the click went to %q, want the page the test added", diff.Snapshot.URL)
	}
}

func TestTheFakeBrowserSwitchesAndClosesTabs(t *testing.T) {
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

	tabs, err := worker.Tabs(ctx, contract.TabSwitch, "t2")
	if err != nil {
		t.Fatalf("switching tabs failed: %v", err)
	}
	active := ""
	for _, tab := range tabs {
		if tab.Active {
			active = tab.ID
		}
	}
	if active != "t2" {
		t.Errorf("the active tab is %q, want t2", active)
	}

	tabs, err = worker.Tabs(ctx, contract.TabClose, "t2")
	if err != nil {
		t.Fatalf("closing a tab failed: %v", err)
	}
	if len(tabs) != 1 {
		t.Errorf("there are %d tabs after closing one, want 1", len(tabs))
	}

	if _, err := worker.Tabs(ctx, contract.TabSwitch, "t9"); err == nil {
		t.Error("switching to a tab that is not there was reported as a success, want an error naming it")
	}
	if _, err := worker.Tabs(ctx, "fold", ""); err == nil {
		t.Error("a tab action nobody defined was accepted, want an error naming the three")
	}
}

func TestTheFakeBrowserRefusesEverythingOnceItIsClosed(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	if _, err := worker.Open(ctx, testkit.FixtureSimplePage); err != nil {
		t.Fatalf("opening the page failed: %v", err)
	}
	if err := worker.Close(); err != nil {
		t.Fatalf("closing the worker failed: %v", err)
	}

	if _, err := worker.Open(ctx, testkit.FixtureSimplePage); err == nil {
		t.Error("a closed worker opened a page, and it should refuse")
	}
	if _, err := worker.Read(ctx, contract.ReadOptions{}); err == nil {
		t.Error("a closed worker read a page, and it should refuse")
	}
	if _, err := worker.Screenshot(ctx); err == nil {
		t.Error("a closed worker took a screenshot, and it should refuse")
	}
	health, err := worker.Health(ctx)
	if err != nil {
		t.Fatalf("asking a closed worker about its health failed: %v", err)
	}
	if health.Healthy || health.Detail == "" {
		t.Errorf("a closed worker reports %+v, want unhealthy with a reason", health)
	}
}

func TestReadingOnlyTheVisiblePartLeavesNothingBelowTheFold(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	if _, err := worker.Open(ctx, testkit.FixtureSimplePage); err != nil {
		t.Fatalf("opening the page failed: %v", err)
	}

	whole, err := worker.Read(ctx, contract.ReadOptions{})
	if err != nil {
		t.Fatalf("reading the page failed: %v", err)
	}
	visible, err := worker.Read(ctx, contract.ReadOptions{VisibleOnly: true})
	if err != nil {
		t.Fatalf("reading the visible part failed: %v", err)
	}

	if whole.BelowFold == 0 {
		t.Error("the whole page says nothing is below the fold, and the fixture has elements down there")
	}
	if visible.BelowFold != 0 {
		t.Errorf("reading only the visible part says %d elements are below the fold, want 0", visible.BelowFold)
	}
}

func TestABatchStopsAtTheStepThatFails(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	if _, err := worker.Open(ctx, testkit.FixtureSimplePage); err != nil {
		t.Fatalf("opening the page failed: %v", err)
	}

	worker.NextActionChangesNothing()
	diffs, err := worker.Act(ctx, []contract.ActStep{
		{Method: "press", Key: "Enter", Expectation: "the form is submitted"},
		{Method: "press", Key: "Enter", Expectation: "this step should never run"},
	})
	if err != nil {
		t.Fatalf("acting failed: %v", err)
	}
	if len(diffs) != 1 {
		t.Errorf("the batch ran %d steps, want it to stop at the first one that failed", len(diffs))
	}

	if _, err := worker.Act(ctx, []contract.ActStep{{Method: "somersault", Expectation: "nothing"}}); err == nil {
		t.Error("a batch step with a method nobody defined was accepted, want an error naming the four")
	}
	if _, err := worker.Act(ctx, []contract.ActStep{{Method: "click", Ref: "e999", Expectation: "nothing"}}); err == nil {
		t.Error("a batch step pointing at an element that is not there was accepted, want an error")
	}
}

func TestABatchCarriesAScrollStep(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	if _, err := worker.Open(ctx, testkit.FixtureSimplePage); err != nil {
		t.Fatalf("opening the page failed: %v", err)
	}

	diffs, err := worker.Act(ctx, []contract.ActStep{
		{Method: "scroll", Direction: contract.ScrollDown, Amount: 3, Expectation: "more of the page shows"},
		{Method: "press", Key: "Enter", Expectation: "the form is submitted"},
	})
	if err != nil {
		t.Fatalf("a batch holding a scroll step was refused: %v", err)
	}
	if len(diffs) != 2 {
		t.Errorf("a batch of a scroll and a press gave %d diffs, want one for each step", len(diffs))
	}
}

func TestTheProtocolServerFillsALoginWithoutHandingTheValuesBack(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	server := testkit.NewBrowserProtocolServer(t, worker)

	callProtocol(t, server.SocketPath(),
		`{"jsonrpc":"2.0","id":1,"method":"open","params":{"url":"`+testkit.FixtureLoginPage+`"}}`)
	answer := callProtocol(t, server.SocketPath(),
		`{"jsonrpc":"2.0","id":2,"method":"loginFill","params":{"usernameRef":"`+testkit.FixtureUsernameRef+
			`","passwordRef":"`+testkit.FixturePasswordRef+
			`","username":"digibyte","password":"correct horse battery staple"}}`)

	if _, failed := answer["error"]; failed {
		t.Fatalf("filling the login over the protocol failed: %+v", answer["error"])
	}
	whole := mustPrint(t, answer)
	if strings.Contains(whole, "correct horse battery staple") {
		t.Errorf("the protocol answer holds the password:\n%s", whole)
	}

	broken := callProtocol(t, server.SocketPath(), `{"jsonrpc":"2.0","id":3,"method":"loginFill","params":"not an object"}`)
	if _, failed := broken["error"]; !failed {
		t.Error("login parameters that are not an object were accepted, want an error")
	}
}

func TestTheProtocolServerReportsAPageThatNeverSettles(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	server := testkit.NewBrowserProtocolServer(t, worker)

	callProtocol(t, server.SocketPath(),
		`{"jsonrpc":"2.0","id":1,"method":"open","params":{"url":"`+testkit.FixtureSimplePage+`"}}`)

	// A page that keeps changing comes back as a diff with settled false, so
	// that a live page such as a chat or a clock stays usable.
	worker.NextActionTimesOutSettling()
	living := callProtocol(t, server.SocketPath(),
		`{"jsonrpc":"2.0","id":2,"method":"press","params":{"key":"Enter","expectation":"the form is submitted"}}`)
	if failed, isFailure := living["error"]; isFailure {
		t.Fatalf("a page that kept changing came back as an error: %+v", failed)
	}
	result, isResult := living["result"].(map[string]any)
	if !isResult {
		t.Fatalf("a page that kept changing came back with no result: %+v", living)
	}
	if settled, said := result["settled"].(bool); !said || settled {
		t.Errorf("the diff says the page settled: %+v", result)
	}

	// A page that cannot be read at all is the one settling failure that is an
	// error, and -32001 is what the protocol answers it with.
	worker.NextActionCannotBeRead()
	answer := callProtocol(t, server.SocketPath(),
		`{"jsonrpc":"2.0","id":3,"method":"press","params":{"key":"Enter","expectation":"the form is submitted"}}`)

	failure, isFailure := answer["error"].(map[string]any)
	if !isFailure {
		t.Fatalf("a page that could not be read came back with no error: %+v", answer)
	}
	if failure["code"] != float64(testkit.CodeSettleTimeout) {
		t.Errorf("the error came back with code %v, want %d", failure["code"], testkit.CodeSettleTimeout)
	}
}

// mustPrint turns an answer back into text so a test can search the whole of it.
func mustPrint(t *testing.T, answer map[string]any) string {
	t.Helper()
	return strings.TrimSpace(strings.Join(printableValues(answer), " "))
}

// printableValues walks a decoded JSON value and returns every string in it.
func printableValues(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case map[string]any:
		found := []string{}
		for _, inner := range typed {
			found = append(found, printableValues(inner)...)
		}
		return found
	case []any:
		found := []string{}
		for _, inner := range typed {
			found = append(found, printableValues(inner)...)
		}
		return found
	default:
		return nil
	}
}
