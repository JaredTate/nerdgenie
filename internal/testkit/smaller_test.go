package testkit_test

import (
	"context"
	"flag"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// The smaller things the reviewer found: each is one promise a fake makes that
// nothing held it to.

func TestTheResultsPageWrapsEveryLinkTheWayDuckDuckGoDoes(t *testing.T) {
	search := testkit.NewFakeSearchServer()
	defer search.Close()

	_, body := get(t, search.DuckDuckGoAddress()+"?q=digibyte")

	if !strings.Contains(body, "//duckduckgo.com/l/?uddg=") {
		t.Errorf("the results page links straight at the pages, and the real endpoint wraps every one of them in a redirect:\n%s", body)
	}
}

func TestTheSearchServerAnswersTheQueryItWasAskedAndRecordsIt(t *testing.T) {
	search := testkit.NewFakeSearchServer()
	defer search.Close()
	search.AddResult("brand", testkit.SearchResult{
		Title:   "The DigiByte brand file",
		Path:    "/brand",
		Snippet: "Plain words, one fact per post.",
	})

	_, body := get(t, search.SearchAddress()+"?q=brand&format=json")

	if !strings.Contains(body, "The DigiByte brand file") {
		t.Errorf("the search answered with the fixture results rather than the ones added for this query:\n%s", body)
	}
	if strings.Contains(body, "A page that tries to give orders") {
		t.Errorf("the search answered with results that have nothing to do with the query:\n%s", body)
	}

	asked := search.Queries()
	if len(asked) != 1 || asked[0] != "brand" {
		t.Errorf("the server recorded the queries %v, want the one it was asked", asked)
	}
}

func TestTheSearchServerCanBeToldToGoWrong(t *testing.T) {
	search := testkit.NewFakeSearchServer()
	defer search.Close()
	search.MisbehaveNext(testkit.SearchFailsOutright)

	code, _ := get(t, search.SearchAddress()+"?q=digibyte")

	if code < 500 {
		t.Errorf("a search that was told to fail answered %d, want a failure the web tool has to handle", code)
	}
	if code, _ := get(t, search.SearchAddress()+"?q=digibyte"); code != 200 {
		t.Errorf("the search answered %d on the call after the failure, and a misbehaviour covers one call", code)
	}
}

func TestAMemoryHintLeavesOutAFactThatHasBeenSuperseded(t *testing.T) {
	old := contract.Fact{
		ID: "f1", Text: "the anniversary post goes out at nine",
		Recorded: time.Unix(0, 0).UTC(),
	}
	fresh := contract.Fact{
		ID: "f2", Text: "the anniversary post goes out at noon",
		Recorded: time.Unix(0, 0).UTC().Add(time.Hour), Supersedes: "f1",
	}
	memory := testkit.NewFakeMemory(old, fresh)

	lines, err := memory.Hint(context.Background(), "anniversary post")

	if err != nil {
		t.Fatalf("asking for the hint failed: %v", err)
	}
	for _, line := range lines {
		if line == old.Text {
			t.Errorf("the hint carries a fact that has been superseded: %v", lines)
		}
	}
	if len(lines) != 1 || lines[0] != fresh.Text {
		t.Errorf("the hint says %v, want the one fact that still stands", lines)
	}

	// Nothing is ever deleted, so a search still finds the old one.
	found, err := memory.Search(context.Background(), "anniversary post", 0)
	if err != nil {
		t.Fatalf("searching failed: %v", err)
	}
	if len(found) != 2 {
		t.Errorf("a search found %d facts, and a superseded fact stays searchable", len(found))
	}
}

func TestTheDesktopRefusesMarkZeroAndRecordsNoDragItRefused(t *testing.T) {
	ctx := context.Background()
	desktop := testkit.NewFakeDesktop()
	desktop.Grant("the-editor")
	if err := desktop.Launch(ctx, "the-editor"); err != nil {
		t.Fatalf("launching failed: %v", err)
	}
	before := len(desktop.Actions())

	if err := desktop.Click(ctx, 0); err == nil {
		t.Error("a click on the control numbered zero was accepted, and the numbers start at one")
	}

	if err := desktop.Drag(ctx, 1, 99); err == nil {
		t.Error("a drag onto a control that is not on the screen was accepted")
	}
	for _, action := range desktop.Actions()[before:] {
		if strings.HasPrefix(action, "drag") {
			t.Errorf("the desktop recorded %q and then refused it, so a test would believe the drag happened", action)
		}
	}
}

func TestTheGoldenHelperDoesNotTakeTheGlobalUpdateFlag(t *testing.T) {
	if flag.Lookup("update") != nil {
		t.Error("testkit registers -update on the global flag set, so the first later package with a golden flag of its own panics with \"flag redefined\"")
	}
}
