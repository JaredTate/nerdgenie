package command

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/skill"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theBrowserSkillWords are the things the first human trial found the model did
// not know about the browser tools, each of which the shipped skill has to say.
var theBrowserSkillWords = []string{
	"browser_open", "browser_read", "browser_click", "browser_type", "browser_act",
	"browser_login", "browser_handoff", "captcha", "two-factor", "vault",
	"coordinates", "shell", "exactly",
}

// aSkillStoreOver is the real skill loader read over one home folder, with fakes
// for the three things a listing never touches.
func aSkillStoreOver(t *testing.T, home contract.Home) *skill.Store {
	t.Helper()
	store, err := skill.New(skill.Options{
		Home:       home,
		Clock:      testkit.NewFakeClock(time.Date(2026, time.September, 3, 9, 0, 0, 0, time.UTC)),
		Tools:      testkit.NewFakeToolRegistry(),
		Permission: testkit.NewFakePermission(contract.RulingAllow),
	})
	if err != nil {
		t.Fatalf("the skill store could not be built over the home folder: %v", err)
	}
	return store
}

// TestInitShipsTheBrowserSkillAndTheLoaderListsIt proves that a fresh home
// holds a browser skill beside the persona files, that the skill loader lists
// it with a description saying when to load it, that its body loads, and that
// the body says everything the trial found the model did not know.
func TestInitShipsTheBrowserSkillAndTheLoaderListsIt(t *testing.T) {
	home := contract.NewHome(t.TempDir())
	if err := makeTheLayout(home); err != nil {
		t.Fatalf("the layout could not be made: %v", err)
	}

	written, err := os.ReadFile(filepath.Join(home.SkillFolder("browser"), skill.DescriptionFile))
	if err != nil {
		t.Fatalf("nerdgenie init did not write the browser skill: %v", err)
	}
	if words := len(strings.Fields(string(written))); words >= 300 {
		t.Errorf("the browser skill is %d words long, and it has to stay under three hundred", words)
	}
	for _, word := range theBrowserSkillWords {
		if !strings.Contains(string(written), word) {
			t.Errorf("the browser skill says nothing about %q", word)
		}
	}

	store := aSkillStoreOver(t, home)
	listed, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("the skill loader could not list the skills folder: %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "browser" {
		t.Fatalf("the skill loader listed %+v, want the one browser skill", listed)
	}
	for _, words := range []string{"browser tools", "before the first browser"} {
		if !strings.Contains(listed[0].Description, words) {
			t.Errorf("the description %q does not say %q, and it has to say when to load the skill", listed[0].Description, words)
		}
	}
	body, err := store.Load(context.Background(), "browser")
	if err != nil {
		t.Fatalf("the skill loader could not load the browser skill's body: %v", err)
	}
	if !strings.Contains(body, "browser_handoff") {
		t.Errorf("the body the loader hands back does not carry the skill's own words:\n%s", body)
	}
}

// TestInitLeavesAPersonsOwnBrowserSkillAlone proves the same rule the persona
// files follow: a browser skill the person already has is theirs, and init
// never writes over it.
func TestInitLeavesAPersonsOwnBrowserSkillAlone(t *testing.T) {
	home := contract.NewHome(t.TempDir())
	folder := home.SkillFolder("browser")
	if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
		t.Fatalf("the person's own skill folder could not be made: %v", err)
	}
	theirs := "# browser\n\nThe person's own notes about the browser.\n"
	if err := os.WriteFile(filepath.Join(folder, skill.DescriptionFile), []byte(theirs), contract.DataFileMode); err != nil {
		t.Fatalf("the person's own skill could not be written: %v", err)
	}

	if err := makeTheLayout(home); err != nil {
		t.Fatalf("the layout could not be made: %v", err)
	}

	after, err := os.ReadFile(filepath.Join(folder, skill.DescriptionFile))
	if err != nil {
		t.Fatalf("the person's own skill could not be read back: %v", err)
	}
	if string(after) != theirs {
		t.Errorf("the person's own browser skill was written over:\n%s", after)
	}
}
