package browser_test

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/skill"
	"github.com/JaredTate/nerdgenie/internal/skill/browser"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// startOfTheTests is the time every fake clock in these tests starts at, so that
// a changelog line a test reads is the same every time it runs.
var startOfTheTests = time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC)

// bench is one skill store on a temporary home with a fixture browser in front
// of it, which is everything a test in this package needs.
type bench struct {
	home    contract.Home
	store   *skill.Store
	worker  *testkit.FakeBrowserWorker
	channel *testkit.FakeChannel
}

// newBench builds the store, the fixture browser, and the screen the previews go
// to. Nothing here touches the real disk outside the temporary home, and nothing
// opens a real browser.
func newBench(t *testing.T) *bench {
	t.Helper()
	built := &bench{
		home:    testkit.NewTempHome(t),
		worker:  testkit.NewFakeBrowserWorker(),
		channel: testkit.NewFakeChannel("terminal"),
	}
	store, err := skill.New(skill.Options{
		Home:       built.home,
		Clock:      testkit.NewFakeClock(startOfTheTests),
		Tools:      testkit.NewFakeToolRegistry(),
		Permission: testkit.NewFakePermission(contract.RulingAllow),
		Ask:        built.channel.ShowPreview,
	})
	if err != nil {
		t.Fatalf("cannot build the skill store: %v", err)
	}
	built.store = store
	t.Cleanup(func() {
		if err := built.worker.Close(); err != nil {
			t.Errorf("cannot close the fixture browser: %v", err)
		}
	})
	return built
}

// definitionOf is the SKILL.md of a recording made in these tests: one site, and
// the steps a caller marks as ones that cannot be undone.
func definitionOf(name string, irreversible ...int) skill.Definition {
	return skill.Definition{
		Name:        name,
		Description: "Walks the fixture page and checks what it shows.",
		Triggers:    []string{name},
		Permissions: skill.Permissions{
			Sites:             []string{"fixture.test"},
			DailyLimit:        skill.DefaultDailyLimit,
			IrreversibleSteps: irreversible,
		},
	}
}

// save writes a recording into the store under a name and returns the folder it
// wrote, read back off disk the way a replay would read it.
func (built *bench) save(t *testing.T, name string, steps []browser.Step, irreversible ...int) skill.Folder {
	t.Helper()
	files := map[string][]byte{
		skill.DescriptionFile: skill.RenderDescriptionFile(definitionOf(name, irreversible...)),
		skill.StepsFile:       browser.RenderSteps(steps),
		skill.TestFile:        skill.RenderTestFile(name, skill.DryRunPlan{Arguments: testkit.FixtureSimplePage}),
	}
	if err := built.store.Save(context.Background(), contract.SkillSavedByPerson, name, files); err != nil {
		t.Fatalf("cannot save the recording %q: %v", name, err)
	}
	return built.load(t, name)
}

// load reads one skill folder off disk, which is what a replay is given.
func (built *bench) load(t *testing.T, name string) skill.Folder {
	t.Helper()
	folder, err := skill.ReadFolder(built.home.SkillFolder(name))
	if err != nil {
		t.Fatalf("cannot read the skill folder %q back: %v", name, err)
	}
	return folder
}

// theThreeStepFlow is the recording every replay test starts from: open the
// fixture page, write a note in the box on it, and follow the link that changes
// the page.
func theThreeStepFlow() []browser.Step {
	return []browser.Step{
		{
			Number: 1, Intent: "Open the fixture page.", Tool: contract.ToolBrowserOpen,
			Address: testkit.FixtureSimplePage, Expectation: "a simple page with a form on it",
		},
		{
			Number: 2, Intent: "Write the note.", Tool: contract.ToolBrowserType, Typed: "nine years",
			Element:     browser.Descriptor{Ref: "e3", Role: "textbox", Name: "Note", Shown: "Note"},
			Expectation: "the note box holds the words",
		},
		{
			Number: 3, Intent: "Follow the link that changes the page.", Tool: contract.ToolBrowserClick,
			Element:     browser.Descriptor{Ref: "e1", Role: "link", Name: "Change the page", Shown: "Change the page"},
			Expectation: "the page changed",
		},
	}
}
