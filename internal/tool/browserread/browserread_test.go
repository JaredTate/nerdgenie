package browserread_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/tool/browserread"
)

// newTool builds the read tool over the fake browser worker, already on the
// simple fixture page.
func newTool(t *testing.T) (*browserread.Tool, *testkit.FakeBrowserWorker) {
	t.Helper()
	worker := testkit.NewFakeBrowserWorker()
	if _, err := worker.Open(context.Background(), testkit.FixtureSimplePage); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	return browserread.New(browserread.Settings{Browser: worker}), worker
}

// run calls the tool with the fields written as JSON.
func run(t *testing.T, tool *browserread.Tool, fields map[string]any) (contract.ToolOutput, error) {
	t.Helper()
	written, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("cannot write the tool's input as JSON: %v", err)
	}
	return tool.Run(context.Background(), written)
}

func TestTheDescriptionFitsInTheCapAndTakesTheFixedFieldNames(t *testing.T) {
	tool, _ := newTool(t)
	spec := tool.Spec()

	if spec.Name != contract.ToolBrowserRead {
		t.Errorf("the tool calls itself %q, want %q", spec.Name, contract.ToolBrowserRead)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		t.Errorf("the description is %d words, and the cap is %d", words, contract.MaxToolDescriptionWords)
	}
	if len(spec.Fields) == 0 || spec.Fields[0].Name != "intent" {
		t.Errorf("the tool's fields are %v, and every browser call says what the step is for", spec.Fields)
	}
}

func TestThePageComesBackAsACompactTree(t *testing.T) {
	tool, _ := newTool(t)

	output, err := run(t, tool, map[string]any{"intent": "see what is on the page"})
	if err != nil {
		t.Fatalf("reading the page failed: %v", err)
	}
	testkit.Golden(t, "a_page.txt", []byte(output.Text))
}

func TestReadingOnlyWhatIsInViewLeavesOutTheCountBelowTheFold(t *testing.T) {
	tool, _ := newTool(t)

	output, err := run(t, tool, map[string]any{"intent": "see the top of the page", "visible_only": true})
	if err != nil {
		t.Fatalf("reading the visible part of the page failed: %v", err)
	}
	if strings.Contains(output.Text, "below the fold") {
		t.Errorf("reading only what is in view still counted what is below it: %q", output.Text)
	}
}

func TestAWallOnThePageIsSaidPlainly(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	if _, err := worker.Open(context.Background(), testkit.FixtureLoginPage); err != nil {
		t.Fatalf("cannot open the login page: %v", err)
	}
	tool := browserread.New(browserread.Settings{Browser: worker})

	output, err := run(t, tool, map[string]any{"intent": "see whether we are signed in"})
	if err != nil {
		t.Fatalf("reading the login page failed: %v", err)
	}
	if !strings.Contains(output.Text, "login") {
		t.Errorf("the login wall was not said plainly: %q", output.Text)
	}
}

func TestBadInputIsRefusedWithALineTheModelCanActOn(t *testing.T) {
	tool, _ := newTool(t)

	if _, err := tool.Run(context.Background(), json.RawMessage("not json")); err == nil {
		t.Errorf("input that is not JSON was treated as a call")
	}
	if _, err := run(t, tool, map[string]any{}); err == nil {
		t.Errorf("a call with no intent was run, and every browser call says what the step is for")
	}
}

func TestAToolWithNoBrowserWiredInSaysSo(t *testing.T) {
	tool := browserread.New(browserread.Settings{})

	_, err := run(t, tool, map[string]any{"intent": "see the page"})
	if err == nil {
		t.Fatalf("a page was read with no browser behind the tool")
	}
	if !strings.Contains(err.Error(), "browser") {
		t.Errorf("the refusal reads %q and does not say what is missing", err)
	}
}

func TestAPageWithNothingOnItStillReads(t *testing.T) {
	text := browserread.PageText(contract.Snapshot{URL: "https://example.com/", Title: "Nothing here"})

	if !strings.Contains(text, "Nothing here") {
		t.Errorf("a page with no elements on it read as %q", text)
	}
}

func TestATreeLongerThanTheCapIsCut(t *testing.T) {
	page := contract.Snapshot{URL: "https://example.com/", Title: "A crowded page"}
	for at := range browserread.MaxElements + 10 {
		page.Elements = append(page.Elements, contract.Element{Ref: contract.ElementRef(at + 1), Role: "link", Name: "a link"})
	}

	text := browserread.PageText(page)
	if !strings.Contains(text, "10 more") {
		t.Errorf("a crowded page was cut without saying how much was left out: %q", text)
	}
}

func TestADiffReadsAsWhatChangedAndWhetherItWasExpected(t *testing.T) {
	changed := contract.Diff{
		URLChanged: true, URL: testkit.FixtureChangedPage, ExpectationMet: true, Settled: true,
		NewElements: []contract.Element{{Ref: "e4", Role: "heading", Name: "The page changed", New: true}},
		NewTab:      "t2",
		Dialog:      &contract.Dialog{Kind: "confirm", Message: "Are you sure?"},
		Download:    &contract.Download{Filename: "notes.pdf", Path: "/tmp/notes.pdf"},
		Snapshot:    contract.Snapshot{URL: testkit.FixtureChangedPage, Title: "The page changed", TabID: "t1"},
	}

	testkit.Golden(t, "a_change.txt", []byte(browserread.ChangeText(changed)))
}

func TestADiffThatDidNotMeetItsExpectationSaysWhatWasSeen(t *testing.T) {
	text := browserread.ChangeText(contract.Diff{
		URL: testkit.FixtureSimplePage, ExpectationMet: false, Settled: false,
		Seen:     "nothing on the page changed",
		Wall:     &contract.Wall{Kind: contract.WallCaptcha, Detail: "a checkbox named I am not a robot"},
		Snapshot: contract.Snapshot{URL: testkit.FixtureSimplePage, Title: "A simple page"},
	})

	for _, wanted := range []string{"not what was expected", "nothing on the page changed", "captcha", "did not come to rest"} {
		if !strings.Contains(text, wanted) {
			t.Errorf("the change does not say %q anywhere: %q", wanted, text)
		}
	}
}
