package browser

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// Every method of worker/browser/PROTOCOL.md goes out over the wire and comes
// back as the shape internal/contract promises. The worker on the other end is
// the fake one from internal/testkit, speaking the protocol over a socket, so
// what is proved here is the document and not a Go interface.
func TestEveryMethodRoundTripsThroughTheProtocol(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	ctx := context.Background()

	page := openTheSimplePage(t, browser)
	if page.URL != testkit.FixtureSimplePage || len(page.Elements) == 0 {
		t.Fatalf("open answered with %+v, and it should have been the fixture page with elements on it", page)
	}
	read, err := browser.Read(ctx, contract.ReadOptions{VisibleOnly: true})
	if err != nil || read.BelowFold != 0 {
		t.Fatalf("read answered with %+v and %v, and reading only what is visible should leave nothing below the fold", read, err)
	}

	typed, err := browser.Type(ctx, testkit.FixtureUsernameRef, "a note", "the box named Name holds the note")
	if err != nil || !typed.ExpectationMet {
		t.Fatalf("type answered with %+v and %v, and the expectation should have been met", typed, err)
	}
	if got := world.worker.TypedInto(testkit.FixtureUsernameRef); len(got) != 1 || got[0] != "a note" {
		t.Fatalf("the worker was typed %v, and it should have been the one note", got)
	}
	roundTripTheRest(t, browser)
}

// roundTripTheRest is the other half of the round trip, kept apart so that no
// test runs past sixty lines.
func roundTripTheRest(t *testing.T, browser *Browser) {
	t.Helper()
	ctx := context.Background()

	pressed, err := browser.Press(ctx, "Enter", "")
	if err != nil {
		t.Fatalf("press failed: %v", err)
	}
	scrolled, err := browser.Scroll(ctx, contract.ScrollDown, 3, "")
	if err != nil {
		t.Fatalf("scroll failed: %v", err)
	}
	if pressed.URL != scrolled.URL {
		t.Fatalf("press left the browser on %s and scroll on %s, and neither moves the page", pressed.URL, scrolled.URL)
	}

	clicked, err := browser.Click(ctx, testkit.FixtureChangeLinkRef, "the page changed")
	if err != nil || clicked.URL != testkit.FixtureChangedPage {
		t.Fatalf("click answered with %+v and %v, and it should have moved to the changed page", clicked, err)
	}
	tabs, err := browser.Tabs(ctx, contract.TabList, "")
	if err != nil || len(tabs) != 1 || tabs[0].ID != "t1" {
		t.Fatalf("tabs answered with %+v and %v, and there should have been one tab called t1", tabs, err)
	}
	picture, err := browser.Screenshot(ctx)
	if err != nil || picture.PNGBase64 == "" || len(picture.Marks) == 0 {
		t.Fatalf("screenshot answered with %d marks and %v, and it should have been a picture with marks on it", len(picture.Marks), err)
	}
	health, err := browser.Health(ctx)
	if err != nil || !health.Healthy {
		t.Fatalf("health answered with %+v and %v, and the fixture worker is always healthy", health, err)
	}
}

// A batch runs step by step and comes back as one diff per step.
func TestABatchComesBackAsOneDiffPerStep(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	openTheSimplePage(t, browser)

	diffs, err := browser.Act(context.Background(), []contract.ActStep{
		{Method: "type", Ref: testkit.FixtureUsernameRef, Text: "first", Expectation: "the box named Name holds it"},
		{Method: "click", Ref: testkit.FixtureChangeLinkRef, Expectation: "the page changed"},
	})
	if err != nil {
		t.Fatalf("the batch failed: %v", err)
	}
	if len(diffs) != 2 {
		t.Fatalf("the batch answered with %d diffs, and it should have been one per step", len(diffs))
	}
	if diffs[1].URL != testkit.FixtureChangedPage {
		t.Fatalf("the last step left the browser on %s, and it should have been the changed page", diffs[1].URL)
	}
	if browser.CurrentAddress() != testkit.FixtureChangedPage {
		t.Fatalf("the browser thinks it is on %s, and it should have followed the batch", browser.CurrentAddress())
	}
}

// Answering the open dialog goes through the protocol and is recorded.
func TestADialogIsAnsweredThroughTheProtocol(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	openTheSimplePage(t, browser)
	world.worker.NextActionOpensADialog(contract.Dialog{Kind: "confirm", Message: "Are you sure?"})

	if _, err := browser.Press(context.Background(), "Enter", ""); err != nil {
		t.Fatalf("the press that opens the dialog failed: %v", err)
	}
	if _, err := browser.Dialog(context.Background(), contract.DialogAccept, "yes"); err != nil {
		t.Fatalf("answering the dialog failed: %v", err)
	}
	answers := world.worker.DialogAnswers()
	if len(answers) != 1 || answers[0].Action != contract.DialogAccept || answers[0].Text != "yes" {
		t.Fatalf("the worker was told %+v, and it should have been one accept with the word yes", answers)
	}
	if _, err := browser.Dialog(context.Background(), contract.DialogAction("wobble"), ""); err == nil {
		t.Fatal("a dialog action that is neither accept nor dismiss should have been refused before it left this side")
	}
}

// The refusals the model can act on come back as refusals, carrying the code the
// protocol's error table gives them and the fresh snapshot it promises.
func TestTheRefusalsTheModelCanActOnCarryTheirCode(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	ctx := context.Background()

	_, err := browser.Read(ctx, contract.ReadOptions{})
	if code := refused(t, err).Code; code != codeNoBrowserOpen {
		t.Fatalf("reading with nothing open came back as %d, and the table says %d", code, codeNoBrowserOpen)
	}

	openTheSimplePage(t, browser)
	_, err = browser.Click(ctx, "e99", "something happens")
	if code := refused(t, err).Code; code != codeNoSuchReference {
		t.Fatalf("clicking an element that is not there came back as %d, and the table says %d", code, codeNoSuchReference)
	}

	world.worker.NextActionCannotBeRead()
	_, err = browser.Press(ctx, "Enter", "")
	if code := refused(t, err).Code; code != codeCouldNotBeRead {
		t.Fatalf("a page that cannot be read came back as %d, and the table says %d", code, codeCouldNotBeRead)
	}

	_, err = browser.Tabs(ctx, contract.TabAction("wobble"), "")
	if code := refused(t, err).Code; code != codeBadParameters {
		t.Fatalf("a tab action that is not one of the three came back as %d, and the table says %d", code, codeBadParameters)
	}
}

// A method the worker does not have is a fault on this side of the pipe, and is
// reported as one rather than handed to the model as page trouble.
func TestAMethodTheWorkerDoesNotHaveIsReportedAsAFaultInCoeus(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)

	err := browser.call(context.Background(), "wobble", map[string]any{}, nil)
	if err == nil || !strings.Contains(err.Error(), "a fault in Coeus") {
		t.Fatalf("asking for a method that does not exist said %v, and it should have named itself a fault in Coeus", err)
	}
	if started, _ := world.starts.counts(); started != 1 {
		t.Fatalf("%d workers were started, and a method that does not exist is no reason to start another", started)
	}
}

// A fresh snapshot rides with a reference the page no longer holds, so that the
// model is handed something it can point at.
func TestAReferenceThatIsGoneCarriesAFreshSnapshot(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, func(options *Options) {
		options.Start = scriptedStart(healthyThen(func(line []byte) string {
			return `{"jsonrpc":"2.0","id":` + identifierIn(line) + `,"error":{"code":-32000,` +
				`"message":"there is no element e9 on the page any more, so point at one in the snapshot below",` +
				`"data":{"url":"https://fixture.test/simple","title":"A simple page","tabId":"t1",` +
				`"elements":[{"ref":"e1","role":"link","name":"Change the page"}],"belowFold":0}}}` + "\n"
		}))
	})

	_, err := browser.Click(context.Background(), "e9", "something happens")
	refusal := refused(t, err)
	page := refusal.Page()
	if page == nil || len(page.Elements) != 1 || page.Elements[0].Ref != "e1" {
		t.Fatalf("the refusal carried %+v, and it should have carried the fresh snapshot with e1 on it", page)
	}
}
