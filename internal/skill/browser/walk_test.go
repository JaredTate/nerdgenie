package browser_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/skill"
	"github.com/JaredTate/coeus/internal/skill/browser"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theWalkCommand builds the command over everything the bench has: the fixture
// browser, the store on the temporary home, the screen the previews go to, and
// whichever model the test wants asked.
func theWalkCommand(built *bench, model contract.Model) contract.Command {
	return browser.WalkCommand(browser.WalkOptions{
		Browser: built.worker,
		Model:   model,
		Ask:     built.channel.ShowPreview,
		Skills:  built.store,
		Home:    built.home,
	})
}

// runTheWalkCommand types one line after "/walk" and returns what came back,
// failing the test when it was refused.
func runTheWalkCommand(t *testing.T, walk contract.Command, arguments string) string {
	t.Helper()
	said, err := walk.Run(context.Background(), arguments, contract.CommandContext{})
	if err != nil {
		t.Fatalf("/walk %s was refused: %v", arguments, err)
	}
	return said
}

func TestTheWalkCommandIsNamedAndSaysItsThreeForms(t *testing.T) {
	built := newBench(t)
	walk := theWalkCommand(built, nil)
	if walk.Name != "walk" {
		t.Errorf("the command is called %q, want walk", walk.Name)
	}
	for _, wanted := range []string{"record", "replay", "check"} {
		if !strings.Contains(walk.Help, wanted) {
			t.Errorf("the help line %q does not mention %q", walk.Help, wanted)
		}
	}
	said := runTheWalkCommand(t, walk, "")
	for _, wanted := range []string{"/walk record", "/walk replay", "/walk check"} {
		if !strings.Contains(said, wanted) {
			t.Errorf("/walk on its own said %q, and it should name %q", said, wanted)
		}
	}
}

func TestWalkRecordWritesDownThePageTheBrowserIsOn(t *testing.T) {
	built := newBench(t)
	if _, err := built.worker.Open(context.Background(), testkit.FixtureSimplePage); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	walk := theWalkCommand(built, nil)

	said := runTheWalkCommand(t, walk, "record shop-walk")
	if !strings.Contains(said, "shop-walk") || !strings.Contains(said, "watching") {
		t.Errorf("the answer is %q, and it should name the walk and say that the browser is being watched", said)
	}
	runTheWalkCommand(t, walk, "stop")
	steps, err := browser.StepsOf(built.load(t, "shop-walk"))
	if err != nil {
		t.Fatalf("what was recorded does not read back as a walk: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("the recording holds %d steps, want the one page the browser was on: %+v", len(steps), steps)
	}
	if steps[0].Tool != contract.ToolBrowserOpen || steps[0].Address != testkit.FixtureSimplePage {
		t.Errorf("the recorded step is %+v, want an opening step on the page the browser was on", steps[0])
	}
	sites := built.load(t, "shop-walk").Definition.Permissions.Sites
	if len(sites) != 1 || sites[0] != "fixture.test" {
		t.Errorf("the walk may visit %v, want only the site the page it recorded is on", sites)
	}
}

func TestWalkReplayWalksTheRecordingAndSaysWhatEveryStepDid(t *testing.T) {
	built := newBench(t)
	built.save(t, "fixture-walk", theThreeStepFlow())
	walk := theWalkCommand(built, nil)

	said := runTheWalkCommand(t, walk, "replay fixture-walk")
	for _, wanted := range []string{"fixture-walk", "step 1", "step 2", "step 3", "met"} {
		if !strings.Contains(said, wanted) {
			t.Errorf("the replay said %q, and it should carry %q", said, wanted)
		}
	}
	page, err := built.worker.Read(context.Background(), contract.ReadOptions{})
	if err != nil {
		t.Fatalf("cannot read the page the browser is on: %v", err)
	}
	if page.URL != testkit.FixtureChangedPage {
		t.Errorf("the browser ended on %s, and the recording ends by following the link", page.URL)
	}
}

func TestWalkReplayTakesNoHealedStepUntilTheUserSaysYes(t *testing.T) {
	built, _, model := benchWithARebuiltPage(t, "e9")
	built.channel.AnswerPreviewsWith(contract.AnswerReject)
	walk := theWalkCommand(built, model)

	said := runTheWalkCommand(t, walk, "replay fixture-walk")
	if !strings.Contains(said, "proposed and not made") {
		t.Errorf("the replay said %q, and it should say the change was not made", said)
	}
	page, err := built.worker.Read(context.Background(), contract.ReadOptions{})
	if err != nil {
		t.Fatalf("cannot read the page the browser is on: %v", err)
	}
	if page.URL == testkit.FixtureChangedPage {
		t.Error("the walk command took a healed step the user refused")
	}
	if shown := len(built.channel.Previews()); shown != 1 {
		t.Errorf("the user was shown %d previews, want the one asked before the step was taken", shown)
	}
}

func TestWalkCheckPhotographsEveryStepIntoTheChecksFolder(t *testing.T) {
	built := newBench(t)
	built.save(t, "fixture-walk", theThreeStepFlow())
	walk := theWalkCommand(built, nil)

	said := runTheWalkCommand(t, walk, "check fixture-walk")
	for _, wanted := range []string{"step 1", "step 3", "Picture:"} {
		if !strings.Contains(said, wanted) {
			t.Errorf("the check said %q, and it should carry %q", said, wanted)
		}
	}
	into := filepath.Join(built.home.Root, "checks", "fixture-walk")
	written, err := os.ReadDir(into)
	if err != nil {
		t.Fatalf("cannot read the folder the pictures went into: %v", err)
	}
	if len(written) != 3 {
		t.Fatalf("the check wrote %d pictures into %s, want one for each of the three steps", len(written), into)
	}
	if first := written[0].Name(); first != "01-open-the-fixture-page.png" {
		t.Errorf("the first picture is called %q, and it should be named after the step it shows", first)
	}
}

func TestWalkRefusesWhatItCannotDoAndNamesTheRule(t *testing.T) {
	built := newBench(t)
	built.save(t, "fixture-walk", theThreeStepFlow())
	walk := theWalkCommand(built, nil)
	for _, one := range []struct {
		arguments string
		says      string
	}{
		{"dance about", "record"},
		{"record", "name"},
		{"replay", "name"},
		{"check", "name"},
		{"replay nowhere-at-all", "nowhere-at-all"},
		{"record shop-walk", "no page open"},
	} {
		said, err := walk.Run(context.Background(), one.arguments, contract.CommandContext{})
		if err == nil {
			t.Errorf("/walk %s was allowed and said %q", one.arguments, said)
			continue
		}
		if !strings.Contains(err.Error(), one.says) {
			t.Errorf("/walk %s was refused with %q, and the refusal should say %q", one.arguments, err, one.says)
		}
	}
}

func TestAWalkCommandBuiltWithoutItsPartsRefusesAndSaysWhichIsMissing(t *testing.T) {
	built := newBench(t)
	for _, one := range []struct {
		name      string
		options   browser.WalkOptions
		arguments string
		says      string
	}{
		{
			name:      "with no browser",
			options:   browser.WalkOptions{Skills: built.store, Home: built.home},
			arguments: "replay fixture-walk",
			says:      "browser",
		},
		{
			name:      "with no skill store to save into",
			options:   browser.WalkOptions{Browser: built.worker, Home: built.home},
			arguments: "record shop-walk",
			says:      "save",
		},
		{
			name:      "with no home folder to read a walk from or put pictures in",
			options:   browser.WalkOptions{Browser: built.worker, Skills: built.store},
			arguments: "check fixture-walk",
			says:      "nowhere to put the pictures",
		},
	} {
		t.Run(one.name, func(t *testing.T) {
			said, err := browser.WalkCommand(one.options).Run(context.Background(), one.arguments, contract.CommandContext{})
			if err == nil {
				t.Fatalf("/walk %s was allowed and said %q", one.arguments, said)
			}
			if !strings.Contains(err.Error(), one.says) {
				t.Errorf("the refusal is %q, and it should say %q", err, one.says)
			}
		})
	}
}

func TestWalkAsksOnTheScreenTheCommandWasTypedOn(t *testing.T) {
	built, _, model := benchWithARebuiltPage(t, "e9")
	typedOn := testkit.NewFakeChannel("the-other-screen")
	typedOn.AnswerPreviewsWith(contract.AnswerReject)
	walk := theWalkCommand(built, model)

	if _, err := walk.Run(context.Background(), "replay fixture-walk", contract.CommandContext{Channel: typedOn}); err != nil {
		t.Fatalf("/walk replay was refused: %v", err)
	}
	if shown := len(typedOn.Previews()); shown != 1 {
		t.Errorf("the screen the command was typed on was shown %d previews, want the one about the healed step", shown)
	}
	if shown := len(built.channel.Previews()); shown != 0 {
		t.Errorf("the screen nobody typed on was shown %d previews", shown)
	}
}

func TestWalkRecordOfAPageWithNoSiteToNameNamesNone(t *testing.T) {
	built := newBench(t)
	built.worker.AddPage(contract.Snapshot{URL: "about:blank", Title: "Nothing yet", TabID: "t1"})
	if _, err := built.worker.Open(context.Background(), "about:blank"); err != nil {
		t.Fatalf("cannot open a page with no site: %v", err)
	}
	walk := theWalkCommand(built, nil)

	runTheWalkCommand(t, walk, "record blank-walk")
	runTheWalkCommand(t, walk, "stop")
	definition := built.load(t, "blank-walk").Definition
	if len(definition.Permissions.Sites) != 0 {
		t.Errorf("the walk may visit %v, and the page it recorded is on no site to name", definition.Permissions.Sites)
	}
	if !strings.Contains(definition.Description, "about:blank") {
		t.Errorf("the walk describes itself as %q, and it should say where it starts", definition.Description)
	}
}

func TestWalkRecordOfAPageOnAVeryLongSiteStillFitsTheOneLineInThePrompt(t *testing.T) {
	built := newBench(t)
	// The host is as long as a host name may be: four labels of sixty letters,
	// each inside the sixty-three a label allows, under the two hundred and
	// fifty-three a whole name allows.
	long := "https://" + strings.Repeat("a", 60) + "." + strings.Repeat("b", 60) + "." + strings.Repeat("c", 60) + "." + strings.Repeat("d", 60) + ".test/shop"
	built.worker.AddPage(contract.Snapshot{URL: long, Title: "A long way from home", TabID: "t1"})
	if _, err := built.worker.Open(context.Background(), long); err != nil {
		t.Fatalf("cannot open the page on the long site: %v", err)
	}
	walk := theWalkCommand(built, nil)

	runTheWalkCommand(t, walk, "record long-walk")
	runTheWalkCommand(t, walk, "stop")
	description := built.load(t, "long-walk").Definition.Description
	if said := len([]rune(description)); said > 200 {
		t.Errorf("the walk describes itself in %d characters, and the one line in the prompt holds two hundred", said)
	}
}

func TestWalkRecordUnderANameNoFolderCanHaveIsRefused(t *testing.T) {
	built := newBench(t)
	if _, err := built.worker.Open(context.Background(), testkit.FixtureSimplePage); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	walk := theWalkCommand(built, nil)

	if said, err := walk.Run(context.Background(), "record Shop Walk", contract.CommandContext{}); err == nil {
		t.Fatalf("a walk was saved under a name no folder can have and said %q", said)
	}
}

func TestWalkReplayOfABrowserThatIsGoneIsRefused(t *testing.T) {
	built := newBench(t)
	built.save(t, "click-walk", []browser.Step{{
		Number: 1, Intent: "Follow the link.", Tool: contract.ToolBrowserClick,
		Element: browser.Descriptor{Ref: "e1", Role: "link", Name: "Change the page"}, Expectation: "the page changed",
	}})
	walk := theWalkCommand(built, nil)
	if err := built.worker.Close(); err != nil {
		t.Fatalf("cannot close the fixture browser: %v", err)
	}

	if said, err := walk.Run(context.Background(), "replay click-walk", contract.CommandContext{}); err == nil {
		t.Fatalf("a walk was replayed in a browser that is gone and said %q", said)
	}
}

func TestWalkCheckSaysWhenThePicturesHaveNowhereToGo(t *testing.T) {
	built := newBench(t)
	built.save(t, "fixture-walk", theThreeStepFlow())
	inTheWay := filepath.Join(built.home.Root, "checks")
	if err := os.WriteFile(inTheWay, []byte("not a folder\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot put something in the way of the checks folder: %v", err)
	}
	walk := theWalkCommand(built, nil)

	said, err := walk.Run(context.Background(), "check fixture-walk", contract.CommandContext{})
	if err == nil {
		t.Fatalf("a check wrote its pictures into a file and said %q", said)
	}
	if !strings.Contains(err.Error(), "checks") {
		t.Errorf("the refusal is %q, and it should name the folder it could not make", err)
	}
}

func TestWalkCheckRefusesASkillThatIsNotAWalk(t *testing.T) {
	built := newBench(t)
	files := map[string][]byte{
		skill.DescriptionFile: skill.RenderDescriptionFile(definitionOf("shell-thing")),
		skill.ScriptFile:      []byte("#!/bin/sh\necho hello\n"),
	}
	if err := built.store.Save(context.Background(), contract.SkillSavedByPerson, "shell-thing", files); err != nil {
		t.Fatalf("cannot save a skill that carries a script: %v", err)
	}
	walk := theWalkCommand(built, nil)

	if said, err := walk.Run(context.Background(), "check shell-thing", contract.CommandContext{}); err == nil {
		t.Fatalf("a skill carrying a script was walked as a browser check and said %q", said)
	}
}
