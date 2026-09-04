//go:build integration

// This file is the integration test for browser skills: the real clock, the real
// permission function, a real skills folder on a real disk under a temporary
// home, and real picture files written to it. The browser is the only fake,
// because a test that drove a real Chrome would be testing Chrome.

package browser_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/clock"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/permission"
	"github.com/JaredTate/nerdgenie/internal/skill"
	"github.com/JaredTate/nerdgenie/internal/skill/browser"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestABrowserSkillGoesThroughItsWholeLifeOnARealDisk(t *testing.T) {
	ctx := context.Background()
	home := testkit.NewTempHome(t)
	realClock := clock.System()
	decider, err := permission.New(contract.DefaultConfig(), realClock)
	if err != nil {
		t.Fatalf("cannot build the permission function: %v", err)
	}
	channel := testkit.NewFakeChannel("terminal")
	store, err := skill.New(skill.Options{
		Home:       home,
		Clock:      realClock,
		Tools:      testkit.NewFakeToolRegistry(),
		Permission: decider,
		Standing:   decider,
		Ask:        channel.ShowPreview,
	})
	if err != nil {
		t.Fatalf("cannot build the skill store: %v", err)
	}
	worker := testkit.NewFakeBrowserWorker()
	t.Cleanup(func() {
		if err := worker.Close(); err != nil {
			t.Errorf("cannot close the fixture browser: %v", err)
		}
	})

	recordSaveAndReplay(ctx, t, store, home, worker)
	checkTheAppAndWriteThePictures(ctx, t, home, worker)
	installTheShippedQualitySkill(ctx, t, store, home)
}

// recordSaveAndReplay drives the fixture browser, saves what it did as a skill
// folder on the real disk, and replays that folder off the disk.
func recordSaveAndReplay(ctx context.Context, t *testing.T, store *skill.Store, home contract.Home, worker *testkit.FakeBrowserWorker) {
	t.Helper()
	recorder, err := browser.NewRecorder(worker)
	if err != nil {
		t.Fatalf("cannot build the recorder: %v", err)
	}
	if _, err := recorder.Open(ctx, "Open the fixture page.", testkit.FixtureSimplePage, "a simple page with a form on it"); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	if _, err := recorder.Type(ctx, "Write the note.", "e3", "nine years", "the note box holds the words"); err != nil {
		t.Fatalf("cannot write the note: %v", err)
	}
	if err := recorder.Save(ctx, store, skill.Definition{
		Name:        "fixture-walk",
		Description: "Walks the fixture page and writes a note on it.",
		Triggers:    []string{"fixture walk"},
		Permissions: skill.Permissions{Sites: []string{"fixture.test"}, DailyLimit: skill.DefaultDailyLimit},
	}); err != nil {
		t.Fatalf("cannot save the recording: %v", err)
	}

	folder, err := skill.ReadFolder(home.SkillFolder("fixture-walk"))
	if err != nil {
		t.Fatalf("the saved recording will not read back off the disk: %v", err)
	}
	replayer, err := browser.New(browser.Options{Browser: worker, Skills: store})
	if err != nil {
		t.Fatalf("cannot build the replayer: %v", err)
	}
	report, err := replayer.Replay(ctx, folder)
	if err != nil {
		t.Fatalf("cannot replay the recording: %v", err)
	}
	if !report.Met() {
		t.Fatalf("the replay off the real disk did not do what was recorded:\n%s", report)
	}
	if listed, err := store.List(ctx); err != nil || len(listed) != 1 {
		t.Fatalf("the skills folder lists %v with error %v, want the one recording", listed, err)
	}
}

// checkTheAppAndWriteThePictures runs a visual check that writes real PNG files
// into a real folder and reads them back.
func checkTheAppAndWriteThePictures(ctx context.Context, t *testing.T, home contract.Home, worker *testkit.FakeBrowserWorker) {
	t.Helper()
	checker, err := browser.NewChecker(worker)
	if err != nil {
		t.Fatalf("cannot build the checker: %v", err)
	}
	into := filepath.Join(home.Root, "pictures")
	result, err := checker.Check(ctx, browser.Request{Name: "the fixture app", Into: into, Steps: []browser.Step{
		{
			Number: 1, Intent: "Open the app.", Tool: contract.ToolBrowserOpen,
			Address: testkit.FixtureSimplePage, Expectation: "a simple page with a form on it",
		},
		{
			Number: 2, Intent: "Follow the link.", Tool: contract.ToolBrowserClick,
			Element:     browser.Descriptor{Ref: "e1", Role: "link", Name: "Change the page", Shown: "Change the page"},
			Expectation: "the invoice was paid in full",
		},
	}})
	if err != nil {
		t.Fatalf("cannot check the fixture app: %v", err)
	}
	if len(result.Findings) != 2 || !result.Findings[0].Met || result.Findings[1].Met {
		t.Fatalf("the check should report one state met and one not:\n%s", result.Markdown())
	}
	for _, finding := range result.Findings {
		about, err := os.Stat(finding.Picture)
		if err != nil {
			t.Fatalf("the picture for step %d is not on the disk: %v", finding.Number, err)
		}
		if about.Size() == 0 {
			t.Errorf("the picture for step %d is empty", finding.Number)
		}
	}
}

// installTheShippedQualitySkill puts the skill the program ships into the real
// skills folder and reads it back as a walk.
func installTheShippedQualitySkill(ctx context.Context, t *testing.T, store *skill.Store, home contract.Home) {
	t.Helper()
	if err := browser.InstallQASkill(ctx, store); err != nil {
		t.Fatalf("cannot install the quality skill: %v", err)
	}
	folder, err := skill.ReadFolder(home.SkillFolder(browser.QASkillName))
	if err != nil {
		t.Fatalf("the installed quality skill will not read back off the disk: %v", err)
	}
	if _, err := browser.StepsOf(folder); err != nil {
		t.Fatalf("the installed quality skill does not read as a walk: %v", err)
	}
	changelog, err := store.Changelog(browser.QASkillName)
	if err != nil {
		t.Fatalf("the installed quality skill has no changelog: %v", err)
	}
	if !strings.Contains(changelog, "shipped with Nerd Genie") {
		t.Errorf("the changelog does not say where the skill came from:\n%s", changelog)
	}
}
