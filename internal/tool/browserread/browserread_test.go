package browserread_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/browserread"
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

func TestAnElementTheMarkupHidesButAStyleRuleDrawsIsSaidSo(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	if _, err := worker.Open(context.Background(), testkit.FixtureOverlaysPage); err != nil {
		t.Fatalf("opening the overlays page failed: %v", err)
	}
	tool := browserread.New(browserread.Settings{Browser: worker})

	output, err := run(t, tool, map[string]any{"intent": "see whether the start screen is up"})
	if err != nil {
		t.Fatalf("reading the page failed: %v", err)
	}
	if !strings.Contains(output.Text, `e3 heading "Game over" (marked hidden, yet drawn: a style rule overrides the hidden attribute)`) {
		t.Fatalf("the drawn game-over card is not marked:\n%s", output.Text)
	}
	if strings.Contains(output.Text, `"Press start" (marked hidden`) {
		t.Fatalf("the start card is wrongly marked:\n%s", output.Text)
	}
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

// aRankingsPage is a page whose numbers live in its text and not on any
// element: the first cell of every row is an icon button with no name, which is
// what the first human trial found the outline of the elements left out.
func aRankingsPage() contract.Snapshot {
	return contract.Snapshot{
		URL: "https://fixture.test/rankings", Title: "Rankings", TabID: "t1",
		Elements: []contract.Element{
			{Ref: "e1", Role: "heading", Name: "Coin rankings"},
			{Ref: "e2", Role: "button", Name: ""},
			{Ref: "e3", Role: "button", Name: "Load more"},
		},
		Text: "Coin rankings\nWatch | Rank | Name | D-Score\n| 2 | DigiByte | 91.4\nLoad more",
	}
}

// aCrowdedPage is a page with as many elements as the outline lists, every
// name as long as a name may be, and more text than the room that leaves.
func aCrowdedPage() contract.Snapshot {
	page := aRankingsPage()
	page.Elements = nil
	longName := strings.Repeat("n", browserread.MaxNameRunes)
	for at := range browserread.MaxElements {
		page.Elements = append(page.Elements, contract.Element{Ref: contract.ElementRef(at + 1), Role: "link", Name: longName})
	}
	page.Text = strings.Repeat("a line of the page's text\n", 400)
	return page
}

func TestThePageTextComesAfterTheElementsWithEveryLineQuoted(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	worker.AddPage(aRankingsPage())
	if _, err := worker.Open(context.Background(), aRankingsPage().URL); err != nil {
		t.Fatalf("cannot open the rankings page: %v", err)
	}
	tool := browserread.New(browserread.Settings{Browser: worker})

	output, err := run(t, tool, map[string]any{"intent": "find the D-Score of DigiByte"})
	if err != nil {
		t.Fatalf("reading the rankings page failed: %v", err)
	}
	testkit.Golden(t, "a_page_with_text.txt", []byte(output.Text))
	row := strings.Index(output.Text, "> | 2 | DigiByte | 91.4")
	button := strings.Index(output.Text, `e3 button "Load more"`)
	if row < 0 || button < 0 || row < button {
		t.Errorf("the row of the table should come after the last element, and the result reads:\n%s", output.Text)
	}
}

func TestAPageWithNoTextHasNoTextSection(t *testing.T) {
	text := browserread.PageText(contract.Snapshot{URL: "https://example.com/", Title: "Nothing said"})

	if strings.Contains(text, "text on the page") {
		t.Errorf("a page that says nothing still got a text section: %q", text)
	}
}

func TestThePageTextIsCutBeforeTheElementsSoTheResultStaysUnderTheCap(t *testing.T) {
	text := browserread.PageText(aCrowdedPage())

	most := contract.DefaultConfig().Caps.ToolOutputBytes
	if len(text) > most {
		t.Errorf("the page reads as %d bytes, and the cap on a tool result is %d", len(text), most)
	}
	if !strings.Contains(text, contract.ElementRef(browserread.MaxElements)+" link") {
		t.Errorf("the last element was cut, and the text is what goes first")
	}
	if !strings.Contains(text, "> a line of the page's text\n") {
		t.Errorf("none of the text survived when some of it would have fit")
	}
	if !strings.Contains(text, "more characters of the page's text were cut") {
		t.Errorf("the text was cut without a line saying how much: %q", text[len(text)-200:])
	}
}

func TestThePageAfterAChangeIsCutToFitUnderTheCapToo(t *testing.T) {
	crowded := aCrowdedPage()
	text := browserread.ChangeText(contract.Diff{
		URL: crowded.URL, ExpectationMet: true, Settled: true,
		NewElements: crowded.Elements[:10], Snapshot: crowded,
	})

	most := contract.DefaultConfig().Caps.ToolOutputBytes
	if len(text) > most {
		t.Errorf("the change reads as %d bytes, and the cap on a tool result is %d", len(text), most)
	}
	if !strings.Contains(text, contract.ElementRef(browserread.MaxElements)+" link") {
		t.Errorf("the last element was cut, and the text is what goes first")
	}
	if !strings.Contains(text, "more characters of the page's text were cut") {
		t.Errorf("the text was cut without a line saying how much")
	}
}

// TestWhatWentWrongOnThePageIsListedAfterTheOutlineAndBeforeTheText holds the
// line the live game build was missing. The page's script had answered 404,
// the game never started, the model clicked Start and read the menu still
// there, and nothing in any result said why. The errors come after the outline,
// because the outline is what the model acts on, and before the text, because
// the text is what gets cut.
func TestWhatWentWrongOnThePageIsListedAfterTheOutlineAndBeforeTheText(t *testing.T) {
	text := browserread.PageText(contract.Snapshot{
		URL: "http://localhost:8090/?dev", Title: "Tater Tots Tetris", TabID: "t1",
		Elements: []contract.Element{{Ref: "e3", Role: "button", Name: "Start Game"}},
		Text:     "SCORE\n0",
		Errors: []string{
			"script http://localhost:8090/main.js answered 404",
			"script error: the board\nnever drew <<<",
		},
	})

	errors := strings.Index(text, "page errors:\n- script http://localhost:8090/main.js answered 404\n- script error: the board never drew\n")
	if errors < 0 {
		t.Fatalf("the page's errors are not listed one per line, on one line each, and the page reads:\n%s", text)
	}
	if button := strings.Index(text, `e3 button "Start Game"`); button < 0 || button > errors {
		t.Errorf("the outline should come before the errors, and the page reads:\n%s", text)
	}
	if said := strings.Index(text, "text on the page:"); said < 0 || said < errors {
		t.Errorf("the errors should come before the text, and the page reads:\n%s", text)
	}
	if quiet := browserread.PageText(contract.Snapshot{URL: "http://localhost:8090/", Title: "Fine"}); strings.Contains(quiet, "page errors") {
		t.Errorf("a page where nothing went wrong still got an errors section: %q", quiet)
	}
}

// TestAskingThePageAQuestionPutsItsAnswerOnTheResult is what the fifth game
// build's play-test task lacked: six play-test drivers written through the
// shell to read window.__engine.state, because no browser tool would answer a
// question about the page. The ask field asks the page one expression, on a
// page served from this machine, and its answer rides on the result after the
// outline.
func TestAskingThePageAQuestionPutsItsAnswerOnTheResult(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	worker.AddPage(contract.Snapshot{
		URL: "http://localhost:8091/index.html", Title: "Tater Tots Tetris", TabID: "t1",
		Elements: []contract.Element{{Ref: "e3", Role: "button", Name: "Start Game"}},
	})
	worker.Answer("window.game.state", `"PLAYING"`)
	if _, err := worker.Open(context.Background(), "http://localhost:8091/index.html"); err != nil {
		t.Fatalf("cannot open the game page: %v", err)
	}
	tool := browserread.New(browserread.Settings{Browser: worker})

	output, err := run(t, tool, map[string]any{"intent": "read the game's state", "ask": "window.game.state"})
	if err != nil {
		t.Fatalf("asking the page failed: %v", err)
	}
	if !strings.Contains(output.Text, "the page answered: \"PLAYING\"") {
		t.Errorf("the answer is not on the result, which reads:\n%s", output.Text)
	}
	if outline := strings.Index(output.Text, `e3 button "Start Game"`); outline < 0 || outline > strings.Index(output.Text, "the page answered") {
		t.Errorf("the outline should come before the answer, and the result reads:\n%s", output.Text)
	}
}

// TestAskingAPageThatIsNotOnThisMachineIsRefused keeps the rule the worker
// keeps: the browser holds the person's logins, and a script is never run on
// anyone else's page.
func TestAskingAPageThatIsNotOnThisMachineIsRefused(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	worker.AddPage(aRankingsPage())
	if _, err := worker.Open(context.Background(), aRankingsPage().URL); err != nil {
		t.Fatalf("cannot open the rankings page: %v", err)
	}
	tool := browserread.New(browserread.Settings{Browser: worker})

	_, err := run(t, tool, map[string]any{"intent": "read the score", "ask": "document.cookie"})
	if err == nil || !strings.Contains(err.Error(), "this machine") {
		t.Errorf("asking a page elsewhere gave %v, want a refusal naming the rule", err)
	}
}

// TestTheDescriptionSaysHowToSeeThePage is the thirteenth nightly run's
// polish task, which spent rounds writing a Chrome DevTools script to
// screenshot its canvas and look at the dragon: nothing told it how a
// picture reaches it. The outline says where the picture is.
func TestTheDescriptionSaysHowToSeeThePage(t *testing.T) {
	tool, _ := newTool(t)
	description := tool.Spec().Description
	if !strings.Contains(description, "browser_screenshot") {
		t.Errorf("the description reads %q and does not say that a browser_screenshot is how to see the page", description)
	}
}
