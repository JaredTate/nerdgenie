package skill_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/skill"
)

func TestAGoldenFolderLoads(t *testing.T) {
	folder, err := skill.ReadFolder("testdata/skills/news-headlines")
	if err != nil {
		t.Fatalf("the golden folder did not load: %v", err)
	}
	if folder.Definition.Name != "news-headlines" {
		t.Errorf("the name is %q, want news-headlines", folder.Definition.Name)
	}
	if folder.Definition.Description != "Reads the day's headlines from the news site and says what they are." {
		t.Errorf("the description is %q, want the one line under the heading", folder.Definition.Description)
	}
	if len(folder.Steps) != 3 {
		t.Fatalf("the folder has %d steps, want 3", len(folder.Steps))
	}
	if folder.Steps[1].Expect != "Rain" {
		t.Errorf("step two expects %q, want Rain", folder.Steps[1].Expect)
	}
	if folder.Plan.Arguments != "today" || folder.Plan.Expect != "Rain" {
		t.Errorf("the dry run is %+v, want the arguments and expectation from test.md", folder.Plan)
	}
}

func TestAGoldenFolderKeepsItsPermissionsBlock(t *testing.T) {
	folder, err := skill.ReadFolder("testdata/skills/news-headlines")
	if err != nil {
		t.Fatalf("the golden folder did not load: %v", err)
	}
	permissions := folder.Definition.Permissions
	if permissions.BrowserProfile != "news" {
		t.Errorf("the browser profile is %q, want news", permissions.BrowserProfile)
	}
	if len(permissions.Sites) != 1 || permissions.Sites[0] != "news.example.com" {
		t.Errorf("the sites are %v, want the one the block names", permissions.Sites)
	}
	if permissions.DailyLimit != 40 {
		t.Errorf("the daily limit is %d, want 40", permissions.DailyLimit)
	}
	if len(permissions.IrreversibleSteps) != 1 || permissions.IrreversibleSteps[0] != 3 {
		t.Errorf("the steps that cannot be undone are %v, want step three", permissions.IrreversibleSteps)
	}
	if want := []string{"headlines", "news"}; len(permissions.Sites) > 0 && strings.Join(folder.Definition.Triggers, ",") != strings.Join(want, ",") {
		t.Errorf("the triggers are %v, want %v", folder.Definition.Triggers, want)
	}
}

func TestAFolderWithAFileMissingIsRefusedWithTheFileNamed(t *testing.T) {
	_, err := skill.ReadFolder("testdata/broken/no-procedure")
	if err == nil {
		t.Fatal("a folder with no procedure loaded, and it must be refused")
	}
	if !strings.Contains(err.Error(), skill.StepsFile) || !strings.Contains(err.Error(), skill.ScriptFile) {
		t.Errorf("the error is %q, want it to name %s and %s", err, skill.StepsFile, skill.ScriptFile)
	}
}

func TestEveryMissingFileIsNamedInItsOwnError(t *testing.T) {
	whole := map[string][]byte{
		skill.DescriptionFile: []byte("# whole\n\nA skill with every file it needs.\n"),
		skill.StepsFile:       []byte("1. Do the one thing.\n"),
		skill.TestFile:        []byte("arguments:\n"),
		skill.ChangelogFile:   []byte("# changelog for whole\n"),
	}
	if _, err := skill.ParseFolder(whole); err != nil {
		t.Fatalf("the whole folder did not parse: %v", err)
	}

	for _, missing := range []string{skill.DescriptionFile, skill.TestFile, skill.ChangelogFile} {
		short := map[string][]byte{}
		for name, content := range whole {
			if name != missing {
				short[name] = content
			}
		}
		_, err := skill.ParseFolder(short)
		if err == nil {
			t.Fatalf("a folder with no %s parsed, and it must be refused", missing)
		}
		if !strings.Contains(err.Error(), missing) {
			t.Errorf("the error for a missing %s is %q, want it to name the file", missing, err)
		}
	}
}

func TestAStepOverTheSizeLimitIsRefused(t *testing.T) {
	steps := "1. " + strings.Repeat("a very long intent ", (skill.MaxStepBytes/19)+2) + "\n"
	files := map[string][]byte{
		skill.DescriptionFile: []byte("# long-step\n\nA skill whose one step is far too long.\n"),
		skill.StepsFile:       []byte(steps),
		skill.TestFile:        []byte("arguments:\n"),
		skill.ChangelogFile:   []byte("# changelog for long-step\n"),
	}
	_, err := skill.ParseFolder(files)
	if err == nil {
		t.Fatal("a step over the size limit parsed, and it must be refused")
	}
	if !strings.Contains(err.Error(), skill.StepsFile) {
		t.Errorf("the error is %q, want it to name %s", err, skill.StepsFile)
	}
}

func TestAFolderWithBothAScriptAndStepsIsRefused(t *testing.T) {
	files := map[string][]byte{
		skill.DescriptionFile: []byte("# two-ways\n\nA skill that says how to do the job twice.\n"),
		skill.StepsFile:       []byte("1. Do the one thing.\n"),
		skill.ScriptFile:      []byte("#!/bin/sh\necho hello\n"),
		skill.TestFile:        []byte("arguments:\n"),
		skill.ChangelogFile:   []byte("# changelog for two-ways\n"),
	}
	if _, err := skill.ParseFolder(files); err == nil {
		t.Fatal("a folder with both a script and steps parsed, and it must be refused")
	}
}

func TestAFolderNameThatDisagreesWithItsHeadingIsRefused(t *testing.T) {
	built := newHarness(t)
	built.writeSkill(t, "testdata/skills/news-headlines", "other-name")
	if _, err := skill.ReadFolder(built.home.SkillFolder("other-name")); err == nil {
		t.Fatal("a folder whose heading names another skill loaded, and it must be refused")
	}
}

func TestAMarkOnAStepThatIsNotThereIsRefused(t *testing.T) {
	files := map[string][]byte{
		skill.DescriptionFile: []byte("# marks-air\n\nMarks a step that does not exist.\n\n## Permissions\n\n- irreversible step: 9\n"),
		skill.StepsFile:       []byte("1. Do the one thing.\n"),
		skill.TestFile:        []byte("arguments:\n"),
		skill.ChangelogFile:   []byte("# changelog for marks-air\n"),
	}
	_, err := skill.ParseFolder(files)
	if err == nil {
		t.Fatal("a mark on a step that is not there parsed, and it must be refused")
	}
	if !strings.Contains(err.Error(), "step 9") {
		t.Errorf("the error is %q, want it to name step 9", err)
	}
}

func TestCheckNameRefusesWhatCannotBeAFolderName(t *testing.T) {
	for _, name := range []string{"", "-leading", "trailing-", "Upper", "with space", "with/slash", strings.Repeat("a", skill.MaxNameRunes+1)} {
		if err := skill.CheckName(name); err == nil {
			t.Errorf("the name %q was allowed, and it must be refused", name)
		}
	}
	for _, name := range []string{"news", "news-headlines", "post-2-mastodon"} {
		if err := skill.CheckName(name); err != nil {
			t.Errorf("the name %q was refused: %v", name, err)
		}
	}
}

func TestCheckFileNamesRefusesAWayOutOfTheFolder(t *testing.T) {
	for _, name := range []string{"", "..", ".", "../escape", "under/steps.md", ".hidden"} {
		if err := skill.CheckFileNames(map[string][]byte{name: []byte("x")}); err == nil {
			t.Errorf("the file name %q was allowed, and it must be refused", name)
		}
	}
	if err := skill.CheckFileNames(map[string][]byte{skill.StepsFile: []byte("1. Do it.\n")}); err != nil {
		t.Errorf("a plain file name was refused: %v", err)
	}
}

func TestReadSummaryReadsOnlyTheHead(t *testing.T) {
	built := newHarness(t)
	built.writeFiles(t, "head-only", map[string]string{
		skill.DescriptionFile: "# head-only\n\nHas a head and nothing else at all.\n",
	})
	definition, err := skill.ReadSummary(built.home.SkillFolder("head-only"))
	if err != nil {
		t.Fatalf("the head of a folder with nothing else did not read: %v", err)
	}
	if definition.Description != "Has a head and nothing else at all." {
		t.Errorf("the description is %q, want the one line under the heading", definition.Description)
	}
	if _, err := skill.ReadFolder(built.home.SkillFolder("head-only")); err == nil {
		t.Error("the same folder loaded whole, and loading is the strict path")
	}
}

func TestLoadNamesASkillThatIsNotThere(t *testing.T) {
	built := newHarness(t)
	_, err := built.store.Load(context.Background(), "no-such-skill")
	if err == nil || !strings.Contains(err.Error(), "no-such-skill") {
		t.Errorf("loading a skill that is not there gave %v, want an error naming it", err)
	}
}
